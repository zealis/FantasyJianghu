package battle

import (
	"context"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

// TestPlayerUnitIncludesEquipAndResonance 4/1 出场单位必须带装备与品质共鸣。
// 这是「穿了装备在战斗里没用」那处缺陷的回归测试：基线 → 装备 → 共鸣一路到 Unit.Stats。
func TestPlayerUnitIncludesEquipAndResonance(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	acctID, err := db.Accounts.CreateAccount(ctx, "战斗共鸣侠", "hash", "")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "剑客", "断岳剑宗", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	if _, err := db.SQL().Exec(`UPDATE characters SET level = 20 WHERE id = ?`, charID); err != nil {
		t.Fatalf("设置等级: %v", err)
	}

	anchor := config.AnchorStat{HP: 1000, MP: 480, PAtk: 100, MAtk: 100, PDef: 50, MDef: 60, Speed: 100}
	items := []config.ItemDef{
		{ID: "w1", Name: "武器", Category: "装备", Stack: 1, Slot: "weapon", Quality: "完美",
			Stats: map[string]int{"patk": 10}},
		{ID: "c1", Name: "衣服", Category: "装备", Stack: 1, Slot: "clothes", Quality: "完美",
			Stats: map[string]int{"pdef": 10}},
		{ID: "h1", Name: "帽子", Category: "装备", Stack: 1, Slot: "hat", Quality: "完美",
			Stats: map[string]int{"pdef": 10}},
		{ID: "n1", Name: "项链", Category: "装备", Stack: 1, Slot: "necklace", Quality: "完美",
			Stats: map[string]int{"mdef": 10}},
		{ID: "b1", Name: "鞋子", Category: "装备", Stack: 1, Slot: "boots", Quality: "完美",
			Stats: map[string]int{"speed": 10}},
	}
	cfg := config.Data{
		Items: config.ItemData{BagCapacity: 20, Items: items},
		Factions: []config.FactionInfo{
			{Name: "断岳剑宗", Category: "物理", Color: "朱橙", Residence: "高天崖", Weapon: "剑",
				Feature: "暴击", Secret: "断岳剑经"},
		},
		Battle: config.BattleData{
			Classes: []config.BattleClass{{Class: "物理", Factions: []string{"断岳剑宗"},
				Anchors: map[string]config.AnchorStat{"20": anchor}}},
			ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
		},
		Suit: config.SuitData{
			AllAttrs:     []string{"hp", "mp", "patk", "matk", "pdef", "mdef", "speed"},
			QualityOrder: map[string]int{"凡品": 1, "良品": 2, "精品": 3, "极品": 4, "完美": 5},
			Tier1AllPct:  []float64{1, 2, 3, 4, 5},
			Tier2MainPct: []float64{5, 6, 7, 8, 9},
			Systems: map[string]config.SuitSystem{
				"物理": {Main: []string{"patk"}, Sub: "pcrit", SubPct: []float64{1.5, 2, 2.5, 3, 3.5}},
			},
			FactionTweaks: map[string]config.FactionTweak{
				"断岳剑宗": {Attr: "pcrit", Pct: []float64{1, 1.5, 2, 2.5, 3}},
			},
		},
	}
	for _, id := range []string{"w1", "c1", "h1", "n1", "b1"} {
		if _, err := db.SQL().Exec(
			`INSERT INTO character_items (character_id, item_id, count, created_at) VALUES (?, ?, 1, 0)`,
			charID, id); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	def := func(slot string) string {
		switch slot {
		case "weapon":
			return "w1"
		case "clothes":
			return "c1"
		case "hat":
			return "h1"
		case "necklace":
			return "n1"
		default:
			return "b1"
		}
	}
	for _, slot := range []string{"weapon", "clothes", "hat", "necklace", "boots"} {
		if err := db.Equips.Wear(ctx, charID, def(slot), slot, cfg.Items.ItemStack, 20); err != nil {
			t.Fatalf("穿戴 %s: %v", slot, err)
		}
	}

	svc := NewService(nil, db.Characters, db.Equips, cfg)
	u, err := svc.playerUnit(ctx, charID)
	if err != nil {
		t.Fatalf("playerUnit: %v", err)
	}
	// 物攻：(100 基线 + 10 装备) × 1.09（二档主项，取代一档全属性）= 119.9 → 120
	if u.Stats.PAtk != 120 {
		t.Fatalf("出场单位物攻应含装备与共鸣 = 120，实际 %d", u.Stats.PAtk)
	}
	// 气血：二档不叠一档全属性 → 保持 1000
	if u.Stats.HP != 1000 {
		t.Fatalf("二档取代一档，气血应保持 1000，实际 %d", u.Stats.HP)
	}
	// 暴击：基线 0（本配置无 formula）+ 副项 3.5% + 微调 3% = 650
	if u.Stats.PCrit != 650 {
		t.Fatalf("出场单位暴击应为 650 万分制，实际 %d", u.Stats.PCrit)
	}
	if u.HP != u.Stats.HP {
		t.Fatalf("初始血条应等于共鸣后上限，HP=%d Stats.HP=%d", u.HP, u.Stats.HP)
	}
}
