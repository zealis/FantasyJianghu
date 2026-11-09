package kv

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
)

// runStoreTests 为内存实现与 Redis 实现共用的行为测试集。
// advance 推进时钟：内存实现用真实 sleep，Redis 用 miniredis FastForward
// （RESP 的 TTL/EXPIRE 最小精度为 1 秒，故 TTL 用例一律取 1s 后整体推进越过）。
func runStoreTests(t *testing.T, s Store, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()

	t.Run("SetGet往返", func(t *testing.T) {
		if err := s.Set(ctx, "t:round", []byte("v1"), 5*time.Second); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := s.Get(ctx, "t:round")
		if err != nil || string(got) != "v1" {
			t.Fatalf("Get = (%q, %v), 期望 (\"v1\", nil)", got, err)
		}
	})

	t.Run("未命中ErrNotFound", func(t *testing.T) {
		_, err := s.Get(ctx, "t:missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get 未命中 = %v, 期望 ErrNotFound", err)
		}
	})

	t.Run("TTL过期", func(t *testing.T) {
		if err := s.Set(ctx, "t:ttl", []byte("x"), time.Second); err != nil {
			t.Fatalf("Set: %v", err)
		}
		advance(time.Second)
		if _, err := s.Get(ctx, "t:ttl"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("过期后 Get = %v, 期望 ErrNotFound", err)
		}
	})

	t.Run("Expire续期", func(t *testing.T) {
		if err := s.Set(ctx, "t:exp", []byte("x"), time.Second); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Expire(ctx, "t:exp", time.Second); err != nil {
			t.Fatalf("Expire: %v", err)
		}
		advance(time.Second)
		if _, err := s.Get(ctx, "t:exp"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("续期后到期 Get = %v, 期望 ErrNotFound", err)
		}
	})

	t.Run("Expire键不存在", func(t *testing.T) {
		if err := s.Expire(ctx, "t:noexp", time.Second); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Expire 不存在键 = %v, 期望 ErrNotFound", err)
		}
	})

	t.Run("Del幂等", func(t *testing.T) {
		if err := s.Set(ctx, "t:del", []byte("x"), time.Second); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Del(ctx, "t:del"); err != nil {
			t.Fatalf("Del: %v", err)
		}
		if _, err := s.Get(ctx, "t:del"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Del 后 Get = %v, 期望 ErrNotFound", err)
		}
		if err := s.Del(ctx, "t:del"); err != nil {
			t.Fatalf("重复 Del = %v, 期望 nil", err)
		}
	})

	t.Run("Incr原子计数", func(t *testing.T) {
		n1, err := s.Incr(ctx, "t:incr", time.Second)
		if err != nil || n1 != 1 {
			t.Fatalf("首次 Incr = (%d, %v), 期望 (1, nil)", n1, err)
		}
		n2, _ := s.Incr(ctx, "t:incr", time.Second)
		if n2 != 2 {
			t.Fatalf("二次 Incr = %d, 期望 2", n2)
		}
		advance(time.Second)
		if _, err := s.Get(ctx, "t:incr"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("计数键到期 Get = %v, 期望 ErrNotFound", err)
		}
		n3, _ := s.Incr(ctx, "t:incr", time.Second)
		if n3 != 1 {
			t.Fatalf("到期后重新计数 = %d, 期望 1", n3)
		}
	})
}

func TestMemoryStore(t *testing.T) {
	runStoreTests(t, NewMemory(), func(d time.Duration) { time.Sleep(d + 100*time.Millisecond) })
}

func TestRedisStore(t *testing.T) {
	mr := miniredisRunT(t)
	s, err := NewRedis(mr.Addr())
	if err != nil {
		t.Fatalf("NewRedis: %v", err)
	}
	runStoreTests(t, s, mr.FastForward)
}

// miniredisRunT 起进程内 Redis（RESP 协议）供 Redis 实现真实收发。
func miniredisRunT(t *testing.T) *miniredis.Miniredis {
	return miniredis.RunT(t)
}
