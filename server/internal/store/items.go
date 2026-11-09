package store

import (
	"context"
	"database/sql"
	"errors"
)

// 错误集：任务/背包域的明确失败原因（§6.4 失败返回可读原因）。
var (
	ErrBagFull      = errors.New("store: 背包容量不足")
	ErrQuestState   = errors.New("store: 任务状态不允许该操作")
	ErrAlreadyTaken = errors.New("store: 任务已接取")
	ErrDailyLimit   = errors.New("store: 日常次数已达上限")
)

// ItemRow 为背包一格（M2b 最小集：物品 id + 数量；实例属性/强化 M2c）。
type ItemRow struct {
	ItemID string
	Count  int
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
		`SELECT item_id, count FROM character_items WHERE character_id = ? AND count > 0 ORDER BY item_id, id`,
		charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemRow
	for rows.Next() {
		var it ItemRow
		if err := rows.Scan(&it.ItemID, &it.Count); err != nil {
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
