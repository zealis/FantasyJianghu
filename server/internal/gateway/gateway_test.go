package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/router"
	"fantasy-jianghu/server/internal/session"
)

// newTestServer 起一个真实 WS 网关并返回客户端连接。
func newTestServer(t *testing.T, cfg Cfg, register func(r *router.Router)) *websocket.Conn {
	t.Helper()
	sessions := session.NewManager(kv.NewMemory(), time.Hour)
	var r router.Router
	if register != nil {
		register(&r)
	}
	s := NewServer(&r, sessions, cfg)
	currentTestServer = s
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("拨号: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// currentTestServer 为最近一次 newTestServer 创建的网关句柄（测试断言用）。
var currentTestServer *Server

func sendFrame(t *testing.T, c *websocket.Conn, f protocol.Frame) {
	t.Helper()
	b, err := protocol.EncodeFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("发帧: %v", err)
	}
}

func readFrame(t *testing.T, c *websocket.Conn, timeout time.Duration) (protocol.Frame, error) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := c.ReadMessage()
	if err != nil {
		return protocol.Frame{}, err
	}
	return protocol.DecodeFrame(raw)
}

func TestPingPong(t *testing.T) {
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, nil)
	sendFrame(t, conn, protocol.Frame{Main: protocol.MainSystem, Sub: protocol.SubPing, Seq: 1, Payload: map[string]any{}})
	f, err := readFrame(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("读 pong: %v", err)
	}
	if f.Main != protocol.MainSystem || f.Sub != protocol.SubPing || f.Seq != 1 {
		t.Fatalf("pong 帧不一致: %+v", f)
	}
	if p, _ := f.Payload["code"].(float64); p != 0 {
		t.Fatalf("pong code = %v, 期望 0", f.Payload["code"])
	}
}

func TestErrorHandlerReturnsCode(t *testing.T) {
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, func(r *router.Router) {
		r.Register(2, 9, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			return nil, 5, "不存在"
		})
	})
	sendFrame(t, conn, protocol.Frame{Main: 2, Sub: 9, Seq: 8, Payload: map[string]any{}})
	f, err := readFrame(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("读错误帧: %v", err)
	}
	if f.Seq != 8 {
		t.Fatalf("错误帧 seq = %d, 期望沿用 8", f.Seq)
	}
	if p, _ := f.Payload["code"].(float64); p != 5 || f.Payload["msg"] != "不存在" {
		t.Fatalf("错误负载不一致: %+v", f.Payload)
	}
}

func TestUnregisteredMainRepliesError(t *testing.T) {
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, nil)
	sendFrame(t, conn, protocol.Frame{Main: 99, Sub: 0, Seq: 2, Payload: map[string]any{}})
	f, err := readFrame(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("读帧: %v", err)
	}
	if p, _ := f.Payload["code"].(float64); p == 0 {
		t.Fatalf("未登记主码应回错误码，收到 %+v", f.Payload)
	}
}

func TestSeqReplayDropped(t *testing.T) {
	var calls int32
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, func(r *router.Router) {
		// 先鉴权建立会话（幂等窗口挂在会话上）。
		r.Register(protocol.MainAccount, protocol.SubAuth, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			c := ConnFromCtx(ctx)
			c.BindSession(&session.Session{Token: "t-replay", AccountID: 1})
			return map[string]any{"token": "t-replay"}, 0, ""
		})
		r.Register(2, 9, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			atomic.AddInt32(&calls, 1)
			return map[string]any{"ok": true}, 0, ""
		})
	})
	sendFrame(t, conn, protocol.Frame{Main: protocol.MainAccount, Sub: protocol.SubAuth, Seq: 6, Payload: map[string]any{"token": "x"}})
	if _, err := readFrame(t, conn, 2*time.Second); err != nil {
		t.Fatalf("鉴权回包: %v", err)
	}
	f := protocol.Frame{Main: 2, Sub: 9, Seq: 7, Payload: map[string]any{}}
	sendFrame(t, conn, f) // 首次执行
	sendFrame(t, conn, f) // 重放：静默丢弃
	got, err := readFrame(t, conn, 2*time.Second)
	if err != nil {
		t.Fatalf("首次回包未收到: %v", err)
	}
	if got.Seq != 7 {
		t.Fatalf("回包 seq = %d", got.Seq)
	}
	if _, err := readFrame(t, conn, 300*time.Millisecond); err == nil {
		t.Fatal("重放不应有第二次回包（应静默丢弃）")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("handler 调用次数 = %d, 期望 1（幂等去重）", n)
	}
}

func TestHeartbeatKick(t *testing.T) {
	// 注入短超时：1 秒心跳 × 2 次容忍 → 静默 2 秒踢线。
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 1, MissLimit: 2}, nil)
	start := time.Now()
	f, err := readFrame(t, conn, 4*time.Second)
	if err != nil {
		t.Fatalf("未收到踢下线帧: %v", err)
	}
	if f.Main != protocol.MainSystem || f.Sub != protocol.SubKick {
		t.Fatalf("踢下线帧不一致: %+v", f)
	}
	if elapsed := time.Since(start); elapsed < 1500*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Fatalf("踢线耗时 %v，期望约 2 秒", elapsed)
	}
	// 踢线后连接应被服务端关闭。
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("踢线后连接应关闭")
	}
}

func TestConnCtxAndBind(t *testing.T) {
	var bound *session.Session
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, func(r *router.Router) {
		r.Register(2, 1, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			c := ConnFromCtx(ctx)
			if c == nil {
				return nil, 1, "无连接上下文"
			}
			s := &session.Session{Token: "t-test", AccountID: 9}
			c.BindSession(s)
			return map[string]any{"token": "t-test"}, 0, ""
		})
		r.Register(2, 2, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			bound = sess
			return map[string]any{"acct": sess.AccountID}, 0, ""
		})
	})
	sendFrame(t, conn, protocol.Frame{Main: 2, Sub: 1, Seq: 1, Payload: map[string]any{}})
	if _, err := readFrame(t, conn, 2*time.Second); err != nil {
		t.Fatalf("鉴权回包: %v", err)
	}
	sendFrame(t, conn, protocol.Frame{Main: 2, Sub: 2, Seq: 2, Payload: map[string]any{}})
	if _, err := readFrame(t, conn, 2*time.Second); err != nil {
		t.Fatalf("第二回包: %v", err)
	}
	if bound == nil || bound.AccountID != 9 {
		t.Fatalf("BindSession 后续请求未取到会话: %+v", bound)
	}
}

func TestOnCloseNotifiedWithToken(t *testing.T) {
	closed := make(chan string, 1)
	conn := newTestServer(t, Cfg{HeartbeatSeconds: 15, MissLimit: 3}, func(r *router.Router) {
		r.Register(protocol.MainAccount, protocol.SubAuth, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			ConnFromCtx(ctx).BindSession(&session.Session{Token: "t-close", AccountID: 1})
			return map[string]any{"token": "t-close"}, 0, ""
		})
	})
	srv := currentTestServer
	srv.OnClose = func(token string) { closed <- token }
	sendFrame(t, conn, protocol.Frame{Main: protocol.MainAccount, Sub: protocol.SubAuth, Seq: 1, Payload: map[string]any{"token": "x"}})
	if _, err := readFrame(t, conn, 2*time.Second); err != nil {
		t.Fatalf("鉴权: %v", err)
	}
	conn.Close()
	select {
	case tok := <-closed:
		if tok != "t-close" {
			t.Fatalf("OnClose token = %q", tok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("连接关闭未通知 OnClose（掉线幽灵玩家根因）")
	}
}

func TestSameTokenRebindKeepsNewConn(t *testing.T) {
	// 同令牌重连：旧连接退出不得删除新连接的广播注册。
	var s *Server
	reg := func(r *router.Router) {
		r.Register(protocol.MainAccount, protocol.SubAuth, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
			ConnFromCtx(ctx).BindSession(&session.Session{Token: "t-rebind", AccountID: 1})
			return map[string]any{"token": "t-rebind"}, 0, ""
		})
	}
	sessions := session.NewManager(kv.NewMemory(), time.Hour)
	var r router.Router
	reg(&r)
	s = NewServer(&r, sessions, Cfg{HeartbeatSeconds: 15, MissLimit: 3})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	c1, _, _ := websocket.DefaultDialer.Dial(url, nil)
	defer c1.Close()
	sendFrame(t, c1, protocol.Frame{Main: protocol.MainAccount, Sub: protocol.SubAuth, Seq: 1, Payload: map[string]any{}})
	if _, err := readFrame(t, c1, 2*time.Second); err != nil {
		t.Fatalf("c1 鉴权: %v", err)
	}
	c2, _, _ := websocket.DefaultDialer.Dial(url, nil)
	defer c2.Close()
	sendFrame(t, c2, protocol.Frame{Main: protocol.MainAccount, Sub: protocol.SubAuth, Seq: 1, Payload: map[string]any{}})
	if _, err := readFrame(t, c2, 2*time.Second); err != nil {
		t.Fatalf("c2 鉴权: %v", err)
	}
	c1.Close() // 旧连接退出
	time.Sleep(300 * time.Millisecond)

	s.SendTo("t-rebind", 3, 3, map[string]any{"x": 1.0})
	// 新连接必须仍能收到推送。
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, raw, err := c2.ReadMessage(); err != nil {
		t.Fatalf("同令牌重连后新连接收不到推送（I2）: %v", err)
	} else {
		f, _ := protocol.DecodeFrame(raw)
		if f.Main != 3 || f.Sub != 3 {
			t.Fatalf("收到非预期帧: %+v", f)
		}
	}
}
