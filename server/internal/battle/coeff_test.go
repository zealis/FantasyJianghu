package battle

import (
	"testing"

	"fantasy-jianghu/server/internal/config"
)

// 数值规范 §9.7 三个角色级系数的结算钩子回归：
// 反伤（不暴击/不连击/不递归、可致死、defender 阵亡也结算）、
// 治疗（施术者口径、衰减在加成之后、溢出截断）、蓄力（仅消耗星辉的攻击秘籍）。

func TestReflectFromCharacterCoefficient(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	atk := mkUnit(1, "甲", 0, 100, 100, 1000)
	def := mkUnit(2, "乙", 1, 100, 100, 1000)
	def.BaseStats.Reflect = 500 // 角色级反伤 5%（管线第 ④⑤ 步已算好）
	def.Stats.Reflect = 500

	evs := b.applyDamage(nil, atk, def, 100, nil)
	if def.HP != 900 {
		t.Fatalf("受击方应扣 100，实际剩 %d", def.HP)
	}
	if atk.HP != 995 {
		t.Fatalf("攻击方应被反伤 100×5%%=5，实际剩 %d", atk.HP)
	}
	var refl *Event
	for i := range evs {
		if evs[i].Type == "reflect" {
			refl = &evs[i]
		}
	}
	if refl == nil {
		t.Fatal("应产出 reflect 事件")
	}
	if refl.Value != 5 || refl.Actor != def.ID || refl.Target != atk.ID {
		t.Fatalf("反伤事件不符: %+v", *refl)
	}
	// 反伤是独立事件，不触发暴击/连击判定（无二次判定字段）。
	for _, f := range []string{"crit", "combo"} {
		if _, has := refl.Flags[f]; has {
			t.Fatalf("反伤不应带 %s 标记: %+v", f, refl.Flags)
		}
	}
}

func TestReflectNotRecursive(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	atk := mkUnit(1, "甲", 0, 100, 100, 1000)
	def := mkUnit(2, "乙", 1, 100, 100, 1000)
	atk.BaseStats.Reflect, atk.Stats.Reflect = 5000, 5000 // 攻击方也有 50% 反伤
	def.BaseStats.Reflect, def.Stats.Reflect = 500, 500

	evs := b.applyDamage(nil, atk, def, 100, nil)
	n := 0
	for _, e := range evs {
		if e.Type == "reflect" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("反伤不得递归，应只有 1 次，实际 %d", n)
	}
}

func TestReflectCanKillAttackerEvenIfDefenderDies(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	atk := mkUnit(1, "甲", 0, 100, 100, 4)
	def := mkUnit(2, "乙", 1, 100, 100, 50)
	def.BaseStats.Reflect, def.Stats.Reflect = 1000, 1000 // 10%

	evs := b.applyDamage(nil, atk, def, 100, nil)
	if def.Alive {
		t.Fatal("受击方应阵亡")
	}
	if atk.HP != 0 || atk.Alive {
		t.Fatalf("反伤 100×10%%=10 应打死攻击方（HP 4），实际 HP=%d alive=%v", atk.HP, atk.Alive)
	}
	deaths := 0
	for _, e := range evs {
		if e.Type == "death" {
			deaths++
		}
	}
	if deaths != 2 {
		t.Fatalf("双方阵亡应记 2 条死亡事件，实际 %d", deaths)
	}
}

func TestHealBonusAppliedThenDecay(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	healer := mkUnit(1, "医", 0, 100, 100, 1000)
	healer.BaseStats.HealBonus, healer.Stats.HealBonus = 800, 800 // +8%
	target := mkUnit(2, "乙", 0, 100, 100, 1000)
	target.HP = 500

	evs := b.applyHeal(nil, healer, target, 100, nil)
	// 100 × 1.08 × 1.00 = 108（衰减在加成之后，第 1 回合为 1.0）。
	if target.HP != 608 {
		t.Fatalf("治疗应为 100×1.08=108，实际 %d", target.HP-500)
	}
	if len(evs) != 1 || evs[0].Type != "heal" || evs[0].Value != 108 {
		t.Fatalf("治疗事件不符: %+v", evs)
	}

	// 第 31 回合：治疗衰减 0.65，顺序必须是先加成后衰减 → 108×0.65=70。
	b.Turn = 31
	target.HP = 500
	b.applyHeal(nil, healer, target, 100, nil)
	if target.HP != 570 {
		t.Fatalf("第 31 回合治疗应为 floor(108×0.65)=70，实际 %d", target.HP-500)
	}
}

func TestHealUsesCasterBonusNotTarget(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	healer := mkUnit(1, "医", 0, 100, 100, 1000) // 施术者无加成
	target := mkUnit(2, "乙", 0, 100, 100, 1000)
	target.BaseStats.HealBonus, target.Stats.HealBonus = 5000, 5000 // 受疗者加成不得生效
	target.HP = 500

	b.applyHeal(nil, healer, target, 100, nil)
	if target.HP != 600 {
		t.Fatalf("治疗只看施术者加成，应为 100，实际 %d", target.HP-500)
	}
}

func TestHealOverhealCapped(t *testing.T) {
	b := &Battle{F: formulaCfg(), rng: flatRng(0.5), Turn: 1}
	healer := mkUnit(1, "医", 0, 100, 100, 1000)
	target := mkUnit(2, "乙", 0, 100, 100, 1000)
	target.HP = 950

	evs := b.applyHeal(nil, healer, target, 200, nil)
	if target.HP != 1000 {
		t.Fatalf("溢出治疗应截断至上限 1000，实际 %d", target.HP)
	}
	if evs[0].Value != 50 {
		t.Fatalf("事件应记实际治疗量 50，实际 %d", evs[0].Value)
	}
	if evs[0].Flags["overheal"] != 150 {
		t.Fatalf("应登记溢出 150，实际 %v", evs[0].Flags["overheal"])
	}
}

func TestChargeDamageOnlyForStarConsumingSkills(t *testing.T) {
	cfg := formulaCfg()
	skills := []config.SkillDef{
		{ID: "plain", Name: "普通法术", Category: "单体", MPCost: 10, AttackType: "magic",
			Segments: []config.SkillSegment{{Mult: 100}}, Targets: 1},
		{ID: "charged", Name: "蓄力法术", Category: "单体", MPCost: 10, AttackType: "magic",
			Segments: []config.SkillSegment{{Mult: 100}}, Targets: 1, ConsumesStar: true},
	}
	run := func(sk config.SkillDef, charge int) int {
		b := &Battle{F: cfg, rng: flatRng(0.5), Turn: 1}
		// 固定不暴击、不连击，排除乘区干扰。
		actor := mkUnit(1, "甲", 0, 1422, 376, 5000)
		actor.BaseStats.MCrit, actor.BaseStats.PCrit, actor.BaseStats.PCombo = 0, 0, 0
		actor.BaseStats.ChargeDmg, actor.Stats.ChargeDmg = charge, charge
		tgt := mkUnit(2, "乙", 1, 100, 376, 100000)
		evs := b.executeSkill(actor, []*Unit{tgt}, sk, 0)
		for _, e := range evs {
			if e.Type == "damage" {
				return e.Value
			}
		}
		t.Fatalf("%s 未产出伤害", sk.ID)
		return 0
	}
	base := run(skills[0], 3000) // 不消耗星辉：蓄力系数不得生效
	if got := run(skills[0], 0); got != base {
		t.Fatalf("非星辉秘籍不应吃蓄力加成：%d vs %d", got, base)
	}
	charged := run(skills[1], 3000)
	// 乘子作用在浮点原值上再截断，允许 ±1 的取整差。
	if charged < int(float64(base)*1.30)-1 || charged > int(float64(base)*1.30)+1 {
		t.Fatalf("消耗星辉的秘籍应 ×1.30（约 %d），实际 %d", int(float64(base)*1.30), charged)
	}
	if got := run(skills[1], 0); got != base {
		t.Fatalf("系数为 0 时蓄力秘籍应与普通一致：%d vs %d", got, base)
	}
}
