// Package session 为会话管理（开发技术规范 §5.2）：会话令牌、心跳时间戳、
// 幂等序号窗口。唯一真相在 kv（Redis/内存），连接期状态在内存 Session。
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"fantasy-jianghu/server/internal/kv"
)

var ErrNotFound = errors.New("session: not found")

// seqWindow 为幂等去重窗口（窗口内同序号只执行一次，§5.2）。
const seqWindow = 256

type Session struct {
	Token     string    `json:"token"`
	AccountID int64     `json:"account_id"`
	CharID    int64     `json:"char_id"`
	LastSeen  time.Time `json:"last_seen"`
	// 以下为连接期内存状态，不序列化。
	seenSeqs  map[uint32]struct{}
	seenOrder []uint32
}

func (s *Session) CheckSeq(seq uint32) bool {
	if s.seenSeqs == nil {
		s.seenSeqs = make(map[uint32]struct{}, seqWindow)
	}
	if _, dup := s.seenSeqs[seq]; dup {
		return false
	}
	s.seenSeqs[seq] = struct{}{}
	s.seenOrder = append(s.seenOrder, seq)
	if len(s.seenOrder) > seqWindow {
		oldest := s.seenOrder[0]
		s.seenOrder = s.seenOrder[1:]
		delete(s.seenSeqs, oldest)
	}
	return true
}

type Manager struct {
	store kv.Store
	ttl   time.Duration
}

func NewManager(store kv.Store, ttl time.Duration) *Manager {
	return &Manager{store: store, ttl: ttl}
}

func key(token string) string { return "sess:" + token }

// Create 生成 32 字节随机令牌（hex 存储）并持久化。
func (m *Manager) Create(ctx context.Context, accountID int64) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	s := &Session{Token: token, AccountID: accountID, LastSeen: time.Now()}
	if err := m.persist(ctx, s); err != nil {
		return "", err
	}
	return token, nil
}

func (m *Manager) Get(ctx context.Context, token string) (*Session, error) {
	raw, err := m.store.Get(ctx, key(token))
	if errors.Is(err, kv.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	s.LastSeen = time.Now()
	// 滑动续期。
	if err := m.store.Expire(ctx, key(token), m.ttl); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return nil, err
	}
	return &s, nil
}

// Save 回写连接期变更（选角绑定等），供断线重连恢复。
func (m *Manager) Save(ctx context.Context, s *Session) error {
	return m.persist(ctx, s)
}

func (m *Manager) persist(ctx context.Context, s *Session) error {
	b, err := json.Marshal(struct {
		Token     string    `json:"token"`
		AccountID int64     `json:"account_id"`
		CharID    int64     `json:"char_id"`
		LastSeen  time.Time `json:"last_seen"`
	}{s.Token, s.AccountID, s.CharID, s.LastSeen})
	if err != nil {
		return err
	}
	return m.store.Set(ctx, key(s.Token), b, m.ttl)
}

func (m *Manager) Delete(ctx context.Context, token string) error {
	return m.store.Del(ctx, key(token))
}

// BindCharacter 绑定已选角色（连接期状态，调用方经 Save 持久化）。
func (m *Manager) BindCharacter(s *Session, charID int64) {
	s.CharID = charID
}
