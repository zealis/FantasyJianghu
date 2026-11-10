package quest

import (
	"context"
	"errors"
	"testing"
	"time"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// fakePusher 捕获 11/4 推送（验证进度推送不依赖网关）。
type fakePusher struct {
	events []map[string]any
}

func (f *fakePusher) SendToChar(_ int64, main, sub uint16, payload map[string]any) bool {
	payload["_main"] = main
	payload["_sub"] = sub
	f.events = append(f.events, payload)
	return true
}

// testEnv 构造内存库 + 最小配置 + 一个角色。
type testEnv struct {
	svc    *Service
	pusher *fakePusher
	db     *store.DB
	charID int64
	cfg    config.Data
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	acctID, err := db.Accounts.CreateAccount(ctx, "任务侠", "hash", "")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "雷震子", "惊雷殿", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	cfg := config.Data{
		Scenes: map[string]config.SceneLogic{
			"清河村": {MapID: "清河村", NPCs: []config.SceneNPC{{ID: "hebo", Name: "何伯"}, {ID: "tieshan", Name: "教头"}}},
		},
		Quests: config.QuestData{
			DailyLimits: map[string]int{"normal": 2, "sect": 1, "dungeon": 1},
			Quests: []config.QuestDef{
				{
					ID: "main_001", Name: "归村", Type: "main", Giver: config.QuestGiver{MapID: "清河村", NPCID: "hebo"},
					MinLevel: 1, Desc: "问何伯",
					Objectives: []config.QuestObjective{{Kind: "talk", Target: "hebo", Count: 1}},
					Rewards: config.QuestRewards{Exp: 100, Xiuwei: 10, Gold: 500,
						Items: []config.QuestItemReward{{ID: "pill", Count: 3}}},
				},
				{
					ID: "main_002", Name: "拜师", Type: "main", Giver: config.QuestGiver{MapID: "清河村", NPCID: "tieshan"},
					MinLevel: 1, Prev: "main_001", Desc: "问教头",
					Objectives: []config.QuestObjective{{Kind: "kill", Target: "muzhuang", Count: 2}},
					Rewards:    config.QuestRewards{Exp: 50, Gold: 100},
				},
				{
					ID: "daily_n_001", Name: "巡逻", Type: "daily_normal", Giver: config.QuestGiver{MapID: "清河村", NPCID: "hebo"},
					MinLevel: 1, Desc: "巡逻",
					Objectives: []config.QuestObjective{{Kind: "visit", Target: "清河村", Count: 1}},
					Rewards:    config.QuestRewards{Exp: 10, Gold: 20},
				},
			},
		},
		Items: config.ItemData{
			BagCapacity: 3,
			Items: []config.ItemDef{
				{ID: "pill", Name: "小还丹", Category: "丹药", Stack: 2},
				{ID: "talisman", Name: "护符", Category: "法宝", Stack: 1},
			},
		},
	}
	pusher := &fakePusher{}
	svc := NewService(db.Quests, db.Characters, db.Items, db.Equips, cfg, pusher, 4)
	fixed := time.Date(2026, 11, 9, 10, 0, 0, 0, cst)
	svc.SetClock(func() time.Time { return fixed })
	return &testEnv{svc: svc, pusher: pusher, db: db, charID: charID, cfg: cfg}
}

func (e *testEnv) sess() *session.Session {
	return &session.Session{Token: "tok", AccountID: 1, CharID: e.charID}
}

func TestAcceptProgressSubmit(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// 接取主线。
	if _, code, msg := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code != 0 {
		t.Fatalf("接取失败: code=%d msg=%s", code, msg)
	}
	// 重复接取应拒绝。
	if _, code, _ := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code == 0 {
		t.Fatal("重复接取应被拒绝")
	}
	// 未完成不得交付。
	if _, code, _ := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code == 0 {
		t.Fatal("目标未完成时交付应被拒绝")
	}
	// 对话推进 → 可交付，且推送 11/4。
	if err := env.svc.RecordProgress(ctx, env.charID, "talk", "hebo", 1); err != nil {
		t.Fatalf("RecordProgress: %v", err)
	}
	if len(env.pusher.events) != 1 {
		t.Fatalf("应推送 1 次进度，收到 %d", len(env.pusher.events))
	}
	if env.pusher.events[0]["_main"] != uint16(11) || env.pusher.events[0]["state"] != "completable" {
		t.Fatalf("推送内容不符: %+v", env.pusher.events[0])
	}
	// 交付发奖。
	res, code, msg := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_001"})
	if code != 0 {
		t.Fatalf("交付失败: code=%d msg=%s", code, msg)
	}
	rewards := res["rewards"].(map[string]any)
	if rewards["gold"].(int64) != 500 || rewards["exp"].(int64) != 100 {
		t.Fatalf("奖励不符: %+v", rewards)
	}
	// 钱包与背包落库。
	wallet, err := env.db.Items.WalletOf(ctx, env.charID)
	if err != nil || wallet.Gold != 500 || wallet.Xiuwei != 10 {
		t.Fatalf("钱包 = %+v, err=%v", wallet, err)
	}
	rows, err := env.db.Items.List(ctx, env.charID)
	if err != nil || len(rows) != 2 { // 3 个丹药 / 单格 2 = 两格
		t.Fatalf("背包 = %+v, err=%v（期望两格）", rows, err)
	}
	if rows[0].Count != 2 || rows[1].Count != 1 {
		t.Fatalf("堆叠分配不符: %+v", rows)
	}
	c, _ := env.db.Characters.GetCharacter(ctx, env.charID)
	if c.Exp != 100 {
		t.Fatalf("经验 = %d, 期望 100", c.Exp)
	}
	// 重复交付拒绝。
	if _, code, _ := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code == 0 {
		t.Fatal("重复交付应被拒绝")
	}
}

func TestPrevQuestGate(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	if _, code, _ := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_002"}); code == 0 {
		t.Fatal("前置未完成时接取应被拒绝")
	}
	// 完成前置后放行。
	_, _, _ = env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_001"})
	_ = env.svc.RecordProgress(ctx, env.charID, "talk", "hebo", 1)
	if _, code, _ := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code != 0 {
		t.Fatal("前置交付应成功")
	}
	if _, code, msg := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_002"}); code != 0 {
		t.Fatalf("前置完成后应可接取: code=%d msg=%s", code, msg)
	}
}

func TestDailyLimit(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// 上限 2：接 1 个 → 交付；再接 1 个 → 交付；第三次被拒。
	for i := 0; i < 2; i++ {
		if _, code, msg := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "daily_n_001"}); code != 0 {
			t.Fatalf("第 %d 次接取失败: %s", i+1, msg)
		}
		_ = env.svc.RecordProgress(ctx, env.charID, "visit", "清河村", 1)
		if _, code, msg := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "daily_n_001"}); code != 0 {
			t.Fatalf("第 %d 次交付失败: %s", i+1, msg)
		}
	}
	if _, code, msg := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "daily_n_001"}); code == 0 {
		t.Fatalf("超过每日上限应拒绝，实际通过（%s）", msg)
	}
	st, code, _ := env.svc.HandleDaily(ctx, env.sess(), nil)
	if code != 0 {
		t.Fatal("日常状态查询失败")
	}
	if d := st["normal"].(dailyView); d.Used != 2 || d.Limit != 2 {
		t.Fatalf("日常状态 = %+v（两次交付后应为 2/2）", d)
	}
}

func TestBagFullRejectsWholeGrant(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	// 首次合法交付：3 个丹药 → 2 格（单格 2）。
	_, _, _ = env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_001"})
	_ = env.svc.RecordProgress(ctx, env.charID, "talk", "hebo", 1)
	if _, code, _ := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_001"}); code != 0 {
		t.Fatal("首次交付应成功")
	}
	// 再占 1 格（不可堆叠的护符，stack=1），使容量 3 用满。
	if err := insertItem(ctx, env.db, env.charID, "talisman", 1); err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	// 追加一个"奖励全新物品"的任务：容量已满 → 整单拒绝。
	env.cfg.Quests.Quests = append(env.cfg.Quests.Quests, config.QuestDef{
		ID: "main_003", Name: "满载", Type: "main", Giver: config.QuestGiver{MapID: "清河村", NPCID: "hebo"},
		MinLevel: 1, Desc: "满载测试", Prev: "main_001",
		Objectives: []config.QuestObjective{{Kind: "talk", Target: "hebo", Count: 1}},
		Rewards:    config.QuestRewards{Exp: 999, Gold: 9999, Items: []config.QuestItemReward{{ID: "talisman", Count: 1}}},
	})
	env.svc.cfg = env.cfg
	if _, code, _ := env.svc.HandleAccept(ctx, env.sess(), map[string]any{"quest_id": "main_003"}); code != 0 {
		t.Fatal("接取应成功")
	}
	_ = env.svc.RecordProgress(ctx, env.charID, "talk", "hebo", 1)
	_, code, msg := env.svc.HandleSubmit(ctx, env.sess(), map[string]any{"quest_id": "main_003"})
	if code != codeBagFull {
		t.Fatalf("背包满应返回 %d，实际 %d（%s）", codeBagFull, code, msg)
	}
	// 不部分发放：经验、金币均未变，任务仍在进行中。
	c, _ := env.db.Characters.GetCharacter(ctx, env.charID)
	if c.Exp != 100 {
		t.Fatalf("拒绝后经验被改动: %d", c.Exp)
	}
	wallet, _ := env.db.Items.WalletOf(ctx, env.charID)
	if wallet.Gold != 500 {
		t.Fatalf("拒绝后金币被改动: %d", wallet.Gold)
	}
	list, _, _ := env.svc.HandleList(ctx, env.sess(), nil)
	found := false
	for _, it := range list["quests"].([]questView) {
		if it.ID == "main_003" && it.State == "completable" {
			found = true
		}
	}
	if !found {
		t.Fatal("拒绝交付后任务应仍为可交付（未完成）")
	}
}

// insertItem 直接写一格物品（模拟既有背包内容）。
func insertItem(ctx context.Context, db *store.DB, charID int64, itemID string, count int) error {
	_, err := db.SQL().ExecContext(ctx,
		`INSERT INTO character_items (character_id, item_id, count, created_at) VALUES (?, ?, ?, 0)`,
		charID, itemID, count)
	return err
}

func TestSubmitRejectsUnknownRow(t *testing.T) {
	env := newEnv(t)
	_, err := env.db.Quests.Submit(context.Background(), store.SubmitRequest{CharacterID: env.charID, QuestRowID: 999})
	if !errors.Is(err, store.ErrQuestState) {
		t.Fatalf("err = %v, 期望 ErrQuestState", err)
	}
}
