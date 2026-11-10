package equip

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

func newEnv(t *testing.T, capacity int) (*Service, *store.DB, *session.Session) {
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
	acctID, err := db.Accounts.CreateAccount(ctx, "装备侠", "hash", "")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "剑痴", "惊雷殿", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	anchor := config.AnchorStat{HP: 1000, MP: 480, PAtk: 71, MAtk: 100, PDef: 50, MDef: 60, Speed: 100}
	cfg := config.Data{
		Items: config.ItemData{
			BagCapacity: capacity,
			Items: []config.ItemDef{
				{ID: "sword_wood", Name: "粗木剑", Category: "装备", Stack: 1, Slot: "weapon",
					Stats: map[string]int{"patk": 18}},
				{ID: "sword_iron", Name: "铁剑", Category: "装备", Stack: 1, Slot: "weapon",
					Stats: map[string]int{"patk": 30}},
				{ID: "robe_yunjin", Name: "云锦袍", Category: "装备", Stack: 1, Slot: "clothes",
					Stats: map[string]int{"pdef": 12, "hp": 60}},
				{ID: "pill", Name: "小还丹", Category: "丹药", Stack: 99},
			},
		},
		Battle: config.BattleData{
			Classes: []config.BattleClass{{Class: "法术", Factions: []string{"惊雷殿"},
				Anchors: map[string]config.AnchorStat{"20": anchor}}},
			ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
		},
	}
	return NewService(db.Items, db.Equips, db.Characters, cfg), db, &session.Session{CharID: charID}
}

func seed(t *testing.T, db *store.DB, charID int64, itemID string, count int) {
	t.Helper()
	if _, err := db.SQL().Exec(
		`INSERT INTO character_items (character_id, item_id, count, created_at) VALUES (?, ?, ?, 0)`,
		charID, itemID, count); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestWearViewRemove(t *testing.T) {
	svc, db, sess := newEnv(t, 10)
	seed(t, db, sess.CharID, "sword_wood", 1)
	ctx := context.Background()
	if _, code, msg := svc.HandleWear(ctx, sess, map[string]any{"item_id": "sword_wood"}); code != codeOK {
		t.Fatalf("穿戴应成功: %d %s", code, msg)
	}
	rows, _ := db.Items.List(ctx, sess.CharID)
	if len(rows) != 0 {
		t.Fatalf("穿戴后背包应无该剑: %v", rows)
	}
	out, code, _ := svc.HandleView(ctx, sess, map[string]any{})
	if code != codeOK {
		t.Fatalf("view code=%d", code)
	}
	slots := out["slots"].(map[string]any)
	if slots["weapon"] == nil {
		t.Fatalf("weapon 槽应已穿戴: %v", slots)
	}
	attrs := out["attrs"].(map[string]any)
	// 基线 PAtk=71（Lv1 线性外延=锚点×1/20×? low_curve 1.0 → 71*1/20? 实际外延口径见 stats；
	// 这里只断言装备加成差值：穿戴前后 patk 差 = 18。
	baseSvc, baseDB, baseSess := newEnv(t, 10)
	baseOut, _, _ := baseSvc.HandleView(ctx, baseSess, map[string]any{})
	baseAttrs := baseOut["attrs"].(map[string]any)
	if attrs["patk"].(int)-baseAttrs["patk"].(int) != 18 {
		t.Fatalf("patk 加成应 +18: %v vs %v", attrs["patk"], baseAttrs["patk"])
	}
	_ = baseDB
	// 重复穿戴拒绝。
	seed(t, db, sess.CharID, "sword_wood", 1)
	if _, code, _ := svc.HandleWear(ctx, sess, map[string]any{"item_id": "sword_wood"}); code != codeBadState {
		t.Fatalf("重复穿戴应拒绝: %d", code)
	}
	// 卸下回包。
	if _, code, msg := svc.HandleRemove(ctx, sess, map[string]any{"slot": "weapon"}); code != codeOK {
		t.Fatalf("卸下应成功: %d %s", code, msg)
	}
	rows, _ = db.Items.List(ctx, sess.CharID)
	if len(rows) != 2 { // 回包的剑 + 重复穿戴种子的那把
		t.Fatalf("卸下应回包: %v", rows)
	}
	if _, code, _ := svc.HandleRemove(ctx, sess, map[string]any{"slot": "weapon"}); code != codeBadState {
		t.Fatalf("空槽卸下应拒绝: %d", code)
	}
}

func TestWearSwapBagFullRejectsWhole(t *testing.T) {
	svc, db, sess := newEnv(t, 1)
	seed(t, db, sess.CharID, "sword_wood", 1)
	seed(t, db, sess.CharID, "sword_iron", 1)
	seed(t, db, sess.CharID, "pill", 1)
	ctx := context.Background()
	if _, code, _ := svc.HandleWear(ctx, sess, map[string]any{"item_id": "sword_wood"}); code != codeOK {
		t.Fatalf("首次穿戴应成功: %d", code)
	}
	// 同槽换装：旧木剑回包需 1 格，但背包剩 pill 1 格 + 回包 1 = 2 > 容量 1 → 整单拒绝。
	if _, code, msg := svc.HandleWear(ctx, sess, map[string]any{"item_id": "sword_iron"}); code != codeBagFull {
		t.Fatalf("换装容量不足应拒绝: %d %s", code, msg)
	}
	equipped, _ := db.Equips.List(ctx, sess.CharID)
	if equipped["weapon"] != "sword_wood" {
		t.Fatalf("拒绝不应变更槽位: %v", equipped)
	}
	rows, _ := db.Items.List(ctx, sess.CharID)
	if len(rows) != 2 { // 铁剑 + pill 仍在包
		t.Fatalf("拒绝不应扣物品: %v", rows)
	}
}

func TestWearRejects(t *testing.T) {
	svc, db, sess := newEnv(t, 10)
	ctx := context.Background()
	if _, code, _ := svc.HandleWear(ctx, sess, map[string]any{"item_id": "ghost"}); code != codeNotFound {
		t.Fatalf("不存在应 NotFound: %d", code)
	}
	seed(t, db, sess.CharID, "pill", 1)
	if _, code, msg := svc.HandleWear(ctx, sess, map[string]any{"item_id": "pill"}); code != codeBadState || msg == "" {
		t.Fatalf("非装备应拒绝且原因可读: %d %s", code, msg)
	}
	if _, code, _ := svc.HandleWear(ctx, sess, map[string]any{"item_id": "sword_wood"}); code != codeBadState {
		t.Fatalf("背包无该物品应拒绝: %d", code)
	}
	if _, code, _ := svc.HandleWear(ctx, nil, map[string]any{"item_id": "sword_wood"}); code != codeUnauthorized {
		t.Fatalf("未鉴权应拒绝: %d", code)
	}
	if _, code, _ := svc.HandleRemove(ctx, sess, map[string]any{"slot": "wing"}); code != codeBadState {
		t.Fatalf("非法槽应拒绝: %d", code)
	}
}
// suitEnv 构造一套带品质共鸣配置的环境：五件完美物理装备 + 断岳剑宗微调。
func suitEnv(t *testing.T) (*Service, *store.DB, *session.Session) {
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
	acctID, err := db.Accounts.CreateAccount(ctx, "共鸣侠", "hash", "")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "剑痴", "断岳剑宗", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	anchor := config.AnchorStat{HP: 1000, MP: 480, PAtk: 100, MAtk: 100, PDef: 50, MDef: 60, Speed: 100}
	// 五槽各一件完美装备，数值统一 +10 物攻以便核对百分比是否乘在装备之后。
	items := []config.ItemDef{
		{ID: "w1", Name: "武器", Category: "装备", Stack: 1, Slot: "weapon", Quality: "完美",
			Stats: map[string]int{"patk": 10}},
		{ID: "c1", Name: "衣服", Category: "装备", Stack: 1, Slot: "clothes", Quality: "完美",
			Stats: map[string]int{"pdef": 10}},
		{ID: "h1", Name: "帽子", Category: "装备", Stack: 1, Slot: "hat", Quality: "完美",
			Stats: map[string]int{"pdef": 10}},
		{ID: "n1", Name: "项链", Category: "装备", Stack: 1, Slot: "necklace", Quality: "完美",
			Stats: map[string]int{"mdef": 10}},
		{ID: "b1", Name: "鞋子", Category: "装备", Stack: 1, Slot: "boots", Quality: "完美",
			Stats: map[string]int{"speed": 10}},
	}
	cfg := config.Data{
		Items: config.ItemData{BagCapacity: 20, Items: items},
		Factions: []config.FactionInfo{
			{Name: "断岳剑宗", Category: "物理", Color: "朱橙", Residence: "高天崖", Weapon: "剑",
				Feature: "暴击", Secret: "断岳剑经"},
		},
		Battle: config.BattleData{
			Classes: []config.BattleClass{{Class: "物理", Factions: []string{"断岳剑宗"},
				Anchors: map[string]config.AnchorStat{"20": anchor}}},
			ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
		},
		Suit: config.SuitData{
			AllAttrs:     []string{"hp", "mp", "patk", "matk", "pdef", "mdef", "speed"},
			QualityOrder: map[string]int{"凡品": 1, "良品": 2, "精品": 3, "极品": 4, "完美": 5},
			Tier1AllPct:  []float64{1, 2, 3, 4, 5},
			Tier2MainPct: []float64{5, 6, 7, 8, 9},
			Systems: map[string]config.SuitSystem{
				"物理": {Main: []string{"patk"}, Sub: "pcrit", SubPct: []float64{1.5, 2, 2.5, 3, 3.5}},
			},
			FactionTweaks: map[string]config.FactionTweak{
				"断岳剑宗": {Attr: "pcrit", Pct: []float64{1, 1.5, 2, 2.5, 3}},
			},
		},
	}
	return NewService(db.Items, db.Equips, db.Characters, cfg), db, &session.Session{CharID: charID}
}

// TestViewSuitResonanceTier2 五件同品质 → 二档：全属性 +5%、物攻 +9%、暴击 3.5%+3%=6.5%（配置表 §4）。
func TestViewSuitResonanceTier2(t *testing.T) {
	svc, db, sess := suitEnv(t)
	ctx := context.Background()
	// 拉到锚点等级 20：否则 1 级会被幂律外延到很低，断言数值不便核对。
	if _, err := db.SQL().Exec(`UPDATE characters SET level = 20 WHERE id = ?`, sess.CharID); err != nil {
		t.Fatalf("设置等级: %v", err)
	}
	for _, id := range []string{"w1", "c1", "h1", "n1", "b1"} {
		seed(t, db, sess.CharID, id, 1)
		if _, code, msg := svc.HandleWear(ctx, sess, map[string]any{"item_id": id}); code != codeOK {
			t.Fatalf("穿戴 %s 失败: %d %s", id, code, msg)
		}
	}
	out, code, msg := svc.HandleView(ctx, sess, map[string]any{})
	if code != codeOK {
		t.Fatalf("view code=%d %s", code, msg)
	}
	suit, ok := out["suit"].(map[string]any)
	if !ok {
		t.Fatal("视图缺 suit 明细")
	}
	if suit["tier"].(int) != 2 || suit["quality"].(string) != "完美" || suit["count"].(int) != 5 {
		t.Fatalf("共鸣应为二档/完美/5 件: %+v", suit)
	}
	if suit["tweak_pct"].(float64) != 3 {
		t.Fatalf("断岳剑宗微调应为 3%%: %+v", suit["tweak_pct"])
	}
	attrs := out["attrs"].(map[string]any)
	// 物攻：(100 基线 + 10 装备) × 1.09（二档主项，取代一档全属性）= 119.9 → 120
	if got := attrs["patk"].(int); got != 120 {
		t.Fatalf("物攻应为 120（含装备与共鸣），实际 %d", got)
	}
	// 暴击：本测试未配 formula.crit.base_rate，基线为 0；副项 3.5% + 微调 3% = 6.5% → 650 万分制。
	if got := attrs["crit"].(int); got != 650 {
		t.Fatalf("暴击应为 650 万分制（6.5%%），实际 %d", got)
	}
	// 气血：二档不叠一档全属性 → 保持 1000
	if got := attrs["hp"].(int); got != 1000 {
		t.Fatalf("二档取代一档，气血应保持 1000，实际 %d", got)
	}
}
