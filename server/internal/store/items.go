package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// 错误集：任务/背包域的明确失败原因（§6.4 失败返回可读原因）。
var (
	ErrBagFull      = errors.New("store: 背包容量不足")
	ErrQuestState   = errors.New("store: 任务状态不允许该操作")
	ErrAlreadyTaken = errors.New("store: 任务已接取")
	ErrDailyLimit   = errors.New("store: 日常次数已达上限")
	ErrItemCount    = errors.New("store: 物品数量不足")
	ErrItemLocked   = errors.New("store: 物品已锁定")
)

// ItemRow 为背包一格（M2b 最小集：物品 id + 数量；M2c 切片五增锁定与排序号）。
type ItemRow struct {
	ItemID string
	Count  int
	Locked bool
	SortNo int
}

// Wallet 为角色钱包余额（双货币 + 修为）。
type Wallet struct {
	Gold    int64
	Xiuwei  int64
	Yuanbao int64
}

// ItemsRepo 为背包与钱包读写（SQL 只在本层，§2.1）。
type ItemsRepo struct{ db *sql.DB }

// List 取背包全部格子（按物品 id 排序，便于客户端稳定渲染）。
func (r *ItemsRepo) List(ctx context.Context, charID int64) ([]ItemRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT item_id, count, locked, sort_no FROM character_items
		 WHERE character_id = ? AND count > 0 ORDER BY sort_no, item_id, id`,
		charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemRow
	for rows.Next() {
		var it ItemRow
		if err := rows.Scan(&it.ItemID, &it.Count, &it.Locked, &it.SortNo); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// WalletOf 取钱包；无记录视为 0（首次发奖时建行）。
func (r *ItemsRepo) WalletOf(ctx context.Context, charID int64) (Wallet, error) {
	var w Wallet
	err := r.db.QueryRowContext(ctx,
		`SELECT gold, xiuwei, yuanbao FROM character_wallets WHERE character_id = ?`, charID).
		Scan(&w.Gold, &w.Xiuwei, &w.Yuanbao)
	if errors.Is(err, sql.ErrNoRows) {
		return Wallet{}, nil
	}
	return w, err
}

// UseRequest 为使用一件物品的结算请求（效果由配置 use 定义映射而来）。
type UseRequest struct {
	CharacterID       int64
	ItemID            string
	Exp, Xiuwei, Gold int64
	Curve             LevelCurve // 等级结算（M2c 切片三；零值=只累加不升级）
}

// UseResult 为使用结果（余额回读 + 该格剩余数量）。
type UseResult struct {
	Gold, Xiuwei int64
	Remaining    int
}

// UseItem 单事务使用一件物品：扣格（归零删行）→ 经验/钱包入账 → 流水（§6.4）。
// 任一步失败整单回滚，不部分扣除。
func (r *ItemsRepo) UseItem(ctx context.Context, req UseRequest) (UseResult, error) {
	var out UseResult
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		var rowID int64
		var count int
		var locked bool
		if err := tx.QueryRowContext(ctx,
			`SELECT id, count, locked FROM character_items WHERE character_id = ? AND item_id = ? AND count > 0 ORDER BY id LIMIT 1`,
			req.CharacterID, req.ItemID).Scan(&rowID, &count, &locked); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrItemCount
			}
			return err
		}
		if locked {
			return ErrItemLocked
		}
		if count <= 1 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM character_items WHERE id = ?`, rowID); err != nil {
				return err
			}
			out.Remaining = 0
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE character_items SET count = count - 1 WHERE id = ?`, rowID); err != nil {
				return err
			}
			out.Remaining = count - 1
		}
		if req.Exp > 0 {
			if err := AddExpTx(ctx, tx, req.CharacterID, int(req.Exp), req.Curve); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO character_wallets (character_id, gold, xiuwei, yuanbao, updated_at)
			 VALUES (?, 0, 0, 0, ?)`, req.CharacterID, nowUnix()); err != nil {
			return err
		}
		if req.Gold != 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE character_wallets SET gold = gold + ?, updated_at = ? WHERE character_id = ?`,
				req.Gold, nowUnix(), req.CharacterID); err != nil {
				return err
			}
		}
		if req.Xiuwei != 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE character_wallets SET xiuwei = xiuwei + ?, updated_at = ? WHERE character_id = ?`,
				req.Xiuwei, nowUnix(), req.CharacterID); err != nil {
				return err
			}
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT gold, xiuwei FROM character_wallets WHERE character_id = ?`, req.CharacterID).
			Scan(&out.Gold, &out.Xiuwei); err != nil {
			return err
		}
		refID := fmt.Sprintf("item:%s", req.ItemID)
		for _, c := range []struct {
			name  string
			delta int64
			bal   int64
		}{{"gold", req.Gold, out.Gold}, {"xiuwei", req.Xiuwei, out.Xiuwei}} {
			if c.delta == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO currency_logs (character_id, currency, delta, balance_after, reason, ref_id, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				req.CharacterID, c.name, c.delta, c.bal, "item_use", refID, nowUnix()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return UseResult{}, err
	}
	return out, nil
}

// SellRequest 为出售物品的结算请求（单价来自配置 sell，store 只做扣格与入账）。
type SellRequest struct {
	CharacterID int64
	ItemID      string
	Count       int
	UnitPrice   int64
}

// SellResult 为出售结果（金币余额 + 该格剩余数量）。
type SellResult struct {
	Gold      int64
	Remaining int
}

// SellItem 单事务出售：校验数量/锁定 → 扣格（归零删行）→ 金币入账 → 流水。
func (r *ItemsRepo) SellItem(ctx context.Context, req SellRequest) (SellResult, error) {
	var out SellResult
	if req.Count <= 0 {
		return out, ErrItemCount
	}
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		var rowID int64
		var count int
		var locked bool
		if err := tx.QueryRowContext(ctx,
			`SELECT id, count, locked FROM character_items WHERE character_id = ? AND item_id = ? AND count > 0 ORDER BY id LIMIT 1`,
			req.CharacterID, req.ItemID).Scan(&rowID, &count, &locked); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrItemCount
			}
			return err
		}
		if locked {
			return ErrItemLocked
		}
		if count < req.Count {
			return ErrItemCount
		}
		if count == req.Count {
			if _, err := tx.ExecContext(ctx, `DELETE FROM character_items WHERE id = ?`, rowID); err != nil {
				return err
			}
			out.Remaining = 0
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE character_items SET count = count - ? WHERE id = ?`, req.Count, rowID); err != nil {
				return err
			}
			out.Remaining = count - req.Count
		}
		total := req.UnitPrice * int64(req.Count)
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO character_wallets (character_id, gold, xiuwei, yuanbao, updated_at)
			 VALUES (?, 0, 0, 0, ?)`, req.CharacterID, nowUnix()); err != nil {
			return err
		}
		if total > 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE character_wallets SET gold = gold + ?, updated_at = ? WHERE character_id = ?`,
				total, nowUnix(), req.CharacterID); err != nil {
				return err
			}
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT gold FROM character_wallets WHERE character_id = ?`, req.CharacterID).
			Scan(&out.Gold); err != nil {
			return err
		}
		if total > 0 {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO currency_logs (character_id, currency, delta, balance_after, reason, ref_id, created_at)
				 VALUES (?, 'gold', ?, ?, 'item_sell', ?, ?)`,
				req.CharacterID, total, out.Gold, "item:"+req.ItemID, nowUnix()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SellResult{}, err
	}
	return out, nil
}

// SetLocked 设置某物品所有格子的锁定状态（不存在返回 ErrItemCount）。
func (r *ItemsRepo) SetLocked(ctx context.Context, charID int64, itemID string, locked bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE character_items SET locked = ? WHERE character_id = ? AND item_id = ?`,
		locked, charID, itemID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrItemCount
	}
	return nil
}

// Reorder 写入整理后的格子顺序（ranks: itemID → 序号，小在前）。
func (r *ItemsRepo) Reorder(ctx context.Context, charID int64, ranks map[string]int) error {
	return withTx(ctx, r.db, func(tx *sql.Tx) error {
		for id, rank := range ranks {
			if _, err := tx.ExecContext(ctx,
				`UPDATE character_items SET sort_no = ? WHERE character_id = ? AND item_id = ?`,
				rank, charID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// slotNeed 计算把 add 塞进已有格子（含堆叠上限 stack）所需的新增格数，
// 并返回塞满后剩余堆叠空间（供预检复用）。纯函数，便于单测。
func slotNeed(existing []ItemRow, add map[string]int, stack map[string]int) (need int, ok bool) {
	free := map[string]int{}
	for _, it := range existing {
		s := stack[it.ItemID]
		if s <= 0 {
			s = 1
		}
		if it.Count < s {
			free[it.ItemID] += s - it.Count
		}
	}
	// 奖励按配置顺序累计：先吃剩余堆叠空间，不足部分按上限折算新格。
	for id, n := range add {
		s := stack[id]
		if s <= 0 {
			s = 1
		}
		rest := n
		if take := free[id]; take > 0 {
			if take >= rest {
				free[id] = take - rest
				rest = 0
			} else {
				rest -= take
				free[id] = 0
			}
		}
		if rest > 0 {
			need += (rest + s - 1) / s
		}
	}
	return need, true
}
