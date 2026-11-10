package item

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/store"
)

func walletGoldOf(t *testing.T, db *store.DB, charID int64) int64 {
	t.Helper()
	var gold int64
	if err := db.SQL().QueryRow(
		`SELECT gold FROM character_wallets WHERE character_id = ?`, charID).Scan(&gold); err != nil {
		t.Fatalf("读金币: %v", err)
	}
	return gold
}

func remainingOf(t *testing.T, db *store.DB, charID int64, itemID string) int {
	t.Helper()
	var n int
	err := db.SQL().QueryRow(
		`SELECT COALESCE(SUM(count), 0) FROM character_items WHERE character_id = ? AND item_id = ?`,
		charID, itemID).Scan(&n)
	if err != nil {
		t.Fatalf("读数量: %v", err)
	}
	return n
}

func TestSellConsumesAndAddsGold(t *testing.T) {
	svc, db, sess := newEnv(t)
	seed(t, db, sess.CharID, "pill", 3)
	out, code, msg := svc.HandleSell(context.Background(), sess,
		map[string]any{"item_id": "pill", "count": float64(2)})
	if code != codeOK {
		t.Fatalf("出售应成功: code=%d msg=%s", code, msg)
	}
	if got := walletGoldOf(t, db, sess.CharID); got != 10 {
		t.Fatalf("金币应为 5×2=10，实际 %d", got)
	}
	if got := remainingOf(t, db, sess.CharID, "pill"); got != 1 {
		t.Fatalf("剩余数量应为 1，实际 %d", got)
	}
	if out["gained"] != int64(10) {
		t.Fatalf("回包 gained 应为 10: %+v", out["gained"])
	}
}

func TestSellRejectsUnsellable(t *testing.T) {
	svc, db, sess := newEnv(t)
	seed(t, db, sess.CharID, "gold_bag", 1)
	if _, code, _ := svc.HandleSell(context.Background(), sess,
		map[string]any{"item_id": "gold_bag"}); code == codeOK {
		t.Fatal("sell=0 的物品应拒绝出售")
	}
}

func TestLockedItemRejectsSellAndUse(t *testing.T) {
	svc, db, sess := newEnv(t)
	ctx := context.Background()
	seed(t, db, sess.CharID, "pill", 2)
	seed(t, db, sess.CharID, "exp_dan", 1)
	if _, code, _ := svc.HandleLock(ctx, sess, map[string]any{"item_id": "pill", "locked": true}); code != codeOK {
		t.Fatal("锁定应成功")
	}
	if _, code, _ := svc.HandleSell(ctx, sess, map[string]any{"item_id": "pill"}); code == codeOK {
		t.Fatal("锁定物品应拒绝出售")
	}
	svc.HandleLock(ctx, sess, map[string]any{"item_id": "exp_dan", "locked": true})
	if _, code, _ := svc.HandleUse(ctx, sess, map[string]any{"item_id": "exp_dan"}); code == codeOK {
		t.Fatal("锁定物品应拒绝使用")
	}
	// 解锁后恢复。
	svc.HandleLock(ctx, sess, map[string]any{"item_id": "pill", "locked": false})
	if _, code, _ := svc.HandleSell(ctx, sess, map[string]any{"item_id": "pill"}); code != codeOK {
		t.Fatal("解锁后应可出售")
	}
}

func TestSortOrdersByQualityThenCategory(t *testing.T) {
	svc, db, sess := newEnv(t)
	ctx := context.Background()
	seed(t, db, sess.CharID, "pill", 1)     // 凡品 丹药
	seed(t, db, sess.CharID, "gold_bag", 1) // 凡品 凭证
	seed(t, db, sess.CharID, "exp_dan", 1)  // 良品 丹药
	if _, code, _ := svc.HandleSort(ctx, sess, nil); code != codeOK {
		t.Fatal("整理应成功")
	}
	rows, err := db.Items.List(ctx, sess.CharID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 || rows[0].ItemID != "exp_dan" {
		t.Fatalf("整理后良品应排第一: %+v", rows)
	}
	// 同为凡品时按类别（丹药 before 凭证）。
	if rows[1].ItemID != "pill" || rows[2].ItemID != "gold_bag" {
		t.Fatalf("同品质应按类别排序: %+v", rows)
	}
}
