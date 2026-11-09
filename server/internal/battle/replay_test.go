package battle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

func fullCfg(t *testing.T) config.BattleData {
	data, err := config.Load(packViaTool(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return data.Battle
}

func mkPlayer(id int64, faction string, cfg config.BattleData, skills []config.SkillDef) *Unit {
	s, err := DerivedStats(20, faction, cfg)
	if err != nil {
		panic(err)
	}
	return &Unit{ID: id, Name: "侠客", Side: 0, Level: 20, Faction: faction,
		BaseStats: s, Stats: s, HP: s.HP, MP: s.MP, Skills: skills, Alive: true}
}

func mkMonster(id int64, cfg config.BattleData, mid string) *Unit {
	for _, m := range cfg.Monsters {
		if m.ID == mid {
			u := &Unit{ID: id, Name: m.Name, Side: 1, Level: m.Level,
				BaseStats: UnitStats{HP: m.Stats.HP, MP: m.Stats.MP, PAtk: m.Stats.PAtk, MAtk: m.Stats.MAtk,
					PDef: m.Stats.PDef, MDef: m.Stats.MDef, Speed: m.Stats.Speed},
				Stats: UnitStats{HP: m.Stats.HP, MP: m.Stats.MP, PAtk: m.Stats.PAtk, MAtk: m.Stats.MAtk,
					PDef: m.Stats.PDef, MDef: m.Stats.MDef, Speed: m.Stats.Speed},
				Alive: true}
			return u
		}
	}
	panic("monster not found: " + mid)
}

// scriptedRun 跑一场脚本化战斗返回全部事件。
func scriptedRun(cfg config.BattleData, seed int64) []Event {
	player := mkPlayer(1, "惊雷殿", cfg, cfg.Skills)
	monster := mkMonster(2, cfg, "yelinang")
	b := NewBattle(seed, []*Unit{player}, []*Unit{monster}, cfg)
	var evs []Event
	for !b.Over && b.Turn < 30 {
		b.SubmitCmd(1, Cmd{Type: "skill", SkillID: "tianlei_po"})
		b.SubmitCmd(2, Cmd{Type: "attack"})
		evs = append(evs, b.ResolveTurn()...)
	}
	return evs
}

func TestSeedReplayIdentical(t *testing.T) {
	cfg := fullCfg(t)
	e1 := scriptedRun(cfg, 20260109)
	e2 := scriptedRun(cfg, 20260109)
	j1, _ := json.Marshal(e1)
	j2, _ := json.Marshal(e2)
	if string(j1) != string(j2) {
		t.Fatalf("同种子重演事件流不一致（复盘口径 Focus 4）:\n%s\n---\n%s", j1, j2)
	}
	if len(e1) < 2 {
		t.Fatalf("事件流过短: %d", len(e1))
	}
	// 不同种子应不同（抽查首个伤害值）。
	e3 := scriptedRun(cfg, 777)
	j3, _ := json.Marshal(e3)
	if string(j1) == string(j3) {
		t.Fatal("不同种子事件流不应完全一致")
	}
}

func TestBattleServiceEnterAndFrame(t *testing.T) {
	cfg := fullCfg(t)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	m := NewManager(cfg, db.Battles)
	var frames []captured
	m.SetFrameHook(func(d FrameDelivery) {
		frames = append(frames, captured{d.TurnVersion, d.Events})
	})
	svc := NewService(m, db.Characters, fullData(cfg))

	// 玩家会话（token→unitID 由服务端映射）。
	p := mkPlayer(1, "惊雷殿", cfg, cfg.Skills)
	svc.testAttach("tokP", p)
	svc.testAttachMonster("muzhuang")

	pEnter, code, msg := svc.HandleEnter(context.Background(), sessionFor("tokP"), map[string]any{"monster_id": "muzhuang"})
	if code != 0 {
		t.Fatalf("进战斗 code=%d (%s)", code, msg)
	}
	if pEnter["battle_id"] == nil || pEnter["enemy"] == nil {
		t.Fatalf("进战斗快照不完整: %+v", pEnter)
	}
	// 指令台秘籍列表与 units[]（与 4/7 同构）必须随 4/1 下发。
	if skills, ok := pEnter["skills"].([]map[string]any); !ok || len(skills) == 0 {
		t.Fatalf("进战斗快照缺可用秘籍: %+v", pEnter["skills"])
	}
	if units, ok := pEnter["units"].([]map[string]any); !ok || len(units) != 2 {
		t.Fatalf("进战斗快照缺 units[]: %+v", pEnter["units"])
	}
	// 提交双方指令 → 帧广播。
	pCmd, code, _ := svc.HandleCmd(context.Background(), sessionFor("tokP"), map[string]any{"cmd": "skill", "skill_id": "tianlei_po"})
	if code != 0 {
		t.Fatalf("指令 code=%d", code)
	}
	// 怪物由服务端托管（单人参战：玩家指令收齐即结算——怪物侧走托管路径）。
	_ = pCmd
	deadline := time.Now().Add(2 * time.Second)
	for len(frames) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(frames) == 0 {
		t.Fatal("未收到战斗帧")
	}
}
