package battle

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

// 心法是**门派身份**：1 级裸装即生效，不依赖装备、更不依赖「5 件完美共鸣」。
// 本测试逐个门派验证：1 级无装备时心法被动已经写进最终属性。

// realConfig 把仓库里的真实配置源打包到临时目录再加载——直接 Load 源目录缺 manifest。
func realConfig(t *testing.T) config.Data {
	t.Helper()
	src := filepath.Join("..", "..", "configs", "source")
	out := t.TempDir()
	if err := config.Pack(src, out, "1.0.0"); err != nil {
		t.Fatalf("打包真实配置源: %v", err)
	}
	packed := filepath.Join(out, "1.0.0")
	if _, err := os.Stat(packed); err != nil {
		entries, _ := os.ReadDir(out)
		for _, e := range entries {
			if e.IsDir() {
				packed = filepath.Join(out, e.Name())
				break
			}
		}
	}
	cfg, err := config.Load(packed)
	if err != nil {
		t.Fatalf("加载真实配置: %v", err)
	}
	return cfg
}

// heartExpect 为各派心法在 1 级的期望收益（万分制；0 表示该项不涉及）。
type heartExpect struct {
	patk, matk, hp, pdef, mdef, heal               int
	pcrit, mcrit, pcombo, sealhit, pdodge, critDmg int
	reflectPerStack, reflectMaxStacks              int
}

func TestEveryFactionHeartAppliesAtLevelOne(t *testing.T) {
	cfg := realConfig(t)
	want := map[string]heartExpect{
		"断岳剑宗": {patk: 300, critDmg: 1000},
		"破军枪府": {patk: 300, pcombo: 500},
		"惊雷殿":  {matk: 300},
		"璇玑星宫": {matk: 300},
		"缚灵符观": {sealhit: 800},
		"织梦谷":  {sealhit: 500},
		"金刚禅院": {hp: 500, pdef: 500, reflectPerStack: 300, reflectMaxStacks: 3},
		"玄水宫":  {hp: 400, mdef: 500, heal: 800},
	}
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()

	names := []string{"行一", "行二", "行三", "行四", "行五", "行六", "行七", "行八"}
	i := 0
	for faction, exp := range want {
		nm := names[i]
		i++
		acctID, err := db.Accounts.CreateAccount(ctx, "心法-"+faction, "hash", "")
		if err != nil {
			t.Fatalf("%s 建号: %v", faction, err)
		}
		charID, err := db.Characters.CreateCharacter(ctx, acctID, nm, faction, "男")
		if err != nil {
			t.Fatalf("%s 建角: %v", faction, err)
		}
		c, err := db.Characters.GetCharacter(ctx, charID)
		if err != nil {
			t.Fatalf("%s 读角: %v", faction, err)
		}
		if c.Level != 1 {
			t.Fatalf("%s 新角色应为 1 级，实际 %d", faction, c.Level)
		}
		base, err := DerivedStatsWithPoints(c.Level, c.Faction, cfg.Battle, cfg.Levels.PointGains, c.PointsMap())
		if err != nil {
			t.Fatalf("%s 派生: %v", faction, err)
		}
		fin, err := LoadFinalStats(ctx, &c, db.Equips, cfg)
		if err != nil {
			t.Fatalf("%s 最终属性: %v", faction, err)
		}
		if fin.Suit.Tier != 0 {
			t.Fatalf("%s 裸装不该有共鸣: %+v", faction, fin.Suit)
		}
		// 期望：基线 × (1 + 心法 %)（四舍五入），概率型按万分制相加。
		round := func(v int, bp int) int {
			return int(math.Round(float64(v) * (1 + float64(bp)/10000.0)))
		}
		if exp.patk > 0 && fin.Stats.PAtk != round(base.PAtk, exp.patk) {
			t.Fatalf("%s 物攻应吃心法 +%d%%，基线 %d → %d，实际 %d",
				faction, exp.patk/100, base.PAtk, round(base.PAtk, exp.patk), fin.Stats.PAtk)
		}
		if exp.matk > 0 && fin.Stats.MAtk != round(base.MAtk, exp.matk) {
			t.Fatalf("%s 法攻应吃心法 +%d%%，实际 %d", faction, exp.matk/100, fin.Stats.MAtk)
		}
		if exp.hp > 0 && fin.Stats.HP != round(base.HP, exp.hp) {
			t.Fatalf("%s 生命应吃心法 +%d%%，实际 %d", faction, exp.hp/100, fin.Stats.HP)
		}
		if exp.pdef > 0 && fin.Stats.PDef != round(base.PDef, exp.pdef) {
			t.Fatalf("%s 物防应吃心法 +%d%%，实际 %d", faction, exp.pdef/100, fin.Stats.PDef)
		}
		if exp.mdef > 0 && fin.Stats.MDef != round(base.MDef, exp.mdef) {
			t.Fatalf("%s 法防应吃心法 +%d%%，实际 %d", faction, exp.mdef/100, fin.Stats.MDef)
		}
		checks := []struct {
			name string
			got  int
			want int
		}{
			{"暴击率", fin.Stats.PCrit - base.PCrit, exp.pcrit},
			{"法暴率", fin.Stats.MCrit - base.MCrit, exp.mcrit},
			{"连击率", fin.Stats.PCombo - base.PCombo, exp.pcombo},
			{"封印命中", fin.Stats.SealHit - base.SealHit, exp.sealhit},
			{"闪避", fin.Stats.PDodge - base.PDodge, exp.pdodge},
			{"暴击伤害", fin.Stats.CritDmg, exp.critDmg},
			{"治疗加成", fin.Stats.HealBonus, exp.heal},
			{"金刚印每层", fin.Stats.ReflectPerStack, exp.reflectPerStack},
			{"金刚印上限", fin.Stats.ReflectMaxStacks, exp.reflectMaxStacks},
		}
		for _, ck := range checks {
			if ck.got != ck.want {
				t.Fatalf("%s %s 应为 %d，实际 %d", faction, ck.name, ck.want, ck.got)
			}
		}
		// 每个门派都必须从心法拿到至少一项收益（防「心法空转」回归）。
		total := exp.patk + exp.matk + exp.hp + exp.pdef + exp.mdef + exp.heal +
			exp.pcrit + exp.mcrit + exp.pcombo + exp.sealhit + exp.pdodge + exp.critDmg +
			exp.reflectPerStack
		if total == 0 {
			t.Fatalf("%s 心法没有任何生效收益", faction)
		}
	}
}

// TestHeartSkillIsLevelOneForEveryFaction 配置层再确认一次：八派各一本 1 级心法。
func TestHeartSkillIsLevelOneForEveryFaction(t *testing.T) {
	cfg := realConfig(t)
	count := map[string]int{}
	for _, sk := range cfg.Battle.Skills {
		if sk.Category == "心法" {
			if sk.LevelReq != 1 {
				t.Fatalf("%s 的心法 %s 开放等级 %d，应为 1 级（门派身份）", sk.Faction, sk.ID, sk.LevelReq)
			}
			count[sk.Faction]++
		}
	}
	for _, f := range cfg.Factions {
		if count[f.Name] != 1 {
			t.Fatalf("门派 %s 心法数量 %d，应为 1", f.Name, count[f.Name])
		}
	}
}
