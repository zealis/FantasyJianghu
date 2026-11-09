package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/kv"
)

func newManager(t *testing.T) (*Manager, kv.Store) {
	t.Helper()
	store := kv.NewMemory()
	return NewManager(store, 24*time.Hour), store
}

func TestCreateGetRoundtrip(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	token, err := m.Create(ctx, 42)
	if err != nil || len(token) != 64 { // 32 字节 hex
		t.Fatalf("Create = (%q, %v)", token, err)
	}
	s, err := m.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.AccountID != 42 || s.CharID != 0 {
		t.Fatalf("会话内容不一致: %+v", s)
	}
	if time.Since(s.LastSeen) > time.Second {
		t.Fatalf("LastSeen 未刷新: %v", s.LastSeen)
	}
}

func TestDelete(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	token, _ := m.Create(ctx, 1)
	if err := m.Delete(ctx, token); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete 后 Get = %v, 期望 ErrNotFound", err)
	}
}

func TestSeqIdempotencyWindow(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	token, _ := m.Create(ctx, 1)
	s, _ := m.Get(ctx, token)

	if !s.CheckSeq(1) {
		t.Fatal("首次 seq 应通过")
	}
	if s.CheckSeq(1) {
		t.Fatal("同 seq 重放应拒绝（窗口内只执行一次）")
	}
	// 填满 256 窗口，最旧 seq 被逐出后可复用。
	for i := uint32(2); i <= 300; i++ {
		s.CheckSeq(i)
	}
	if !s.CheckSeq(1) {
		t.Fatal("窗口逐出后旧 seq 应可通过")
	}
}

func TestBindAndSavePersistsChar(t *testing.T) {
	m, _ := newManager(t)
	ctx := context.Background()
	token, _ := m.Create(ctx, 1)
	s, _ := m.Get(ctx, token)
	m.BindCharacter(s, 7)
	if s.CharID != 7 {
		t.Fatalf("BindCharacter 后 CharID = %d", s.CharID)
	}
	if err := m.Save(ctx, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, _ := m.Get(ctx, token)
	if s2.CharID != 7 {
		t.Fatalf("Save 后重读 CharID = %d, 期望 7（断线重连恢复选角）", s2.CharID)
	}
}
