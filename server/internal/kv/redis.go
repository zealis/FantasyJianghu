package kv

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisStore 基于 go-redis v9，Redis 服务端真实 RESP 协议（单测以 miniredis 承载）。
type RedisStore struct {
	c *goredis.Client
}

func NewRedis(addr string) (*RedisStore, error) {
	c := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := c.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	return &RedisStore{c: c}, nil
}

func (r *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.c.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return nil, ErrNotFound
	}
	return val, err
}

func (r *RedisStore) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	return r.c.Set(ctx, key, val, ttl).Err()
}

func (r *RedisStore) Del(ctx context.Context, key string) error {
	return r.c.Del(ctx, key).Err()
}

func (r *RedisStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	ok, err := r.c.Expire(ctx, key, ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// Incr 原子自增；首次自增（结果为 1）时设置 TTL。
func (r *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := r.c.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 && ttl > 0 {
		if err := r.c.Expire(ctx, key, ttl).Err(); err != nil {
			return n, err
		}
	}
	return n, nil
}
