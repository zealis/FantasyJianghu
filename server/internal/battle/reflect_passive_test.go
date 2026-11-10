package battle

import (
	"context"
	"math"
	"testing"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

// 反伤作为**角色属性**（数值规范 §9.7 / 秘籍表 V1.1 §8）：
// 来源四类并列——心法金刚印 +3%/层、绝学不动明王 +25%、共鸣微调 +1~3%、法宝昆仑镜（待落地）。
// 心法是门派身份：1 级裸装挨打就要有反伤，不是毕业装备奖励。

func monkSkills() []config.SkillDef {
	return []config.SkillDef{
		{ID: "jingang_buhuai", Faction: "金刚禅院", Category: "心法", LevelReq: 1,
			Passive: &config.PassiveDef{HPPct: 500, PDefPct: 500, ReflectPerStack: 300, ReflectMaxStacks: 3}},
		{ID: "budong_mingwang", Faction: "金刚禅院", Category: "绝学", LevelReq: 80, MPCost: 650,
			BuffSelf: &struct {
				Stat  string `json:"stat"`
				Value int    `json:"value"`
				Turns int    `json:"turns"`
			}{Stat: "reflect_add", Value: 2500, Turns: 3}},
		{ID: "jinglei_baodian", Faction: "惊雷殿", Category: "心法", LevelReq: 1,
			Passive: &config.PassiveDef{MAtkPct: 300}},
	}
}

func TestCollectPassivesByFaction(t *testing.T) {
	monk := CollectPassives(monkSkills(), "金刚禅院", 1)
	if monk.HPPct != 500 || monk.PDefPct != 500 {
		t.Fatalf("禅院心法应给 生命+5%%/物防+5%%: %+v", monk)
	}
	if monk.ReflectPerStack != 300 || monk.ReflectStacksCap() != 3 {
		t.Fatalf("金刚印应为 3%%/层、上限 3 层: %+v", monk)
	}
	lei := CollectPassives(monkSkills(), "惊雷殿", 1)
	if lei.ReflectPerStack != 0 {
		t.Fatalf("惊雷殿不该有金刚印（门派隔离）: %+v", lei)
	}
	if lei.MAtkPct != 300 {
		t.Fatalf("惊雷殿心法应给 法攻+3%%: %+v", lei)
	}
	// 绝学 80 级才开：1 级时被动里不该出现任何绝学项（本设计绝学无 passive，双保险）。
	if low := CollectPassives(monkSkills(), "金刚禅院", 1); low.HealBonusPct != 0 {
		t.Fatalf("1 级不该有额外被动: %+v", low)
	}
}

// TestNakedMonkReflectsOnFirstHit 1 级裸装禅院：第一次挨打就叠 1 层并按 3% 回弹。
func TestNakedMonkReflectsOnFirstHit(t *testing.T) {
	cfg := formulaCfg()
	cfg.CoeffCaps = config.CoeffCaps{ReflectPct: 5000, HealBonusPct: 6000}
	b := &Battle{F: cfg, rng: flatRng(0.5), Turn: 1}
	atk := mkUnit(1, "甲", 0, 100, 100, 5000)
	def := mkUnit(2, "禅院", 1, 100, 100, 5000)
	def.BaseStats.ReflectPerStack, def.BaseStats.ReflectMaxStacks = 300, 3
	def.Stats = def.BaseStats

	// 第 1 击：叠到 1 层 → 3% → 100×3% = 3。
	b.applyDamage(nil, atk, def, 100, nil)
	if def.ReflectStacks != 1 {
		t.Fatalf("受击应叠 1 层金刚印，实际 %d", def.ReflectStacks)
	}
	if atk.HP != 4997 {
		t.Fatalf("首次受击应反伤 3，攻击方剩 4997，实际 %d", atk.HP)
	}
	// 第 2 击：2 层 → 6%。
	b.applyDamage(nil, atk, def, 100, nil)
	if atk.HP != 4991 {
		t.Fatalf("第二击应反伤 6（累计 9），实际剩 %d", atk.HP)
	}
	// 第 3、4 击：3 层封顶 9%。
	b.applyDamage(nil, atk, def, 100, nil)
	b.applyDamage(nil, atk, def, 100, nil)
	if def.ReflectStacks != 3 {
		t.Fatalf("金刚印应封顶 3 层，实际 %d", def.ReflectStacks)
	}
	if atk.HP != 4973 {
		t.Fatalf("4 击累计反伤 3+6+9+9=27，应剩 4973，实际 %d", atk.HP)
	}
}

// TestReflectSourcesAddUp 四来源并列相加：金刚印 3 层(9%) + 不动明王(25%) + 共鸣微调(3%) = 37%。
func TestReflectSourcesAddUp(t *testing.T) {
	cfg := formulaCfg()
	cfg.CoeffCaps = config.CoeffCaps{ReflectPct: 5000}
	b := &Battle{F: cfg, rng: flatRng(0.5), Turn: 1}
	u := mkUnit(2, "禅院", 1, 100, 100, 5000)
	u.BaseStats.Reflect = 300 // 共鸣微调（禅院二档 +3%）
	u.BaseStats.ReflectPerStack, u.BaseStats.ReflectMaxStacks = 300, 3
	u.Stats = u.BaseStats
	u.ReflectStacks = 3 // 已叠满
	u.Buffs = append(u.Buffs, Buff{Stat: "reflect_add", Value: 2500, Turns: 3})

	if got := b.reflectRate(u); got != 3700 {
		t.Fatalf("反伤合计应为 9%%+25%%+3%%=37%%（3700），实际 %d", got)
	}
	// 上限 50% 仍然生效。
	u.Buffs = append(u.Buffs, Buff{Stat: "reflect_add", Value: 2000, Turns: 3})
	if got := b.reflectRate(u); got != 5000 {
		t.Fatalf("反伤应被 §9.7 上限裁到 50%%（5000），实际 %d", got)
	}
}

// TestLoadFinalStatsNakedMonkPassive 心法被动进属性管线：1 级裸装也有 生命/物防 +5% 与金刚印参数。
func TestLoadFinalStatsNakedMonkPassive(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	acctID, _ := db.Accounts.CreateAccount(ctx, "裸装禅院", "hash", "")
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "行者", "金刚禅院", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	anchor := config.AnchorStat{HP: 1000, MP: 480, PAtk: 100, MAtk: 100, PDef: 50, MDef: 60, Speed: 100}
	cfg := config.Data{
		Items: config.ItemData{BagCapacity: 20},
		Factions: []config.FactionInfo{
			{Name: "金刚禅院", Category: "肉盾", Color: "金", Residence: "少林", Weapon: "拳", Secret: "金刚经"},
		},
		Battle: config.BattleData{
			Classes: []config.BattleClass{{Class: "肉盾", Factions: []string{"金刚禅院"},
				Anchors: map[string]config.AnchorStat{"20": anchor}}},
			ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
			Skills:    monkSkills(),
			Formula:   config.BattleFormula{CoeffCaps: config.CoeffCaps{ReflectPct: 5000, HealBonusPct: 6000}},
		},
	}
	c, err := db.Characters.GetCharacter(ctx, charID)
	if err != nil {
		t.Fatalf("GetCharacter: %v", err)
	}
	base, err := DerivedStatsWithPoints(c.Level, c.Faction, cfg.Battle, nil, nil)
	if err != nil {
		t.Fatalf("派生基线: %v", err)
	}
	fin, err := LoadFinalStats(ctx, &c, db.Equips, cfg)
	if err != nil {
		t.Fatalf("LoadFinalStats: %v", err)
	}
	if fin.Suit.Tier != 0 {
		t.Fatalf("裸装不应有共鸣: %+v", fin.Suit)
	}
	if fin.Passive.ReflectPerStack != 300 || fin.Passive.ReflectMaxStacks != 3 {
		t.Fatalf("1 级裸装就该带金刚印被动: %+v", fin.Passive)
	}
	if fin.Stats.ReflectPerStack != 300 || fin.Stats.ReflectMaxStacks != 3 {
		t.Fatalf("金刚印参数应写进属性: %+v", fin.Stats)
	}
	if want := int(math.Round(float64(base.HP) * 1.05)); fin.Stats.HP != want {
		t.Fatalf("生命应吃心法 +5%%: 期望 %d，实际 %d", want, fin.Stats.HP)
	}
	if want := int(math.Round(float64(base.PDef) * 1.05)); fin.Stats.PDef != want {
		t.Fatalf("物防应吃心法 +5%%: 期望 %d，实际 %d", want, fin.Stats.PDef)
	}
	if fin.Stats.Reflect != 0 {
		t.Fatalf("裸装静态反伤应为 0（反伤靠受击叠层），实际 %d", fin.Stats.Reflect)
	}
}

// TestSkillsFilteredByFaction 多门派秘籍谱合并后，按门派取谱不能串门。
func TestSkillsFilteredByFaction(t *testing.T) {
	svc := &Service{cfg: config.Data{Battle: config.BattleData{Skills: monkSkills()}}}
	monk := svc.skillsForLevel("金刚禅院", 1)
	if len(monk) != 1 || monk[0].ID != "jingang_buhuai" {
		t.Fatalf("1 级禅院应只有心法，实际 %+v", monk)
	}
	if got := svc.skillsForLevel("金刚禅院", 80); len(got) != 2 {
		t.Fatalf("80 级禅院应有心法+绝学，实际 %d 本", len(got))
	}
	lei := svc.skillsForLevel("惊雷殿", 80)
	for _, sk := range lei {
		if sk.Faction != "惊雷殿" {
			t.Fatalf("惊雷殿不该拿到 %s（%s）", sk.ID, sk.Faction)
		}
	}
	if got := svc.skillsForLevel("玄水宫", 80); len(got) != 0 {
		t.Fatalf("未出谱的门派应为空，实际 %d 本", len(got))
	}
}

// TestPlayerUnitCarriesReflectStacks 4/1 出场单位必须带金刚印参数（否则战斗里永远不会叠层）。
func TestPlayerUnitCarriesReflectStacks(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	ctx := context.Background()
	acctID, _ := db.Accounts.CreateAccount(ctx, "出场禅院", "hash", "")
	charID, err := db.Characters.CreateCharacter(ctx, acctID, "护法", "金刚禅院", "男")
	if err != nil {
		t.Fatalf("CreateCharacter: %v", err)
	}
	anchor := config.AnchorStat{HP: 1000, MP: 480, PAtk: 100, MAtk: 100, PDef: 50, MDef: 60, Speed: 100}
	cfg := config.Data{
		Items: config.ItemData{BagCapacity: 20},
		Factions: []config.FactionInfo{
			{Name: "金刚禅院", Category: "肉盾", Color: "金", Residence: "少林", Weapon: "拳", Secret: "金刚经"},
		},
		Battle: config.BattleData{
			Classes: []config.BattleClass{{Class: "肉盾", Factions: []string{"金刚禅院"},
				Anchors: map[string]config.AnchorStat{"20": anchor}}},
			ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
			Skills:    monkSkills(),
			Formula:   config.BattleFormula{CoeffCaps: config.CoeffCaps{ReflectPct: 5000, HealBonusPct: 6000}},
		},
	}
	svc := &Service{chars: db.Characters, equips: db.Equips, cfg: cfg, testUnits: map[string]*Unit{}}
	u, err := svc.playerUnit(ctx, charID)
	if err != nil {
		t.Fatalf("playerUnit: %v", err)
	}
	if u.BaseStats.ReflectPerStack != 300 || u.BaseStats.ReflectMaxStacks != 3 {
		t.Fatalf("出场单位应带金刚印 3%%/层、上限 3: %+v", u.BaseStats)
	}
	if u.ReflectStacks != 0 {
		t.Fatalf("进场层数应为 0，实际 %d", u.ReflectStacks)
	}
	if len(u.Skills) != 1 || u.Skills[0].ID != "jingang_buhuai" {
		t.Fatalf("1 级禅院只应有心法，实际 %+v", u.Skills)
	}
}
