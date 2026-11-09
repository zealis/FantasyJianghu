// Package item 为背包与钱包的只读视图（M2b 最小集）。
// 完整道具使用、装备穿戴与强化在 M2c 接入，本包只负责 5/1 列表。
package item

import (
	"context"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
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
		if d := s.cfg.Items.ByID(r.ItemID); d != nil {
			name, category, stack = d.Name, d.Category, d.Stack
		}
		out = append(out, map[string]any{
			"id": r.ItemID, "name": name, "category": category, "count": r.Count, "stack": stack,
		})
	}
	return map[string]any{
		"capacity": s.cfg.Items.BagCapacity,
		"used":     len(rows),
		"items":    out,
		"wallet":   map[string]any{"gold": wallet.Gold, "xiuwei": wallet.Xiuwei, "yuanbao": wallet.Yuanbao},
	}, codeOK, ""
}
