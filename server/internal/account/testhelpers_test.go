package account

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/gateway"
	"fantasy-jianghu/server/internal/session"
)

// fakeBinder 为测试注入的会话绑定假件（捕获 BindSession 结果）。
type fakeBinder struct{ bound *session.Session }

func (f *fakeBinder) BindSession(s *session.Session) { f.bound = s }

func newFakeConn() *fakeBinder { return &fakeBinder{} }

func withConn(ctx context.Context, f *fakeBinder) context.Context {
	return gateway.WithConn(ctx, f)
}

func mustRegister(t *testing.T, svc *Service, username, password string) string {
	t.Helper()
	if _, code, msg := svc.Register(context.Background(), username, password, ""); code != 0 {
		t.Fatalf("预置账号 %s 失败: %d %s", username, code, msg)
	}
	return username
}
