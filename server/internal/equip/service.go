// Package equip 为装备穿戴域（M2c 切片二：6/1 穿戴、6/2 卸下、6/3 视图）。
// 属性 = 门派×等级派生基线（battle.DerivedStats）+ 已穿戴装备 stats 累加；
// 品质共鸣加成数值待数值规范 §9.2 落地，本域不发明加成。
package equip

import (
	"context"
	"errors"

	"fantasy-jianghu/server/internal/battle"
	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/session"
	"fantasy-jianghu/server/internal/store"
)

const (
	codeOK           = 0
	codeGeneric      = 1
	codeUnauthorized = 2
	codeNotFound     = 5
	codeBagFull      = 8
	codeBadState     = 9
)

var validSlots = map[string]bool{"weapon": true, "clothes": true, "hat": true, "necklace": true, "boots": true}

type Service struct {
	items  *store.ItemsRepo
	equips *store.EquipsRepo
	chars  *store.CharactersRepo
	cfg    config.Data
}

func NewService(items *store.ItemsRepo, equips *store.EquipsRepo, chars *store.CharactersRepo, cfg config.Data) *Service {
	return &Service{items: items, equips: equips, chars: chars, cfg: cfg}
}

// HandleWear 处理 6/1 穿戴：校验装备定义后走单事务（换装回包预检在仓储层）。
func (s *Service) HandleWear(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	itemID, _ := payload["item_id"].(string)
	def := s.cfg.Items.ByID(itemID)
	if def == nil {
		return nil, codeNotFound, "物品不存在"
	}
	if def.Slot == "" {
		return nil, codeBadState, "该物品不是装备"
	}
	err := s.equips.Wear(ctx, sess.CharID, itemID, def.Slot, s.cfg.Items.ItemStack, s.cfg.Items.BagCapacity)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrItemCount):
			return nil, codeBadState, "背包中该物品数量不足"
		case errors.Is(err, store.ErrEquipState):
			return nil, codeBadState, "该槽已穿戴此物品"
		case errors.Is(err, store.ErrBagFull):
			return nil, codeBagFull, "背包容量不足，请先整理背包再换装"
		default:
			return nil, codeGeneric, "穿戴失败"
		}
	}
	return map[string]any{"ok": true, "slot": def.Slot}, codeOK, ""
}

// HandleRemove 处理 6/2 卸下：回包容量预检在仓储层单事务内。
func (s *Service) HandleRemove(ctx context.Context, sess *session.Session, payload map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	slot, _ := payload["slot"].(string)
	if !validSlots[slot] {
		return nil, codeBadState, "装备槽非法"
	}
	item, err := s.equips.Remove(ctx, sess.CharID, slot, s.cfg.Items.ItemStack, s.cfg.Items.BagCapacity)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrEquipState):
			return nil, codeBadState, "该槽未穿戴装备"
		case errors.Is(err, store.ErrBagFull):
			return nil, codeBagFull, "背包容量不足，无法卸下"
		default:
			return nil, codeGeneric, "卸下失败"
		}
	}
	return map[string]any{"ok": true, "item_id": item}, codeOK, ""
}

// HandleView 处理 6/3 装备视图：槽位明细 + 属性（基线 + 装备加成）。
func (s *Service) HandleView(ctx context.Context, sess *session.Session, _ map[string]any) (map[string]any, int, string) {
	if sess == nil || sess.CharID == 0 {
		return nil, codeUnauthorized, "未鉴权或未选角"
	}
	char, err := s.chars.GetCharacter(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取角色失败"
	}
	base, err := battle.DerivedStatsWithPoints(char.Level, char.Faction, s.cfg.Battle, s.cfg.Levels.PointGains, char.PointsMap())
	if err != nil {
		return nil, codeGeneric, "属性派生失败"
	}
	equipped, err := s.equips.List(ctx, sess.CharID)
	if err != nil {
		return nil, codeGeneric, "读取装备失败"
	}
	slots := map[string]any{}
	sum := map[string]int{}
	for slot, itemID := range equipped {
		def := s.cfg.Items.ByID(itemID)
		if def == nil {
			continue
		}
		slots[slot] = map[string]any{
			"id": itemID, "name": def.Name, "quality": s.cfg.Items.ItemQuality(itemID), "stats": def.Stats,
		}
		for k, v := range def.Stats {
			sum[k] += v
		}
	}
	attrs := map[string]any{
		"hp": base.HP + sum["hp"], "mp": base.MP + sum["mp"],
		"patk": base.PAtk + sum["patk"], "matk": base.MAtk + sum["matk"],
		"pdef": base.PDef + sum["pdef"], "mdef": base.MDef + sum["mdef"],
		"speed": base.Speed + sum["speed"],
		"hit": base.PHit, "dodge": base.PDodge,
		"crit": base.PCrit, "combo": base.PCombo, "resist": base.SealRes,
	}
	return map[string]any{"slots": slots, "attrs": attrs}, codeOK, ""
}