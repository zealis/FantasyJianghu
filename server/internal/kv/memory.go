package kv

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	val []byte
	exp time.Time // 零值 = 不过期
}

func (e memEntry) dead(now time.Time) bool {
	return !e.exp.IsZero() && now.After(e.exp)
}

// MemoryStore 为进程内实现，开发与单测默认；重启即失，不承载唯一真相。
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]memEntry
}

func NewMemory() *MemoryStore {
	m := &MemoryStore{data: make(map[string]memEntry)}
	go m.janitor()
	return m
}

func (m *MemoryStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	e, ok := m.data[key]
	m.mu.RUnlock()
	if !ok || e.dead(time.Now()) {
		return nil, ErrNotFound
	}
	return e.val, nil
}

func (m *MemoryStore) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	m.data[key] = memEntry{val: append([]byte(nil), val...), exp: exp}
	return nil
}

func (m *MemoryStore) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *MemoryStore) Expire(_ context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.data[key]
	if !ok || e.dead(time.Now()) {
		return ErrNotFound
	}
	e.exp = time.Now().Add(ttl)
	m.data[key] = e
	return nil
}

func (m *MemoryStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	e, ok := m.data[key]
	if !ok || e.dead(now) {
		var exp time.Time
		if ttl > 0 {
			exp = now.Add(ttl)
		}
		m.data[key] = memEntry{val: []byte("1"), exp: exp}
		return 1, nil
	}
	var n int64
	for _, d := range e.val {
		n = n*10 + int64(d-'0')
	}
	n++
	m.data[key] = memEntry{val: []byte(itoa(n)), exp: e.exp}
	return n, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (m *MemoryStore) janitor() {
	for range time.Tick(time.Minute) {
		now := time.Now()
		m.mu.Lock()
		for k, e := range m.data {
			if e.dead(now) {
				delete(m.data, k)
			}
		}
		m.mu.Unlock()
	}
}
