package world

import (
	"context"
	"math"
	"testing"
	"time"
)

// TestTwoDimensionalSnapshotAndMove 覆盖 V1.43 主流程切俯视后的 x/y 坐标系：
// 3/1 快照须下发二维 bounds 与 spawn，3/2 须接受 y 并按欧氏距离做防瞬移。
func TestTwoDimensionalSnapshotAndMove(t *testing.T) {
	f := newFixture(t)
	sess, _ := enterScene(t, f, "俯视侠")
	ctx := context.Background()

	snap, code, msg := f.svc.HandleEnter(ctx, sess, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("进场 code=%d (%s)", code, msg)
	}
	bounds, ok := snap["bounds"].(map[string]any)
	if !ok || bounds["y_min"] == nil || bounds["y_max"] == nil {
		t.Fatalf("快照缺二维 bounds: %+v", snap["bounds"])
	}
	spawn, ok := snap["spawn"].(map[string]any)
	if !ok || spawn["x"] == nil || spawn["y"] == nil {
		t.Fatalf("快照缺二维 spawn: %+v", snap["spawn"])
	}
	self := snap["self"].(map[string]any)
	sx, sy := self["x"].(float64), self["y"].(float64)

	// 正常二维位移：y 必须被服务端接受并回发，而不是被强制写回 ground_y。
	f.clock.Advance(time.Second)
	p, code, msg := f.svc.HandleMove(ctx, sess, map[string]any{"x": sx + 100, "y": sy - 80})
	if code != 0 {
		t.Fatalf("二维移动 code=%d (%s)", code, msg)
	}
	if p["x"] != sx+100 || p["y"] != sy-80 {
		t.Fatalf("二维坐标未生效: %+v, 期望 (%.1f, %.1f)", p, sx+100, sy-80)
	}

	// 斜向瞬移：按欧氏距离等比缩回，x/y 同比例，不得只钳 x 而放任 y 跳变。
	px, py := p["x"].(float64), p["y"].(float64)
	f.clock.Advance(time.Second)
	p2, code, _ := f.svc.HandleMove(ctx, sess, map[string]any{"x": px + 1000, "y": py + 1000})
	if code != 0 {
		t.Fatal("超速移动应被钳制而非拒绝")
	}
	allowed := 320.0*1.0 + 16.0
	dist := math.Hypot(p2["x"].(float64)-px, p2["y"].(float64)-py)
	if dist > allowed+0.5 {
		t.Fatalf("二维防瞬移失效: 位移 %.1f > 允许 %.1f", dist, allowed)
	}
	if p2["y"].(float64) == py+1000 {
		t.Fatal("y 轴被放任瞬移")
	}
}
