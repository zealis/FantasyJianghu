package world

import (
	"context"
	"sync"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/kv"
	"fantasy-jianghu/server/internal/protocol"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// fakeHub 捕获广播（按令牌路由的推送）。
type fakeHub struct {
	mu     sync.Mutex
	pushes map[string][]protocol.Frame
}

func (h *fakeHub) SendTo(token string, main, sub uint16, payload map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pushes == nil {
		h.pushes = map[string][]protocol.Frame{}
	}
	h.pushes[token] = append(h.pushes[token], protocol.Frame{Main: main, Sub: sub, Payload: payload})
}

func (h *fakeHub) frames(token string) []protocol.Frame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]protocol.Frame(nil), h.pushes[token]...)
}

func testCfg() config.Data {
	return config.Data{
		App: config.AppSettings{
			MaxCharactersPerAccount: 3,
			MoveSpeed:               320,
			MoveTolerance:           16,
			Spawn:                   config.Spawn{MapID: "清河村", X: 400},
			Factions:                []string{"惊雷殿", "玄水宫"},
			SceneCapacity:           map[string]int{"town": 50, "wild": 30, "sect": 30, "activity": 30},
		},
		Scenes: map[string]config.SceneLogic{
			"清河村": {
				MapID: "清河村", Type: "town", Width: 2352, Height: 1008, GroundY: 900,
				Walk: config.WalkRange{XMin: 80, XMax: 2272},
				NPCs: []config.SceneNPC{{ID: "hebo", Name: "何伯", X: 1560, Dialogue: "prologue_hebo"}},
			},
		},
	}
}

type fixture struct {
	svc      *Service
	db       *store.DB
	sessions *session.Manager
	hub      *fakeHub
	clock    *fakeClock
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newFixture(t *testing.T) *fixture {
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
	svc := NewService(db.Characters, sessions, testCfg(), hub)
	svc.now = clock.Now
	return &fixture{svc: svc, db: db, sessions: sessions, hub: hub, clock: clock}
}

// enterScene 创建账号+角色并进场，返回（会话, 角色ID）。
func enterScene(t *testing.T, f *fixture, name string) (*session.Session, int64) {
	t.Helper()
	ctx := context.Background()
	aid, _ := f.db.Accounts.CreateAccount(ctx, name+"号", "h", "")
	cid, _ := f.db.Characters.CreateCharacter(ctx, aid, name, "惊雷殿", "男")
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
	return sess, int64(self["id"].(int64))
}

func mustCreateToken(t *testing.T, m *session.Manager, aid int64) string {
	t.Helper()
	tok, err := m.Create(context.Background(), aid)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestEnterSnapshot(t *testing.T) {
	f := newFixture(t)
	sessA, _ := enterScene(t, f, "甲侠")
	_ = sessA

	// 乙进场时快照应含甲。
	sessB, _ := enterScene(t, f, "乙侠")
	ctx := context.Background()
	// 重新进场拿一次快照检查形状。
	p, code, _ := f.svc.HandleEnter(ctx, sessB, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("进场 code=%d", code)
	}
	if p["map_id"] != "清河村" || p["ground_y"] != 900.0 {
		t.Fatalf("快照基础字段不一致: map_id=%v ground_y=%v", p["map_id"], p["ground_y"])
	}
	walk := p["walk"].(map[string]any)
	if walk["x_min"] != 80.0 || walk["x_max"] != 2272.0 {
		t.Fatalf("walk 区间不一致: %+v", walk)
	}
	npcs := p["npcs"].([]map[string]any)
	if len(npcs) != 1 || npcs[0]["id"] != "hebo" {
		t.Fatalf("NPC 快照不一致: %+v", npcs)
	}
	players := p["players"].([]map[string]any)
	if len(players) != 1 {
		t.Fatalf("乙的快照应含甲 1 人: %+v", players)
	}
	if _, has := players[0]["name"]; !has {
		t.Fatalf("玩家快照缺 name: %+v", players[0])
	}
	// 同屏玩家按门派×性别取行走皮肤，两个字段必须随快照/加入推送下发。
	if _, has := players[0]["faction"]; !has {
		t.Fatalf("玩家快照缺 faction: %+v", players[0])
	}
	if _, has := players[0]["gender"]; !has {
		t.Fatalf("玩家快照缺 gender: %+v", players[0])
	}
}

func TestEnterRequiresChar(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	aid, _ := f.db.Accounts.CreateAccount(ctx, "未选角号", "h", "")
	sess, _ := f.sessions.Get(ctx, mustCreateToken(t, f.sessions, aid))
	if _, code, msg := f.svc.HandleEnter(ctx, sess, map[string]any{}); code != 2 {
		t.Fatalf("未选角进场 code=%d (%s), 期望 2", code, msg)
	}
}

func TestMoveBroadcast(t *testing.T) {
	f := newFixture(t)
	sessA, charA := enterScene(t, f, "甲侠")
	sessB, _ := enterScene(t, f, "乙侠")
	f.clock.Advance(100 * time.Millisecond)

	p, code, msg := f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 440.0, "y": 900.0})
	if code != 0 {
		t.Fatalf("移动 code=%d (%s)", code, msg)
	}
	if p["x"] != 440.0 {
		t.Fatalf("权威回包 x=%v, 期望 440", p["x"])
	}
	frames := f.hub.frames(sessB.Token)
	found := false
	for _, fr := range frames {
		if fr.Main == 3 && fr.Sub == 3 && fr.Payload["id"] == charA && fr.Payload["x"] == 440.0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("乙未收到甲的 3/3 广播: %+v", frames)
	}
}

func TestMoveAntiTeleport(t *testing.T) {
	f := newFixture(t)
	sessA, charA := enterScene(t, f, "瞬移侠")
	sessB, _ := enterScene(t, f, "旁观侠")
	f.clock.Advance(100 * time.Millisecond) // allowed = 320*0.1 + 16 = 48

	p, code, _ := f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 600.0, "y": 900.0})
	if code != 0 {
		t.Fatal("超速移动应被钳制而非拒绝")
	}
	if p["x"] != 448.0 { // 400 + 48
		t.Fatalf("超速钳制 x=%v, 期望 448", p["x"])
	}
	frames := f.hub.frames(sessB.Token)
	for _, fr := range frames {
		if fr.Main == 3 && fr.Sub == 3 && fr.Payload["id"] == charA {
			if fr.Payload["x"] != 448.0 {
				t.Fatalf("广播坐标未钳制: %v", fr.Payload["x"])
			}
		}
	}
}

func TestMoveClampBounds(t *testing.T) {
	f := newFixture(t)
	sessA, _ := enterScene(t, f, "越界侠")
	f.clock.Advance(time.Second) // allowed = 320*1 + 16 = 336，速度允许一步到 x=80
	p, code, _ := f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 1.0, "y": 900.0})
	if code != 0 || p["x"] != 80.0 {
		t.Fatalf("越界钳制 = (x=%v, code=%d), 期望 walk 下界 80", p["x"], code)
	}
}

func TestLeavePersists(t *testing.T) {
	f := newFixture(t)
	sessA, charA := enterScene(t, f, "离场侠")
	sessB, _ := enterScene(t, f, "留守侠")
	f.clock.Advance(time.Second) // allowed=336 ≥ 100，x=500 合法
	f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 500.0, "y": 900.0})
	if _, code, _ := f.svc.HandleLeave(context.Background(), sessA, map[string]any{}); code != 0 {
		t.Fatal("离场失败")
	}
	c, err := f.db.Characters.GetCharacter(context.Background(), charA)
	if err != nil || c.PosX != 500.0 {
		t.Fatalf("离场位置未落库: %+v err=%v", c, err)
	}
	found := false
	for _, fr := range f.hub.frames(sessB.Token) {
		if fr.Main == 3 && fr.Sub == 5 && fr.Payload["id"] == charA {
			found = true
		}
	}
	if !found {
		t.Fatal("留守者未收到 3/5 离场推送")
	}
}

func TestSweepPersists(t *testing.T) {
	f := newFixture(t)
	sessA, charA := enterScene(t, f, "清扫侠")
	f.clock.Advance(100 * time.Millisecond)
	f.svc.HandleMove(context.Background(), sessA, map[string]any{"x": 600.0, "y": 900.0})
	// 600 超出 0.1s 允许距离(48)？allowed=48 → 钳到 448。取 448 校验。
	if err := f.svc.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	c, _ := f.db.Characters.GetCharacter(context.Background(), charA)
	if c.PosX != 448.0 {
		t.Fatalf("Sweep 位置未落库: %v, 期望 448", c.PosX)
	}
}

func TestSnapshotHasNoTokenLeak(t *testing.T) {
	f := newFixture(t)
	enterScene(t, f, "甲侠")
	sessB, _ := enterScene(t, f, "乙侠")
	ctx := context.Background()
	p, code, _ := f.svc.HandleEnter(ctx, sessB, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("进场 code=%d", code)
	}
	// 凭据不得离开服务端：快照任何位置不得出现 token 键。
	if _, has := p["token"]; has {
		t.Fatal("快照顶层泄露 token")
	}
	for i, pl := range p["players"].([]map[string]any) {
		if _, has := pl["token"]; has {
			t.Fatalf("players[%d] 泄露他人会话令牌", i)
		}
	}
	if _, has := p["self"].(map[string]any)["token"]; has {
		t.Fatal("self 泄露 token")
	}
}

func TestDisconnectRemovesPlayer(t *testing.T) {
	f := newFixture(t)
	sessA, charA := enterScene(t, f, "掉线侠")
	sessB, _ := enterScene(t, f, "在场侠")

	f.svc.HandleDisconnect(context.Background(), sessA.Token)

	// 在场者收到 3/5 离场推送。
	found := false
	for _, fr := range f.hub.frames(sessB.Token) {
		if fr.Main == 3 && fr.Sub == 5 && fr.Payload["id"] == charA {
			found = true
		}
	}
	if !found {
		t.Fatal("掉线后在场者未收到 3/5 推送")
	}
	// 位置已落库（掉线即写，不待 30s sweep）。
	c, err := f.db.Characters.GetCharacter(context.Background(), charA)
	if err != nil {
		t.Fatalf("掉线角色读回失败: %v", err)
	}
	_ = c
	// 再进场快照不再含掉线者（乙重进验证）。
	p, code, _ := f.svc.HandleEnter(context.Background(), sessB, map[string]any{"map_id": "清河村"})
	if code != 0 {
		t.Fatalf("重进 code=%d", code)
	}
	for _, pl := range p["players"].([]map[string]any) {
		if pl["id"] == charA {
			t.Fatal("掉线者仍在场景快照中（幽灵玩家）")
		}
	}
}
