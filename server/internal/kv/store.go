// Package kv 为会话级热数据提供存储抽象（开发技术规范 §2.2）。
// 只存令牌、幂等键等非唯一真相数据；资产类数据一律走主库。
package kv

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("kv: not found")

// Store 为最小热数据接口：Set 的 ttl 为 0 表示不过期。
// Incr 为原子自增（首次自增时设置 ttl），用于限流计数等。
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}
