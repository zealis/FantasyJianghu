package world

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/protocol"
)

// TestTeleportUsesServerConfig 传送点交互（WS 3/9）：客户端只给 portal_id，
// 落点必须来自服务端场景配置，伪造坐标不生效，并写盘、广播 3/3。
func TestTeleportUsesServerConfig(t *testing.T) {
	f := newFixture(t)
	sc := f.svc.scenes["清河村"]
	sc.Portals = []config.ScenePortal{{
		ID: "field_gate", Name: "清河村野外", X: 468, Y: 972,
		ToX: 1500, ToY: 700,
	}}
	f.svc.scenes["清河村"] = sc
	ctx := context.Background()
	sess, selfID := enterScene(t, f, "传送侠")
	other, _ := enterScene(t, f, "旁观者")

	// 客户端塞入伪造坐标也必须被忽略。
	resp, code, msg := f.svc.HandleTeleport(ctx, sess, map[string]any{
		"portal_id": "field_gate", "x": float64(1), "y": float64(1),
	})
	if code != 0 {
		t.Fatalf("合法传送 code=%d (%s)", code, msg)
	}
	if resp["x"] != float64(1500) || resp["y"] != float64(700) {
		t.Fatalf("落点未取服务端配置（或客户端坐标生效）: %+v", resp)
	}

	// 同线他人应收到 3/3 移动广播，坐标与落点一致。
	var saw bool
	for _, fr := range f.hub.frames(other.Token) {
		if fr.Main == protocol.MainWorld && fr.Sub == protocol.SubMoveBroadcast {
			if fr.Payload["id"] == selfID &&
				fr.Payload["x"] == float64(1500) && fr.Payload["y"] == float64(700) {
				saw = true
			}
		}
	}
	if !saw {
		t.Fatalf("传送未广播 3/3 给同线玩家: %+v", f.hub.frames(other.Token))
	}

	// 离场落盘后重进，位置仍为传送落点（不被 3/2 防瞬移影响）。
	if _, code, msg := f.svc.HandleLeave(ctx, sess, map[string]any{}); code != 0 {
		t.Fatalf("离场 code=%d (%s)", code, msg)
	}
	snap, code, _ := f.svc.HandleEnter(ctx, sess, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatal("重进失败")
	}
	self := snap["self"].(map[string]any)
	if self["x"] != float64(1500) || self["y"] != float64(700) {
		t.Fatalf("传送未落盘: 重进 self=%+v", self)
	}
	_ = other
}

// TestTeleportRejectsUnknownPortal 未知 / 空 id 不得传送，防止客户端任意指定落点。
func TestTeleportRejectsUnknownPortal(t *testing.T) {
	f := newFixture(t)
	sc := f.svc.scenes["清河村"]
	sc.Portals = []config.ScenePortal{{ID: "field_gate", ToX: 1500, ToY: 700}}
	f.svc.scenes["清河村"] = sc
	ctx := context.Background()
	sess, _ := enterScene(t, f, "传送侠")
	for _, id := range []any{"", "not_exist", float64(9)} {
		if _, code, _ := f.svc.HandleTeleport(ctx, sess, map[string]any{"portal_id": id}); code == 0 {
			t.Fatalf("非法 portal_id %v 竟被受理", id)
		}
	}
}

// TestSceneValidateRejectsOutOfBoundsPortal 配置校验须拦住越界落点。
func TestSceneValidateRejectsOutOfBoundsPortal(t *testing.T) {
	cfg := testCfg()
	sc := cfg.Scenes["清河村"]
	sc.Portals = []config.ScenePortal{{ID: "bad", ToX: 99999, ToY: 700}}
	cfg.Scenes["清河村"] = sc
	if err := config.Validate(cfg); err == nil {
		t.Fatal("越界传送落点未被配置校验拦住")
	}
}
