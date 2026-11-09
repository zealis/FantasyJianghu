package gateway

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/router"
	"fantasy-jianghu/server/internal/session"
)

// Cfg 为心跳口径（§5.2：15 秒心跳、连续 3 次无响应断开）。
type Cfg struct {
	HeartbeatSeconds int
	MissLimit        int
}

func (c Cfg) silenceLimit() time.Duration {
	return time.Duration(c.HeartbeatSeconds*c.MissLimit) * time.Second
}

type ctxKeyConn struct{}

// SessionBinder 为连接侧会话绑定契约（业务模块经 ctx 取用，测试可注入假件）。
type SessionBinder interface {
	BindSession(sess *session.Session)
}

// WithConn 把连接句柄放入请求上下文。
func WithConn(ctx context.Context, b SessionBinder) context.Context {
	return context.WithValue(ctx, ctxKeyConn{}, b)
}

// ConnFromCtx 取当前连接句柄（鉴权等处理器经此绑定会话）。
func ConnFromCtx(ctx context.Context) SessionBinder {
	b, _ := ctx.Value(ctxKeyConn{}).(SessionBinder)
	return b
}

type Conn struct {
	ws         *websocket.Conn
	srv        *Server
	out        chan protocol.Frame
	mu         sync.RWMutex
	sess       *session.Session
	registered bool
}

// BindSession 绑定会话并进入按令牌索引的在线表（供 Hub 推送）。
func (c *Conn) BindSession(sess *session.Session) {
	c.mu.Lock()
	c.sess = sess
	c.registered = true
	c.mu.Unlock()
	c.srv.mu.Lock()
	c.srv.conns[sess.Token] = c
	c.srv.mu.Unlock()
}

func (c *Conn) session() *session.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sess
}

func (c *Conn) token() string {
	s := c.session()
	if s == nil {
		return ""
	}
	return s.Token
}

// Send 入队一帧（广播类可丢弃过期：满则丢）。
func (c *Conn) Send(main, sub uint16, payload map[string]any) {
	select {
	case c.out <- protocol.Frame{Main: main, Sub: sub, Seq: 0, Payload: payload}:
	default:
		c.srv.logf("out 满丢弃 main=%d sub=%d token=%s", main, sub, c.token())
	}
}

// SendTo 为 Hub 接口实现：按令牌定向推送（世界模块广播用）。
func (s *Server) SendTo(token string, main, sub uint16, payload map[string]any) {
	s.mu.RLock()
	c, ok := s.conns[token]
	s.mu.RUnlock()
	if ok {
		c.Send(main, sub, payload)
	}
}

// SendToChar 按角色定向推送（任务进度 11/4 等）。返回是否命中在线连接。
// 连接按令牌索引，角色绑定在会话上，故遍历在线表匹配 CharID。
// 注意：先快照连接再逐个取会话——BindSession 的锁序是 c.mu → s.mu，
// 此处若持 s.mu 再取 c.mu 会构成 ABBA 死锁。
func (s *Server) SendToChar(charID int64, main, sub uint16, payload map[string]any) bool {
	if charID == 0 {
		return false
	}
	s.mu.RLock()
	all := make([]*Conn, 0, len(s.conns))
	for _, c := range s.conns {
		all = append(all, c)
	}
	s.mu.RUnlock()
	hit := 0
	for _, c := range all {
		if sess := c.session(); sess != nil && sess.CharID == charID {
			c.Send(main, sub, payload)
			hit++
		}
	}
	return hit > 0
}

type Server struct {
	Router   *router.Router
	Sessions *session.Manager
	Cfg      Cfg
	logf     func(string, ...any)
	// OnClose 为连接关闭回调（按令牌）：装配层接到世界域做掉线清理。
	// 仅当关闭连接仍是该令牌的注册连接时触发（旧连接误杀重连会话防护）。
	OnClose func(token string)

	mu    sync.RWMutex
	conns map[string]*Conn
}

func NewServer(r *router.Router, s *session.Manager, cfg Cfg) *Server {
	srv := &Server{
		Router:   r,
		Sessions: s,
		Cfg:      cfg,
		logf:     log.Printf,
		conns:    make(map[string]*Conn),
	}
	// 心跳 1/0：ping → pong（网关内置）。
	r.Register(protocol.MainSystem, protocol.SubPing, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
		return map[string]any{"code": 0, "server_time": time.Now().Unix()}, 0, ""
	})
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true }, // M1 开发期同机联调，跨源放开
	}
	ws, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn := &Conn{ws: ws, srv: s, out: make(chan protocol.Frame, 64)}
	defer func() {
		if tok := conn.token(); tok != "" {
			s.mu.Lock()
			isCurrent := s.conns[tok] == conn
			if isCurrent {
				delete(s.conns, tok)
			}
			s.mu.Unlock()
			if isCurrent && s.OnClose != nil {
				s.OnClose(tok)
			}
		}
		ws.Close()
		close(conn.out)
	}()
	go conn.writeLoop()
	conn.readLoop()
}

func (c *Conn) writeLoop() {
	for f := range c.out {
		b, err := protocol.EncodeFrame(f)
		if err != nil {
			continue
		}
		c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := c.ws.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}

// readLoop 逐帧读取与分发；静默超过 心跳×容忍 即踢线（§5.2）。
func (c *Conn) readLoop() {
	limit := c.srv.Cfg.silenceLimit()
	for {
		c.ws.SetReadDeadline(time.Now().Add(limit))
		c.ws.SetReadLimit(64 << 10)
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			if isTimeout(err) {
				c.Send(protocol.MainSystem, protocol.SubKick, protocol.ErrPayload(1, "心跳超时"))
				c.srv.logf("kick heartbeat_timeout token=%s", c.token())
				time.Sleep(200 * time.Millisecond) // 留出踢下线帧发送窗口
			}
			return
		}
		f, err := protocol.DecodeFrame(raw)
		if err != nil {
			c.srv.logf("bad frame: %v", err)
			continue
		}
		c.dispatch(f)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func (c *Conn) dispatch(f protocol.Frame) {
	ctx := WithConn(context.Background(), c)
	sess := c.session()

	// 幂等去重：窗口内同序号只执行一次（§5.2）；重放静默丢弃。
	if sess != nil && c.srv.Router.Registered(f.Main, f.Sub) {
		if !sess.CheckSeq(f.Seq) {
			c.srv.logf("replay dropped main=%d sub=%d seq=%d", f.Main, f.Sub, f.Seq)
			return
		}
	}

	payload, code, msg, ok := c.srv.Router.Dispatch(ctx, sess, f)
	if !ok {
		c.wsSend(protocol.Frame{Main: f.Main, Sub: f.Sub, Seq: f.Seq, Payload: protocol.ErrPayload(1, "协议未登记")})
		return
	}
	if code != 0 {
		// 错误回包同样沿用客户端 seq（请求-响应配对）。
		c.wsSend(protocol.Frame{Main: f.Main, Sub: f.Sub, Seq: f.Seq, Payload: protocol.ErrPayload(code, msg)})
		return
	}
	if payload == nil {
		payload = map[string]any{"code": 0}
	}
	// 成功回包统一携带 code=0（错误码表 §gateway），客户端一致判读。
	if _, has := payload["code"]; !has {
		payload["code"] = 0
	}
	// 回包沿用客户端 seq（请求-响应配对）。
	c.wsSend(protocol.Frame{Main: f.Main, Sub: f.Sub, Seq: f.Seq, Payload: payload})
}

func (c *Conn) wsSend(f protocol.Frame) {
	select {
	case c.out <- f:
	default:
		c.srv.logf("out 满丢弃响应 main=%d seq=%d", f.Main, f.Seq)
	}
}
