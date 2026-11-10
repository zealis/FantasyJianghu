package item

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

// newEnv 内存库 + 最小物品配置 + 一个角色（M2c 切片一：5/2 使用物品）。
func newEnv(t *testing.T) (*Service, *store.DB, *session.Session) {
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
	acctID, err := db.Accounts.CreateAccount(ctx, "物品侠", "hash", "")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "丹痴", "惊雷殿", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	cfg := config.Data{Items: config.ItemData{
		BagCapacity: 10,
		Items: []config.ItemDef{
			{ID: "exp_dan", Name: "经验丹", Category: "丹药", Stack: 99, Quality: "良品", Sell: 20,
				Desc: "使用得经验 100", Use: &config.ItemUseDef{Kind: "exp", Value: 100}},
			{ID: "pill", Name: "小还丹", Category: "丹药", Stack: 99, Sell: 5},
			{ID: "gold_bag", Name: "金币袋", Category: "凭证", Stack: 99, Sell: 0,
				Use: &config.ItemUseDef{Kind: "gold", Value: 50}},
		},
	}}
	return NewService(db.Items, cfg), db, &session.Session{CharID: charID}
}

func seed(t *testing.T, db *store.DB, charID int64, itemID string, count int) {
	t.Helper()
	if _, err := db.SQL().Exec(
		`INSERT INTO character_items (character_id, item_id, count, created_at) VALUES (?, ?, ?, 0)`,
		charID, itemID, count); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func expOf(t *testing.T, db *store.DB, charID int64) int64 {
	t.Helper()
	var exp int64
	if err := db.SQL().QueryRow(`SELECT exp FROM characters WHERE id = ?`, charID).Scan(&exp); err != nil {
		t.Fatalf("read exp: %v", err)
	}
	return exp
}

func TestUseExpSingleTransaction(t *testing.T) {
	svc, db, sess := newEnv(t)
	seed(t, db, sess.CharID, "exp_dan", 2)
	before := expOf(t, db, sess.CharID)
	out, code, msg := svc.HandleUse(context.Background(), sess, map[string]any{"item_id": "exp_dan"})
	if code != codeOK || msg != "" {
		t.Fatalf("使用应成功: code=%d msg=%s", code, msg)
	}
	if got := expOf(t, db, sess.CharID); got != before+100 {
		t.Fatalf("exp = %d, 期望 %d", got, before+100)
	}
	if out["remaining"] != 1 {
		t.Fatalf("remaining = %v, 期望 1", out["remaining"])
	}
	rows, err := db.Items.List(context.Background(), sess.CharID)
	if err != nil || len(rows) != 1 || rows[0].Count != 1 {
		t.Fatalf("扣格后应剩 1 格 count=1: %v %v", rows, err)
	}
	// 经验类不写货币流水（流水只记货币）。
	var logs int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM currency_logs WHERE character_id = ?`, sess.CharID).Scan(&logs); err != nil {
		t.Fatal(err)
	}
	if logs != 0 {
		t.Fatalf("经验使用不应写货币流水: %d", logs)
	}
}

func TestUseGoldWritesCurrencyLog(t *testing.T) {
	svc, db, sess := newEnv(t)
	seed(t, db, sess.CharID, "gold_bag", 1)
	out, code, _ := svc.HandleUse(context.Background(), sess, map[string]any{"item_id": "gold_bag"})
	if code != codeOK {
		t.Fatalf("code=%d", code)
	}
	wallet := out["wallet"].(map[string]any)
	if wallet["gold"] != int64(50) {
		t.Fatalf("gold = %v, 期望 50", wallet["gold"])
	}
	var reason, ref string
	var delta int64
	if err := db.SQL().QueryRow(
		`SELECT reason, ref_id, delta FROM currency_logs WHERE character_id = ? ORDER BY id DESC LIMIT 1`,
		sess.CharID).Scan(&reason, &ref, &delta); err != nil {
		t.Fatalf("流水缺失: %v", err)
	}
	if reason != "item_use" || ref != "item:gold_bag" || delta != 50 {
		t.Fatalf("流水不符: %s %s %d", reason, ref, delta)
	}
	// 归零删行：List 不再含该格。
	rows, _ := db.Items.List(context.Background(), sess.CharID)
	if len(rows) != 0 {
		t.Fatalf("归零应删行: %v", rows)
	}
}

func TestUseRejects(t *testing.T) {
	svc, db, sess := newEnv(t)
	cases := []struct {
		name    string
		payload map[string]any
		seedIt  bool
		code    int
	}{
		{"物品不存在", map[string]any{"item_id": "ghost"}, false, codeNotFound},
		{"不可使用", map[string]any{"item_id": "pill"}, true, codeBadState},
		{"数量不足", map[string]any{"item_id": "exp_dan"}, false, codeBadState},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.seedIt {
				seed(t, db, sess.CharID, "pill", 1)
			}
			_, code, msg := svc.HandleUse(context.Background(), sess, c.payload)
			if code != c.code || msg == "" {
				t.Fatalf("code=%d msg=%q, 期望 code=%d 且原因可读", code, msg, c.code)
			}
		})
	}
	// 拒绝路径不扣物品、不加经验。
	seed(t, db, sess.CharID, "exp_dan", 1)
	before := expOf(t, db, sess.CharID)
	if _, code, _ := svc.HandleUse(context.Background(), sess, map[string]any{"item_id": "pill"}); code != codeBadState {
		t.Fatalf("code=%d", code)
	}
	rows, _ := db.Items.List(context.Background(), sess.CharID)
	if len(rows) != 2 {
		t.Fatalf("拒绝不应扣格: %v", rows)
	}
	if expOf(t, db, sess.CharID) != before {
		t.Fatalf("拒绝不应加经验")
	}
}

func TestUseUnauthorized(t *testing.T) {
	svc, _, _ := newEnv(t)
	if _, code, _ := svc.HandleUse(context.Background(), nil, map[string]any{"item_id": "exp_dan"}); code != codeUnauthorized {
		t.Fatalf("code=%d", code)
	}
}

func TestListCarriesQualityDescUse(t *testing.T) {
	svc, db, sess := newEnv(t)
	seed(t, db, sess.CharID, "exp_dan", 1)
	seed(t, db, sess.CharID, "pill", 1)
	out, code, _ := svc.HandleList(context.Background(), sess, map[string]any{})
	if code != codeOK {
		t.Fatalf("code=%d", code)
	}
	items := out["items"].([]map[string]any)
	byID := map[string]map[string]any{}
	for _, it := range items {
		byID[it["id"].(string)] = it
	}
	if byID["exp_dan"]["quality"] != "良品" || byID["exp_dan"]["desc"] == "" || byID["exp_dan"]["use"] == nil {
		t.Fatalf("经验丹视图缺品质/描述/use: %v", byID["exp_dan"])
	}
	if byID["pill"]["quality"] != "凡品" || byID["pill"]["use"] != nil {
		t.Fatalf("小还丹应凡品且不可使用: %v", byID["pill"])
	}
}