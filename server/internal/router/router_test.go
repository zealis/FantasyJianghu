package router

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/session"
)

func TestRouterDispatch(t *testing.T) {
	var r Router
	called := 0
	r.Register(3, 2, func(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
		called++
		return map[string]any{"x": 1.0}, 0, ""
	})
	p, code, msg, ok := r.Dispatch(context.Background(), nil, protocol.Frame{Main: 3, Sub: 2, Seq: 1, Payload: map[string]any{}})
	if !ok || code != 0 || msg != "" || p["x"] != 1.0 || called != 1 {
		t.Fatalf("Dispatch = (%+v, %d, %q, %v) called=%d", p, code, msg, ok, called)
	}
	if _, _, _, ok := r.Dispatch(context.Background(), nil, protocol.Frame{Main: 99, Sub: 0, Seq: 2, Payload: map[string]any{}}); ok {
		t.Fatal("未登记主码应返回 ok=false")
	}
	if !r.Registered(3, 2) || r.Registered(99, 0) {
		t.Fatal("Registered 判定不一致")
	}
}
