package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrEquipState 为装备槽状态不允许该操作（未穿戴/重复穿戴）。
var ErrEquipState = errors.New("store: 装备状态不允许该操作")

// EquipsRepo 为装备穿戴读写（M2c 切片二：一槽一件；SQL 只在本层）。
type EquipsRepo struct{ db *sql.DB }

// List 取已穿戴槽位 → 物品 id。
func (r *EquipsRepo) List(ctx context.Context, charID int64) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT slot, item_id FROM character_equips WHERE character_id = ?`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var slot, item string
		if err := rows.Scan(&slot, &item); err != nil {
			return nil, err
		}
		out[slot] = item
	}
	return out, rows.Err()
}

// Wear 单事务穿戴：背包有该物品 → 槽未穿同件 → 换装旧件回包容量预检 →
// 扣新件 → upsert 槽 → 旧件入包。任一步失败整单回滚（§6.4 不部分变更）。
func (r *EquipsRepo) Wear(ctx context.Context, charID int64, itemID, slot string,
	stackOf func(string) int, bagCapacity int) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		var rowID int64
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT id, count FROM character_items WHERE character_id = ? AND item_id = ? AND count > 0 ORDER BY id LIMIT 1`,
			charID, itemID).Scan(&rowID, &count); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrItemCount
			}
			return err
		}
		var cur string
		err := tx.QueryRowContext(ctx,
			`SELECT item_id FROM character_equips WHERE character_id = ? AND slot = ?`, charID, slot).Scan(&cur)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if cur == itemID {
			return ErrEquipState
		}
		existing, err := itemsTx(ctx, tx, charID)
		if err != nil {
			return err
		}
		// 扣掉新件后的背包快照，用于旧件回包预检（同物品多格只扣被消耗的那格）。
		after := make([]ItemRow, 0, len(existing))
		consumed := false
		for _, it := range existing {
			if it.ItemID == itemID && !consumed {
				consumed = true
				if count-1 > 0 {
					after = append(after, ItemRow{ItemID: it.ItemID, Count: count - 1})
				}
				continue
			}
			after = append(after, it)
		}
		if cur != "" {
			need, _ := slotNeed(after, map[string]int{cur: 1}, stackMap(map[string]int{cur: 1}, stackOf))
			if len(after)+need > bagCapacity {
				return ErrBagFull
			}
		}
		if count <= 1 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM character_items WHERE id = ?`, rowID); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE character_items SET count = count - 1 WHERE id = ?`, rowID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO character_equips (character_id, slot, item_id) VALUES (?, ?, ?)`,
			charID, slot, itemID); err != nil {
			return err
		}
		if cur != "" {
			if err := addItemTx(ctx, tx, charID, cur, 1, stackOf); err != nil {
				return err
			}
		}
		return nil
	})
}

// Remove 单事务卸下：槽非空 → 回包容量预检 → 删槽 → 入包；返回卸下的物品 id。
func (r *EquipsRepo) Remove(ctx context.Context, charID int64, slot string,
	stackOf func(string) int, bagCapacity int) (string, error) {
	var item string
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`SELECT item_id FROM character_equips WHERE character_id = ? AND slot = ?`, charID, slot).Scan(&item); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEquipState
			}
			return err
		}
		existing, err := itemsTx(ctx, tx, charID)
		if err != nil {
			return err
		}
		need, _ := slotNeed(existing, map[string]int{item: 1}, stackMap(map[string]int{item: 1}, stackOf))
		if len(existing)+need > bagCapacity {
			return ErrBagFull
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM character_equips WHERE character_id = ? AND slot = ?`, charID, slot); err != nil {
			return err
		}
		return addItemTx(ctx, tx, charID, item, 1, stackOf)
	})
	if err != nil {
		return "", err
	}
	return item, nil
}