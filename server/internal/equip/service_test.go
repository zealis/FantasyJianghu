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