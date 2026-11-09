package world

import (
	"context"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// lineTestCfg 与 testCfg 同构，但城镇容量压到 2 以便测开线边界。
func lineTestCfg() config.Data {
	cfg := testCfg()
	cfg.App.SceneCapacity = map[string]int{"town": 2, "wild": 30, "sect": 30, "activity": 30}
	return cfg
}

func newLineFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	sessions := session.NewManager(kv.NewMemory(), time.Hour)
	hub := &fakeHub{}
	clock := &fakeClock{now: time.Now()}
	svc := NewService(db.Characters, sessions, lineTestCfg(), hub)
	svc.now = clock.Now
	return &fixture{svc: svc, db: db, sessions: sessions, hub: hub, clock: clock}
}

// enterLine 让一名玩家进场并返回其会话与所在线号。
func enterLine(t *testing.T, f *fixture, name string) (*session.Session, int64, int) {
	t.Helper()
	sess, _, lineNo := enterLineWithID(t, f, name)
	return sess, 0, lineNo
}

func enterLineWithID(t *testing.T, f *fixture, name string) (*session.Session, int64, int) {
	t.Helper()
	ctx := context.Background()
	aid, _ := f.db.Accounts.CreateAccount(ctx, name+"号", "h", "")
	cid, err := f.db.Characters.CreateCharacter(ctx, aid, name, "惊雷殿", "男")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := f.sessions.Get(ctx, mustCreateToken(t, f.sessions, aid))
	if err != nil {
		t.Fatal(err)
	}
	f.sessions.BindCharacter(sess, cid)
	p, code, msg := f.svc.HandleEnter(ctx, sess, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("%s 进场 code=%d (%s)", name, code, msg)
	}
	self := p["self"].(map[string]any)
	return sess, asInt(self["id"]), int(asInt(p["line_no"]))
}

func TestAutoOpenLine(t *testing.T) {
	f := newLineFixture(t)
	_, _, l1 := enterLineWithID(t, f, "甲侠")
	_, _, l2 := enterLineWithID(t, f, "乙侠")
	_, _, l3 := enterLineWithID(t, f, "丙侠")
	if l1 != 1 || l2 != 1 {
		t.Fatalf("前两人应进 1 线: %d, %d", l1, l2)
	}
	if l3 != 2 {
		t.Fatalf("第三人应自动开 2 线, 收 %d（Focus 1）", l3)
	}
}

func TestLineListCounts(t *testing.T) {
	f := newLineFixture(t)
	sessA, _, _ := enterLineWithID(t, f, "甲侠")
	enterLineWithID(t, f, "乙侠")
	enterLineWithID(t, f, "丙侠")

	p, code, _ := f.svc.HandleLineList(context.Background(), sessA, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("line_list code=%d", code)
	}
	lines := p["lines"].([]map[string]any)
	if len(lines) != 2 {
		t.Fatalf("lines = %d 条, 期望 2", len(lines))
	}
	l1, l2 := lines[0], lines[1]
	if asInt(l1["no"]) != 1 || asInt(l1["count"]) != 2 || asInt(l1["capacity"]) != 2 {
		t.Fatalf("线 1 = %+v, 期望 no=1 count=2 capacity=2", l1)
	}
	if asInt(l2["no"]) != 2 || asInt(l2["count"]) != 1 {
		t.Fatalf("线 2 = %+v, 期望 no=2 count=1", l2)
	}
	if asInt(p["current"]) != 1 {
		t.Fatalf("current = %v, 期望 1（甲在 1 线）", p["current"])
	}
}

func TestBroadcastScopedByLine(t *testing.T) {
	f := newLineFixture(t)
	sessA, charA, _ := enterLineWithID(t, f, "甲侠")
	sessB, _, _ := enterLineWithID(t, f, "乙侠")
	sessC, _, _ := enterLineWithID(t, f, "丙侠") // 2 线
	f.clock.Advance(time.Second)

	f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 450.0, "y": 900.0})

	foundInB := false
	for _, fr := range f.hub.frames(sessB.Token) {
		if fr.Main == 3 && fr.Sub == 3 && fr.Payload["id"] == charA {
			foundInB = true
		}
	}
	if !foundInB {
		t.Fatal("同线乙未收到甲的移动广播")
	}
	for _, fr := range f.hub.frames(sessC.Token) {
		if fr.Main == 3 && fr.Sub == 3 {
			t.Fatalf("跨线丙不应收到广播: %+v（Focus 3）", fr)
		}
	}
}

func TestSwitchLine(t *testing.T) {
	f := newLineFixture(t)
	sessA, charA, _ := enterLineWithID(t, f, "甲侠")
	sessB, _, _ := enterLineWithID(t, f, "乙侠") // 1 线满
	sessC, _, _ := enterLineWithID(t, f, "丙侠") // 2 线
	f.clock.Advance(time.Second)

	// 甲从 1 线切到 2 线（未满）。
	p, code, msg := f.svc.HandleSwitchLine(context.Background(), sessA, map[string]any{"line_no": 2.0})
	if code != 0 {
		t.Fatalf("切线 code=%d (%s)", code, msg)
	}
	if asInt(p["line_no"]) != 2 {
		t.Fatalf("切线回包 line_no = %v, 期望 2", p["line_no"])
	}
	for _, pl := range p["players"].([]map[string]any) {
		if pl["id"] == charA {
			t.Fatal("切线快照不应含自己")
		}
		if asInt(pl["id"]) != charA && pl["name"] == "丙侠" {
			// 目标线快照应含丙。
			_ = pl
		}
	}
	foundC := false
	for _, pl := range p["players"].([]map[string]any) {
		if pl["name"] == "丙侠" {
			foundC = true
		}
	}
	if !foundC {
		t.Fatal("切线后快照应含目标线的丙")
	}
	// 原线乙收到甲的 3/5 离场。
	foundLeave := false
	for _, fr := range f.hub.frames(sessB.Token) {
		if fr.Main == 3 && fr.Sub == 5 && fr.Payload["id"] == charA {
			foundLeave = true
		}
	}
	if !foundLeave {
		t.Fatal("原线乙未收到甲的离场推送")
	}
	// 目标线丙收到甲的 3/6 加入。
	foundJoin := false
	for _, fr := range f.hub.frames(sessC.Token) {
		if fr.Main == 3 && fr.Sub == 6 && fr.Payload["id"] == charA {
			foundJoin = true
		}
	}
	if !foundJoin {
		t.Fatal("目标线丙未收到甲的加入推送")
	}
}

func TestSwitchLineFullReject(t *testing.T) {
	f := newLineFixture(t)
	sessA, _, lineA := enterLineWithID(t, f, "甲侠")
	enterLineWithID(t, f, "乙侠") // 1 线满（2/2）
	enterLineWithID(t, f, "丙侠") // 2 线（1/2）
	enterLineWithID(t, f, "丁侠") // 2 线满（2/2）

	// 甲切向 2 线（已满）→ code 7 且原线不变（Focus 2）。
	if _, code, _ := f.svc.HandleSwitchLine(context.Background(), sessA, map[string]any{"line_no": 2.0}); code != 7 {
		t.Fatalf("切向满线 code=%d, 期望 7", code)
	}
	p, code, _ := f.svc.HandleLineList(context.Background(), sessA, map[string]any{"map_id": "清河村"})
	if code != 0 || asInt(p["current"]) != int64(lineA) {
		t.Fatalf("拒绝切线后 current = %v, 期望原线 %d", p["current"], lineA)
	}
}

func TestSwitchLineInvalid(t *testing.T) {
	f := newLineFixture(t)
	sessA, _, _ := enterLineWithID(t, f, "甲侠")
	if _, code, _ := f.svc.HandleSwitchLine(context.Background(), sessA, map[string]any{"line_no": 0.0}); code != 3 {
		t.Fatalf("线号 0 code=%d, 期望 3", code)
	}
	if _, code, _ := f.svc.HandleSwitchLine(context.Background(), sessA, map[string]any{"line_no": 99.0}); code != 3 {
		t.Fatalf("线号 99 code=%d, 期望 3", code)
	}
	if _, code, _ := f.svc.HandleSwitchLine(context.Background(), sessA, map[string]any{}); code != 3 {
		t.Fatalf("缺线号 code=%d, 期望 3", code)
	}
}

func TestDisconnectScopedByLine(t *testing.T) {
	f := newLineFixture(t)
	sessA, _, _ := enterLineWithID(t, f, "甲侠")     // 1 线
	sessC, charC, _ := enterLineWithID(t, f, "丙侠") // 1 线（第 2 人，容量 2）
	sessB, _, _ := enterLineWithID(t, f, "乙侠")     // 2 线（1 线已满）

	// 掉线：先触发清理，再断言两端推送。
	f.svc.HandleDisconnect(context.Background(), sessC.Token)

	// 同线甲收到丙的离场推送。
	foundInA := false
	for _, fr := range f.hub.frames(sessA.Token) {
		if fr.Main == 3 && fr.Sub == 5 && fr.Payload["id"] == charC {
			foundInA = true
		}
	}
	if !foundInA {
		t.Fatal("同线甲未收到丙的离场推送")
	}

	// 跨线乙（2 线）不得收到丙（1 线）的离场推送（Focus 4）。
	for _, fr := range f.hub.frames(sessB.Token) {
		if fr.Main == 3 && fr.Sub == 5 && fr.Payload["id"] == charC {
			t.Fatal("跨线乙不应收到丙的离场推送（Focus 4）")
		}
	}
	// 丙的位置已落库（掉线即写）。
	if _, err := f.db.Characters.GetCharacter(context.Background(), charC); err != nil {
		t.Fatalf("掉线角色读回: %v", err)
	}
	// 甲再进场快照不含丙。
	p, code, _ := f.svc.HandleEnter(context.Background(), sessA, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("重进 code=%d", code)
	}
	for _, pl := range p["players"].([]map[string]any) {
		if pl["name"] == "丙侠" {
			t.Fatal("掉线者仍在快照中（幽灵）")
		}
	}
}

func TestSnapshotPlayersCarryMountPetFields(t *testing.T) {
	f := newLineFixture(t)
	enterLineWithID(t, f, "甲侠")
	sessB, _, _ := enterLineWithID(t, f, "乙侠")
	p, code, _ := f.svc.HandleEnter(context.Background(), sessB, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatal("进场失败")
	}
	for _, pl := range p["players"].([]map[string]any) {
		if _, has := pl["mount"]; !has {
			t.Fatal("快照 players 缺 mount 字段（V1.27 展示范围预留）")
		}
		if _, has := pl["pet"]; !has {
			t.Fatal("快照 players 缺 pet 字段（V1.27 展示范围预留）")
		}
		if pl["mount"] != nil || pl["pet"] != nil {
			t.Fatal("M1 无坐骑宠物系统，字段应为 null")
		}
	}
}

// asInt 兼容进程内 int64 与 JSON float64 两种数字形态。
func asInt(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
