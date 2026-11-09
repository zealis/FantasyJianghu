// Package router 为主码/子码到处理器的路由（登记制，§5.1：新增系统先登记后开发）。
package router

import (
	"context"
	"sync"

	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/session"
)

// HandlerFunc 返回（回包负载，错误码，错误信息）；错误码 0 = OK。
type HandlerFunc func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string)

type Router struct {
	mu sync.RWMutex
	h  map[uint32]HandlerFunc
}

func frameKey(main, sub uint16) uint32 { return uint32(main)<<16 | uint32(sub) }

func (r *Router) Register(main, sub uint16, h HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.h == nil {
		r.h = make(map[uint32]HandlerFunc)
	}
	r.h[frameKey(main, sub)] = h
}

// Dispatch 路由一帧；ok=false 表示主码/子码未登记。
func (r *Router) Dispatch(ctx context.Context, sess *session.Session, f protocol.Frame) (map[string]any, int, string, bool) {
	r.mu.RLock()
	h, ok := r.h[frameKey(f.Main, f.Sub)]
	r.mu.RUnlock()
	if !ok {
		return nil, 0, "", false
	}
	p, code, msg := h(ctx, sess, f.Payload)
	return p, code, msg, true
}

// Registered 供网关判断是否登记（重放丢弃发生在登记请求上）。
func (r *Router) Registered(main, sub uint16) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.h[frameKey(main, sub)]
	return ok
}
