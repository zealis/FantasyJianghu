package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// QuestObjectiveProgress 为单条目标进度（kind+target 定位，cur 封顶 need）。
type QuestObjectiveProgress struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Cur    int    `json:"cur"`
	Need   int    `json:"need"`
}

// CharacterQuest 为角色任务行；日常每次接取一行（可重复），主线同任务仅一行。
type CharacterQuest struct {
	ID          int64
	CharacterID int64
	QuestID     string
	Kind        string // main|daily_normal|daily_sect|daily_dungeon
	State       string // active|done
	Progress    []QuestObjectiveProgress
	AcceptedAt  int64
	CompletedAt int64
}

// QuestRepo 为任务存档读写；交付发奖在单事务内完成（§6.4）。
type QuestRepo struct{ db *sql.DB }

const questCols = `id, character_id, quest_id, kind, state, progress, accepted_at, completed_at`

func scanQuest(row interface{ Scan(...any) error }) (CharacterQuest, error) {
	var q CharacterQuest
	var raw string
	if err := row.Scan(&q.ID, &q.CharacterID, &q.QuestID, &q.Kind, &q.State, &raw, &q.AcceptedAt, &q.CompletedAt); err != nil {
		return q, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &q.Progress); err != nil {
			return q, fmt.Errorf("解析任务进度 %d: %w", q.ID, err)
		}
	}
	return q, nil
}

// List 取角色全部任务行（含已交付，供状态机判定前置）。
func (r *QuestRepo) List(ctx context.Context, charID int64) ([]CharacterQuest, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+questCols+` FROM character_quests WHERE character_id = ? ORDER BY id`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharacterQuest
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// CompletedToday 取某类日常当日已完成次数（交付计数）。
func (r *QuestRepo) CompletedToday(ctx context.Context, charID int64, category, day string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(count), 0) FROM character_daily WHERE character_id = ? AND day = ? AND category = ?`,
		charID, day, category).Scan(&n)
	return n, err
}

// Accept 接取任务：主线防重复、日常校验当日额度，单事务插入进度行。
func (r *QuestRepo) Accept(ctx context.Context, charID int64, questID, kind string,
	progress []QuestObjectiveProgress, day, dailyCategory string, dailyLimit int, now int64) (CharacterQuest, error) {
	var out CharacterQuest
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		if kind == "main" {
			var n int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM character_quests WHERE character_id = ? AND quest_id = ? AND state IN ('active','done')`,
				charID, questID).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrAlreadyTaken
			}
		} else {
			var active, done int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM character_quests WHERE character_id = ? AND kind = ? AND state = 'active'`,
				charID, kind).Scan(&active); err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx,
				`SELECT COALESCE(SUM(count), 0) FROM character_daily WHERE character_id = ? AND day = ? AND category = ?`,
				charID, day, dailyCategory).Scan(&done); err != nil {
				return err
			}
			if active+done >= dailyLimit {
				return ErrDailyLimit
			}
		}
		raw, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO character_quests (character_id, quest_id, kind, state, progress, accepted_at)
			 VALUES (?, ?, ?, 'active', ?, ?)`, charID, questID, kind, string(raw), now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		out = CharacterQuest{ID: id, CharacterID: charID, QuestID: questID, Kind: kind,
			State: "active", Progress: progress, AcceptedAt: now}
		return nil
	})
	if err != nil {
		return CharacterQuest{}, err
	}
	return out, nil
}

// AddProgress 推进角色所有匹配目标的活动任务；返回发生变化的任务行（用于推送）。
// 同一目标进度封顶到 need，重复事件不溢出。
func (r *QuestRepo) AddProgress(ctx context.Context, charID int64, kind, target string, n int) ([]CharacterQuest, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+questCols+` FROM character_quests WHERE character_id = ? AND state = 'active' ORDER BY id`, charID)
	if err != nil {
		return nil, err
	}
	var list []CharacterQuest
	for rows.Next() {
		q, err := scanQuest(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, q)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var changed []CharacterQuest
	for _, q := range list {
		dirty := false
		for i := range q.Progress {
			p := &q.Progress[i]
			if p.Kind != kind || p.Target != target || p.Cur >= p.Need {
				continue
			}
			p.Cur += n
			if p.Cur > p.Need {
				p.Cur = p.Need
			}
			dirty = true
		}
		if !dirty {
			continue
		}
		raw, err := json.Marshal(q.Progress)
		if err != nil {
			return nil, err
		}
		if _, err := r.db.ExecContext(ctx,
			`UPDATE character_quests SET progress = ? WHERE id = ?`, string(raw), q.ID); err != nil {
			return nil, err
		}
		changed = append(changed, q)
	}
	return changed, nil
}

// GrantItem 为发放物品条目。
type GrantItem struct {
	ID    string
	Count int
}

// SubmitRequest 为一次交付发奖的完整描述（校验→扣除→发放→流水，单事务）。
type SubmitRequest struct {
	CharacterID       int64
	QuestRowID        int64
	QuestID           string
	Kind              string // main|daily_*
	Day               string // 日常计数分桶日期（UTC+8）
	DailyCategory     string
	DailyLimit        int
	Exp, Xiuwei, Gold int64
	Items             []GrantItem
	StackOf           func(itemID string) int // 单格上限（配置来源）
	BagCapacity       int
}

// SubmitResult 为交付结果（客户端展示奖励与到账后余额）。
type SubmitResult struct {
	DailyCount   int
	Gold, Xiuwei int64
	Exp          int64
	Items        []GrantItem
}

// Submit 单事务完成交付：状态机校验 → 背包预检 → 奖励入账 → 流水。
// 预检不通过整单拒绝（§9.1/§6.4：不部分发放）。
func (r *QuestRepo) Submit(ctx context.Context, req SubmitRequest) (SubmitResult, error) {
	var out SubmitResult
	err := withTx(ctx, r.db, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx,
			`SELECT state FROM character_quests WHERE id = ? AND character_id = ?`,
			req.QuestRowID, req.CharacterID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrQuestState
			}
			return err
		}
		if state != "active" {
			return ErrQuestState
		}
		// 日常交付计数与上限二次校验（接取时已卡一次，防并发超发）。
		dailyCount := 0
		if req.DailyCategory != "" {
			if err := tx.QueryRowContext(ctx,
				`SELECT COALESCE(SUM(count), 0) FROM character_daily WHERE character_id = ? AND day = ? AND category = ?`,
				req.CharacterID, req.Day, req.DailyCategory).Scan(&dailyCount); err != nil {
				return err
			}
			if dailyCount >= req.DailyLimit {
				return ErrDailyLimit
			}
		}
		// 背包容量预检（按格 + 堆叠余量）。
		existing, err := itemsTx(ctx, tx, req.CharacterID)
		if err != nil {
			return err
		}
		add := map[string]int{}
		for _, it := range req.Items {
			add[it.ID] += it.Count
		}
		need, _ := slotNeed(existing, add, stackMap(add, req.StackOf))
		if len(existing)+need > req.BagCapacity {
			return fmt.Errorf("%w：需要 %d 格，当前已用 %d/%d", ErrBagFull, need, len(existing), req.BagCapacity)
		}
		// 结算任务状态。
		if _, err := tx.ExecContext(ctx,
			`UPDATE character_quests SET state = 'done', completed_at = ? WHERE id = ?`,
			nowUnix(), req.QuestRowID); err != nil {
			return err
		}
		// 经验累加（等级曲线/突破关口 M2c 单独接入，本轮只累加）。
		if req.Exp > 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE characters SET exp = exp + ? WHERE id = ?`, req.Exp, req.CharacterID); err != nil {
				return err
			}
		}
		// 钱包（首次建行）与流水。
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
		refID := fmt.Sprintf("quest:%s", req.QuestID)
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
				req.CharacterID, c.name, c.delta, c.bal, "quest_reward", refID, nowUnix()); err != nil {
				return err
			}
		}
		// 物品入包：先填已有格余量，再开新格。
		sorted := make([]GrantItem, len(req.Items))
		copy(sorted, req.Items)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
		for _, it := range sorted {
			if err := addItemTx(ctx, tx, req.CharacterID, it.ID, it.Count, req.StackOf); err != nil {
				return err
			}
		}
		if req.DailyCategory != "" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO character_daily (character_id, day, category, count) VALUES (?, ?, ?, 1)
				 ON CONFLICT(character_id, day, category) DO UPDATE SET count = count + 1`,
				req.CharacterID, req.Day, req.DailyCategory); err != nil {
				return err
			}
			dailyCount++
		}
		out.DailyCount = dailyCount
		out.Exp = req.Exp
		out.Items = sorted
		return nil
	})
	if err != nil {
		return SubmitResult{}, err
	}
	return out, nil
}

// itemsTx 在事务内读背包（提交与预检必须看到同一快照）。
func itemsTx(ctx context.Context, tx *sql.Tx, charID int64) ([]ItemRow, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT item_id, count FROM character_items WHERE character_id = ? AND count > 0 ORDER BY item_id, id`, charID)
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

// addItemTx 入库：按格上限填充，允许同一物品多格。
func addItemTx(ctx context.Context, tx *sql.Tx, charID int64, itemID string, count int, stackOf func(string) int) error {
	stack := stackOf(itemID)
	if stack <= 0 {
		stack = 1
	}
	rest := count
	rows, err := tx.QueryContext(ctx,
		`SELECT id, count FROM character_items WHERE character_id = ? AND item_id = ? AND count < ? ORDER BY id`,
		charID, itemID, stack)
	if err != nil {
		return err
	}
	type slot struct {
		id    int64
		count int
	}
	var slots []slot
	for rows.Next() {
		var s slot
		if err := rows.Scan(&s.id, &s.count); err != nil {
			rows.Close()
			return err
		}
		slots = append(slots, s)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, s := range slots {
		if rest <= 0 {
			break
		}
		add := stack - s.count
		if add > rest {
			add = rest
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE character_items SET count = count + ? WHERE id = ?`, add, s.id); err != nil {
			return err
		}
		rest -= add
	}
	for rest > 0 {
		add := stack
		if add > rest {
			add = rest
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO character_items (character_id, item_id, count, created_at) VALUES (?, ?, ?, ?)`,
			charID, itemID, add, nowUnix()); err != nil {
			return err
		}
		rest -= add
	}
	return nil
}

func stackMap(add map[string]int, stackOf func(string) int) map[string]int {
	out := make(map[string]int, len(add))
	for id := range add {
		out[id] = stackOf(id)
	}
	return out
}

// withTx 为 store 内部事务助手（§6.4 校验→扣除→发放→流水四步原子）。
func withTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if rb := tx.Rollback(); rb != nil {
			return fmt.Errorf("%w（回滚亦失败: %v）", err, rb)
		}
		return err
	}
	return tx.Commit()
}
