// Package item 为背包与钱包的只读视图（M2b 最小集）。
// 完整道具使用、装备穿戴与强化在 M2c 接入，本包只负责 5/1 列表。
package item

import (
	"context"
	"errors"
	"sort"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeNotFound     = 5
	codeBadState     = 9
)

type Service struct {
	items *store.ItemsRepo
	cfg   config.Data
}

func NewService(items *store.ItemsRepo, cfg config.Data) *Service {
	return &Service{items: items, cfg: cfg}
}

// HandleList 处理 5/1 背包列表：格子占用 + 物品明细 + 钱包余额。
func (s *Service) HandleList(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	rows, err := s.items.List(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取背包失败"
	}
	wallet, err := s.items.WalletOf(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取钱包失败"
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		name, category, stack := r.ItemID, "其他", 1
		quality, desc, slot := "凡品", "", ""
		sell := 0
		var use any
		if d := s.cfg.Items.ByID(r.ItemID); d != nil {
			name, category, stack = d.Name, d.Category, d.Stack
			quality, desc, slot = s.cfg.Items.ItemQuality(r.ItemID), d.Desc, d.Slot
			sell = d.Sell
			if d.Use != nil {
				use = map[string]any{"kind": d.Use.Kind, "value": d.Use.Value}
			}
		}
		out = append(out, map[string]any{
			"id": r.ItemID, "name": name, "category": category, "count": r.Count, "stack": stack,
			"quality": quality, "desc": desc, "use": use, "slot": slot,
			"sell": sell, "locked": r.Locked,
		})
	}
	return map[string]any{
		"capacity": s.cfg.Items.BagCapacity,
		"used":     len(rows),
		"items":    out,
		"wallet":   map[string]any{"gold": wallet.Gold, "xiuwei": wallet.Xiuwei, "yuanbao": wallet.Yuanbao},
	}, codeOK, ""
}

// HandleUse 处理 5/2 使用物品：配置 use 定义映射效果，单事务扣物品并结算（M2c 切片一）。
func (s *Service) HandleUse(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	itemID, _ := payload["item_id"].(string)
	def := s.cfg.Items.ByID(itemID)
	if def == nil {
		return nil, codeNotFound, "物品不存在"
	}
	if def.Use == nil {
		return nil, codeBadState, "该物品不可使用"
	}
	req := store.UseRequest{CharacterID: sess.CharID, ItemID: itemID,
		Curve: store.LevelCurve{ExpToNext: s.cfg.Levels.ExpToNext, IsGate: s.cfg.Levels.IsGate,
			MaxLevel: s.cfg.Levels.MaxLevel, PointsPerLevel: s.cfg.Levels.PointsPerLevel}}
	switch def.Use.Kind {
	case "exp":
		req.Exp = def.Use.Value
	case "xiuwei":
		req.Xiuwei = def.Use.Value
	case "gold":
		req.Gold = def.Use.Value
	}
	res, err := s.items.UseItem(ctx, req)
	if err != nil {
		if errors.Is(err, store.ErrItemLocked) {
			return nil, codeBadState, "物品已锁定，先解锁再使用"
		}
		if errors.Is(err, store.ErrItemCount) {
			return nil, codeBadState, "背包中该物品数量不足"
		}
		return nil, codeGeneric, "使用物品失败"
	}
	return map[string]any{
		"ok":        true,
		"item_id":   itemID,
		"effect":    map[string]any{"kind": def.Use.Kind, "value": def.Use.Value},
		"remaining": res.Remaining,
		"wallet":    map[string]any{"gold": res.Gold, "xiuwei": res.Xiuwei},
	}, codeOK, ""
}

// HandleSort 处理 5/3 整理：按品质降序 → 类别 → 名称重排并写回 sort_no（持久化）。
func (s *Service) HandleSort(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	rows, err := s.items.List(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取背包失败"
	}
	order := map[string]int{"完美": 5, "极品": 4, "精品": 3, "良品": 2, "凡品": 1}
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := s.cfg.Items.ByID(rows[i].ItemID), s.cfg.Items.ByID(rows[j].ItemID)
		qi := order[s.cfg.Items.ItemQuality(rows[i].ItemID)]
		qj := order[s.cfg.Items.ItemQuality(rows[j].ItemID)]
		if qi != qj {
			return qi > qj
		}
		ci, cj := "其他", "其他"
		ni, nj := rows[i].ItemID, rows[j].ItemID
		if di != nil {
			ci, ni = di.Category, di.Name
		}
		if dj != nil {
			cj, nj = dj.Category, dj.Name
		}
		if ci != cj {
			return ci < cj
		}
		return ni < nj
	})
	ranks := make(map[string]int, len(rows))
	for i, r := range rows {
		ranks[r.ItemID] = i
	}
	if err := s.items.Reorder(ctx, sess.CharID, ranks); err != nil {
		return nil, codeGeneric, "整理失败"
	}
	return map[string]any{"ok": true, "used": len(rows)}, codeOK, ""
}

// HandleSell 处理 5/4 出售：按配置 sell 单价入账，锁定物品拒绝。
func (s *Service) HandleSell(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	itemID, _ := payload["item_id"].(string)
	def := s.cfg.Items.ByID(itemID)
	if def == nil {
		return nil, codeNotFound, "物品不存在"
	}
	if def.Sell <= 0 {
		return nil, codeBadState, "该物品不可出售"
	}
	count := 1
	if v, ok := payload["count"].(float64); ok && int(v) > 0 {
		count = int(v)
	}
	res, err := s.items.SellItem(ctx, store.SellRequest{
		CharacterID: sess.CharID, ItemID: itemID, Count: count, UnitPrice: int64(def.Sell),
	})
	if err != nil {
		if errors.Is(err, store.ErrItemLocked) {
			return nil, codeBadState, "物品已锁定，先解锁再出售"
		}
		if errors.Is(err, store.ErrItemCount) {
			return nil, codeBadState, "背包中该物品数量不足"
		}
		return nil, codeGeneric, "出售失败"
	}
	rows, _ := s.items.List(ctx, sess.CharID)
	return map[string]any{
		"ok": true, "item_id": itemID, "count": count,
		"gained": int64(def.Sell) * int64(count), "remaining": res.Remaining,
		"gold": res.Gold, "used": len(rows),
	}, codeOK, ""
}

// HandleLock 处理 5/5 锁定/解锁（锁定后禁止使用与出售）。
func (s *Service) HandleLock(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	itemID, _ := payload["item_id"].(string)
	locked, _ := payload["locked"].(bool)
	if err := s.items.SetLocked(ctx, sess.CharID, itemID, locked); err != nil {
		if errors.Is(err, store.ErrItemCount) {
			return nil, codeNotFound, "背包中没有该物品"
		}
		return nil, codeGeneric, "锁定失败"
	}
	return map[string]any{"ok": true, "item_id": itemID, "locked": locked}, codeOK, ""
}
