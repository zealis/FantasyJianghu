package battle

import (
	"encoding/json"
	"math"
	"testing"

	"fantasy-jianghu/server/internal/config"
)

// mockRng 确定性随机源：按预设序列返回 [0,1) 值（浮动/命中/暴击判定共用）。
// 序列耗尽后循环。
type mockRng struct {
	seq []float64
	i   int
}

func (m *mockRng) next() float64 {
	v := m.seq[m.i%len(m.seq)]
	m.i++
	return v
}
func (m *mockRng) Float64() float64 { return m.next() }
func (m *mockRng) Intn(n int) int   { return int(m.next() * float64(n)) }

func formulaCfg() config.BattleFormula {
	// 与 configs/source/battle/formula.json 同口径的内存副本。
	return config.BattleFormula{
		Damage: config.DamageParams{K: 1.0, FloatLow: 0.95, FloatHigh: 1.05, MinDamage: 1, LevelDiffPerLevel: 0.015, LevelDiffClamp: 0.30, ArmorBreakPerStack: 0.03, ArmorBreakMaxStacks: 5},
		Crit:   config.CritParams{BaseRate: 0.05, Damage: 1.50},
		Combo: struct {
			ExtraMult float64 `json:"extra_mult"`
		}{ExtraMult: 0.50},
		MultiplierCap: 2.25,
		Hit:           config.HitParams{PhysicalBase: 0.95, ClampLow: 0.60, ClampHigh: 1.00, MagicDodgeBase: 0.10},
		Control:       config.ControlParams{SealResBase: 0.10, ComboDecay: 0.70, ImmuneTurns: 1, HardMaxTurns: 2, HardTypes: []string{"麻痹", "定身", "沉睡", "冰冻", "混乱"}},
		Decay: []config.DecayStep{
			{From: 1, To: 20, Dmg: 1.0, Heal: 1.0},
			{From: 21, To: 30, DmgStep: 0.05, HealStep: -0.03},
			{From: 31, DmgStep: 0.10, HealStep: -0.05, HealFloor: 0.20},
		},
		SpeedJitter:       0.03,
		MaxTurns:          50,
		CmdTimeoutSeconds: 30,
		Effects: config.EffectsParams{
			Shock:   config.ShockParams{MaxStacks: 3, DmgTakenPctPerStack: 400, DOTPctOfMAtkPerStack: 300, DOTTurns: 3, BonusVsShockedPct: 1000},
			Reflect: config.ReflectParams{PerStackPct: 300, MaxPct: 1500, MinDamage: 1},
			Dot:     config.DotParams{MinDamage: 1},
		},
		Drill: config.DrillParams{
			Level: 20, Allies: 3, Enemies: []string{"muzhuang", "yelinang", "shanzei", "jingji_wushi"},
			AllyHPPct: 8000, AllyAtkPct: 8000, AllySpeedPct: 9000,
			EnemyHPPctOfPlayer: 6000, EnemyAtkPctOfPlayer: 4000,
			EnemyDefPctOfPlayer: 5000, EnemySpeedPctOfPlayer: 9000,
		},
		SpecialsDeferred: []string{"shock_jump_50", "shock_detonate_25"},
	}
}

func mkUnit(id int64, name string, side int, atk, def, hp int) *Unit {
	return &Unit{
		ID: id, Name: name, Side: side, Level: 20,
		BaseStats: UnitStats{HP: hp, MP: 1000, PAtk: atk, MAtk: atk, PDef: def, MDef: def, Speed: 1000},
		Stats:     UnitStats{HP: hp, MP: 1000, PAtk: atk, MAtk: atk, PDef: def, MDef: def, Speed: 1000},
		HP:        hp, MP: 1000, Alive: true,
	}
}

// flatRng 恒定返回 v（浮动=0.95+0.1v、命中/暴击判定 f<v 判真）。
func flatRng(v float64) *mockRng { return &mockRng{seq: []float64{v}} }

func TestDamageHandCalc(t *testing.T) {
	cfg := formulaCfg()
	// 攻 1422 防 376 倍率 100% 压制 0 浮动 1.0：1422²/(1422+376) = 1124（截断）。
	b := &Battle{F: cfg, rng: flatRng(0.5)}
	a := mkUnit(1, "甲", 0, 1422, 376, 5000)
	d := mkUnit(2, "乙", 1, 1000, 376, 5000)
	got := b.damageRoll(a, d, 1.0, 0, nil)
	if got != 1124 {
		t.Fatalf("伤害 = %d, 期望 1124（1422²/1798 截断）", got)
	}
}

func TestDamageFloor(t *testing.T) {
	cfg := formulaCfg()
	b := &Battle{F: cfg, rng: flatRng(0.5)}
	a := mkUnit(1, "甲", 0, 10, 0, 100)
	d := mkUnit(2, "乙", 1, 1000, 9999, 100000)
	if got := b.damageRoll(a, d, 1.0, 0, nil); got != 1 {
		t.Fatalf("下限伤害 = %d, 期望 1", got)
	}
}

func TestMultiplierCap(t *testing.T) {
	cfg := formulaCfg()
	// rng 消耗序：命中→暴击→连击→浮动；序列 0.5/0.01/0.01/0.5 → 必中+暴击+连击+浮动 1.0。
	b := &Battle{F: cfg, rng: &mockRng{seq: []float64{0.5, 0.01, 0.01, 0.5}}}
	a := mkUnit(1, "甲", 0, 1422, 0, 5000)
	a.BaseStats.PCrit = 10000  // 100% 暴击来源
	a.BaseStats.PCombo = 10000 // 100% 连击来源（乘区合并 1.5×2=3.0 → 钳 2.25）
	d := mkUnit(2, "乙", 1, 1000, 376, 500000)
	// 乘区作用于未取整原始值（取整只在最终一次）：1422²/1798 × 2.25 = 2530。
	raw := 1422.0 * 1422.0 / (1422.0 + 376.0)
	want := int(math.Floor(raw * 2.25))
	if got := b.damageRoll(a, d, 1.0, 0, nil); got != want {
		t.Fatalf("乘区钳制伤害 = %d, 期望 %d", got, want)
	}
}

func TestHitClamp(t *testing.T) {
	cfg := formulaCfg()
	// 命中差极大 → 钳 100%；极小 → 钳 60%。
	if got := hitChance(99999, 0, cfg); mathAbs(got-1.0) > 1e-9 {
		t.Fatalf("命中上限 = %v", got)
	}
	if got := hitChance(0, 99999, cfg); mathAbs(got-0.60) > 1e-9 {
		t.Fatalf("命中下限 = %v", got)
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestControlDecayAndImmune(t *testing.T) {
	cfg := formulaCfg()
	b := &Battle{F: cfg, rng: flatRng(0.01), Turn: 1}
	tgt := mkUnit(2, "乙", 1, 100, 100, 1000)
	// 基础 70% + 0 封印命中 × (1-10% 抗性) = 63% × 压制 1.0 = 63%；f=0.01 < 63% 必中。
	if !b.controlRoll("麻痹", 70, nil, tgt, 0) {
		t.Fatal("首次控制应命中")
	}
	// 连控递减 ×0.7：第二次 44.1% 仍命中（f=0.01）。
	b.Turn = 2
	if !b.controlRoll("麻痹", 70, nil, tgt, 0) {
		t.Fatal("递减后仍应命中")
	}
	// 免疫窗：硬控结束当回合后 1 回合内同类必失败（f=0.01 但免疫直接 false）。
	b.Turn = 3
	tgt.LastHardEndTurn = 2 // 第 2 回合硬控结束 → 第 3 回合免疫
	if b.controlRoll("麻痹", 70, nil, tgt, 0) {
		t.Fatal("免疫窗内同类硬控应失败（Focus 3）")
	}
	// 封技能不属硬控免疫范围。
	if !b.controlRoll("封技能", 70, nil, tgt, 0) {
		t.Fatal("封技能不受硬控免疫窗限制")
	}
}

func TestMPFallback(t *testing.T) {
	cfg := formulaCfg()
	b := &Battle{F: cfg, rng: flatRng(0.5)}
	a := mkUnit(1, "甲", 0, 1422, 376, 5000)
	d := mkUnit(2, "乙", 1, 1000, 376, 5000)
	a.MP = 10 // 不足 150
	skill := config.SkillDef{ID: "tianlei_po", Name: "天雷破", Category: "单体", MPCost: 150, AttackType: "magic", Segments: []config.SkillSegment{{Mult: 160}}, Targets: 1}
	a.Skills = []config.SkillDef{skill}
	evs := b.executeSkill(a, []*Unit{d}, skill, 0)
	fallback := false
	for _, e := range evs {
		if e.Flags["回退"] == true {
			fallback = true
		}
	}
	if !fallback {
		t.Fatalf("真气不足应回退普攻并标记: %+v", evs)
	}
}

func TestLongBattleDecay(t *testing.T) {
	cfg := formulaCfg()
	b := &Battle{F: cfg, rng: flatRng(0.5)}
	// 第 21 回合：伤害 ×1.05；第 31 回合：×1.05 + 10×0.10 = ×1.15 累计。
	if got := b.decayDmg(21); mathAbsF(got-1.05) > 1e-9 {
		t.Fatalf("第 21 回合伤害修正 = %v, 期望 1.05", got)
	}
	if got := b.decayDmg(31); mathAbsF(got-1.60) > 1e-9 {
		t.Fatalf("第 31 回合伤害修正 = %v, 期望 1.60（21-30 累计 +0.5，31 起再 +0.10）", got)
	}
	if got := b.decayHeal(31); mathAbsF(got-0.65) > 1e-9 {
		t.Fatalf("第 31 回合治疗修正 = %v, 期望 0.65（1-0.03×10-0.05）", got)
	}
	// 治疗下限 0.20（第 51 回合起）。
	if got := b.decayHeal(51); mathAbsF(got-0.20) > 1e-9 {
		t.Fatalf("治疗下限 = %v, 期望 0.20", got)
	}
}

func mathAbsF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestPriorityDefendFirst(t *testing.T) {
	cfg := formulaCfg()
	// 同回合：乙（速度快）选择普攻，甲（速度慢）选择防御 → 防御先结算，甲受伤减半。
	b := &Battle{F: cfg, rng: flatRng(0.5), Turn: 1}
	a := mkUnit(1, "甲", 0, 1422, 376, 5000)
	d := mkUnit(2, "乙", 1, 1422, 376, 5000)
	d.Stats.Speed = 5000 // 乙先手（速度序）
	b.Units = []*Unit{a, d}
	b.cmds = map[int64]Cmd{1: {Type: "defend"}, 2: {Type: "attack"}}
	hpBefore := a.HP
	evs := b.ResolveTurn()
	defFound := false
	for _, e := range evs {
		if e.Type == "defend" && e.Actor == 1 {
			defFound = true
		}
	}
	if !defFound {
		t.Fatalf("应有防御事件: %+v", evs)
	}
	if a.HP >= hpBefore {
		t.Fatal("乙应命中甲")
	}
	// 防御减半：受伤 = 基础/2（浮动 1.0、无暴击）。
	base := 1422 * 1422 / (1422 + 376)
	if a.HP != hpBefore-base/2 {
		t.Fatalf("防御减伤后 HP = %d, 期望 %d（%d - %d/2）", a.HP, hpBefore-base/2, hpBefore, base)
	}
}

// ---- 独立评审回归：衰减接线 / 秘籍防御 / 暴击标记 / 封印递减 ----

func deterministicBattle(cfg config.BattleFormula, actor, target *Unit) *Battle {
	b := NewBattle(1, []*Unit{actor}, []*Unit{target}, config.BattleData{Formula: cfg})
	b.SetRng(flatRng(0.0)) // 命中/不暴击/不连击/浮动 0.95 全确定
	return b
}

// TestSkillRespectsDefend 防御指令对秘籍同样生效（旧实现只减半普攻）。
func TestSkillRespectsDefend(t *testing.T) {
	cfg := formulaCfg()
	skill := config.SkillDef{ID: "s", Name: "试招", Category: "单体", MPCost: 0,
		AttackType: "magic", Segments: []config.SkillSegment{{Mult: 100}}}
	run := func(defend bool) int {
		a := mkUnit(1, "甲", 0, 1000, 100, 5000)
		a.Skills = []config.SkillDef{skill}
		d := mkUnit(2, "乙", 1, 1000, 100, 5000)
		d.Defending = defend
		b := deterministicBattle(cfg, a, d)
		b.SubmitCmd(1, Cmd{Type: "skill", SkillID: "s"})
		b.SubmitCmd(2, Cmd{Type: "attack"})
		for _, e := range b.ResolveTurn() {
			if e.Type == "damage" && e.Actor == 1 {
				return e.Value
			}
		}
		return 0
	}
	normal, defended := run(false), run(true)
	if normal <= 0 || defended <= 0 {
		t.Fatalf("两次结算都应产出伤害: normal=%d defended=%d", normal, defended)
	}
	if defended < normal/2-2 || defended > normal/2+2 {
		t.Fatalf("秘籍伤害应被防御减半: normal=%d defended=%d", normal, defended)
	}
}

// TestCritFlagEmitted 暴击必须下发 crit 标记，否则客户端红色大字飘字永远不触发。
func TestCritFlagEmitted(t *testing.T) {
	a := mkUnit(1, "甲", 0, 1422, 376, 99999)
	a.BaseStats.PCrit = 10000 // 必暴击
	d := mkUnit(2, "乙", 1, 1000, 376, 99999)
	b := deterministicBattle(formulaCfg(), a, d)
	b.SubmitCmd(1, Cmd{Type: "attack"})
	b.SubmitCmd(2, Cmd{Type: "attack"})
	found := false
	for _, e := range b.ResolveTurn() {
		if e.Type == "damage" && e.Actor == 1 {
			found = true
			if e.Flags["crit"] != true {
				t.Fatalf("暴击伤害缺 crit 标记: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("未产出伤害事件")
	}
}

// TestLongBattleDecayApplied 长局衰减必须真的作用于伤害，且系数来自配置而非硬编码。
func TestLongBattleDecayApplied(t *testing.T) {
	run := func(turn int, cfg config.BattleFormula) int {
		a := mkUnit(1, "甲", 0, 1422, 376, 99999)
		d := mkUnit(2, "乙", 1, 1000, 376, 99999)
		b := deterministicBattle(cfg, a, d)
		b.Turn = turn - 1 // ResolveTurn 会 ++
		b.SubmitCmd(1, Cmd{Type: "attack"})
		b.SubmitCmd(2, Cmd{Type: "attack"})
		for _, e := range b.ResolveTurn() {
			if e.Type == "damage" && e.Actor == 1 {
				return e.Value
			}
		}
		return 0
	}
	cfg := formulaCfg()
	d1, d21 := run(1, cfg), run(21, cfg)
	if d1 <= 0 || float64(d21) < float64(d1)*1.04 {
		t.Fatalf("第 21 回合应享受 +5%% 伤害衰减加成: t1=%d t21=%d", d1, d21)
	}
	cfg2 := formulaCfg()
	cfg2.Decay[1].DmgStep = 0.10
	if d21b := run(21, cfg2); d21b <= d21 {
		t.Fatalf("改配置后伤害未变化（衰减仍是硬编码）: %d -> %d", d21, d21b)
	}
}

// TestSealedTurnsDecaysRegardlessOfCmd 封技能回合数按回合递减，与玩家选什么指令无关。
func TestSealedTurnsDecaysRegardlessOfCmd(t *testing.T) {
	a := mkUnit(1, "甲", 0, 1422, 376, 99999)
	a.SealedTurns = 2
	d := mkUnit(2, "乙", 1, 1000, 376, 99999)
	b := deterministicBattle(formulaCfg(), a, d)
	b.SubmitCmd(1, Cmd{Type: "attack"})
	b.SubmitCmd(2, Cmd{Type: "attack"})
	b.ResolveTurn()
	if a.SealedTurns != 1 {
		t.Fatalf("选普攻时封技能回合数也应递减: %d", a.SealedTurns)
	}
}

// ---- M2a-2 回归：感电 DOT / 反伤 / 群攻 / 行动序 / 种子重演 ----

func shockSkill() config.SkillDef {
	return config.SkillDef{ID: "s", Name: "试雷法", Category: "单体", MPCost: 0,
		AttackType: "magic", Segments: []config.SkillSegment{{Mult: 100}}, Targets: 1}
}

func TestShockStacksDOTAndCap(t *testing.T) {
	a := mkUnit(1, "雷法", 0, 1000, 100, 99999)
	a.Skills = []config.SkillDef{shockSkill()}
	d := mkUnit(2, "木桩", 1, 100, 100, 99999)
	b := deterministicBattle(formulaCfg(), a, d)
	var all []Event
	for i := 0; i < 4 && !b.Over; i++ {
		b.SubmitCmd(1, Cmd{Type: "skill", SkillID: "s"})
		b.SubmitCmd(2, Cmd{Type: "attack"})
		all = append(all, b.ResolveTurn()...)
	}
	if d.ShockStacks != 3 {
		t.Fatalf("感电层数应封顶 3, 实得 %d", d.ShockStacks)
	}
	shocks, dots, maxDOT := 0, 0, 0
	for _, e := range all {
		switch e.Type {
		case "shock":
			shocks++
		case "dot":
			dots++
			if e.Value > maxDOT {
				maxDOT = e.Value
			}
		}
	}
	if shocks != 4 || dots == 0 {
		t.Fatalf("应产出 4 个 shock 与至少 1 个 dot: shocks=%d dots=%d", shocks, dots)
	}
	// 1000 法攻 × 3 层 × 3% = 90 / 回合。
	if maxDOT != 90 {
		t.Fatalf("3 层 DOT 期望 90, 实得 %d", maxDOT)
	}
}

func TestReflectDamage(t *testing.T) {
	a := mkUnit(1, "甲", 0, 1422, 376, 99999)
	d := mkUnit(2, "荆棘", 1, 1000, 376, 99999)
	d.ReflectPct = 900 // 9%
	b := deterministicBattle(formulaCfg(), a, d)
	b.SubmitCmd(1, Cmd{Type: "attack"})
	b.SubmitCmd(2, Cmd{Type: "attack"})
	dmg, ref := 0, 0
	for _, e := range b.ResolveTurn() {
		if e.Type == "damage" && e.Actor == 1 {
			dmg = e.Value
		}
		if e.Type == "reflect" {
			ref = e.Value
		}
	}
	if dmg == 0 || ref == 0 {
		t.Fatalf("应产出伤害与反伤: dmg=%d reflect=%d", dmg, ref)
	}
	want := int(math.Floor(float64(dmg) * 0.09))
	if ref != want {
		t.Fatalf("反伤 = %d, 期望 %d（不触发二次判定）", ref, want)
	}
	if a.HP >= 99999 {
		t.Fatal("反伤应扣除攻击者 HP")
	}
}

func TestShockVulnerabilityAndSpecial(t *testing.T) {
	a := mkUnit(1, "甲", 0, 1000, 100, 99999)
	d := mkUnit(2, "乙", 1, 1000, 100, 99999)
	b := deterministicBattle(formulaCfg(), a, d)
	plain := b.damageRollTyped(a, d, 1.0, 0, "magic", false).value
	d.ShockStacks = 3
	boosted := b.damageRollTyped(a, d, 1.0, 0, "magic", false).value
	if float64(boosted) < float64(plain)*1.11 {
		t.Fatalf("3 层感电易伤应 +12%%: plain=%d boosted=%d", plain, boosted)
	}
	run := func(special string) int {
		actor := mkUnit(1, "甲", 0, 1000, 100, 99999)
		actor.MP = 99999
		sk := shockSkill()
		sk.Special = special
		actor.Skills = []config.SkillDef{sk}
		tgt := mkUnit(2, "乙", 1, 1000, 100, 99999)
		bt := deterministicBattle(formulaCfg(), actor, tgt)
		tgt.ShockStacks = 3 // NewBattle 会清空对局内状态，需在构造后注入
		for _, e := range bt.executeSkill(actor, []*Unit{tgt}, sk, 0) {
			if e.Type == "damage" {
				return e.Value
			}
		}
		return 0
	}
	if withBonus, without := run("shock_bonus_10"), run(""); withBonus <= without {
		t.Fatalf("怒雷天罚对感电目标应额外 +10%%: %d vs %d", withBonus, without)
	}
}

func TestMultiTargetSkillAndOrderEvent(t *testing.T) {
	cfg := formulaCfg()
	a := mkUnit(1, "甲", 0, 1000, 100, 99999)
	a.MP = 99999
	sk := shockSkill()
	sk.Targets = 2
	a.Skills = []config.SkillDef{sk}
	d1 := mkUnit(2, "乙1", 1, 1000, 100, 99999)
	d2 := mkUnit(3, "乙2", 1, 1000, 100, 99999)
	b := &Battle{F: cfg, rng: flatRng(0.0), Units: []*Unit{a, d1, d2}}
	evs := b.executeSkill(a, []*Unit{d1, d2}, sk, 0)
	if d1.HP >= 99999 || d2.HP >= 99999 {
		t.Fatalf("群攻应同时命中两个目标: %d / %d", d1.HP, d2.HP)
	}
	for _, e := range evs {
		if e.Type == "act" && e.Flags["targets"] != 2 {
			t.Fatalf("act 事件应标记 2 目标: %+v", e.Flags)
		}
	}
	// 4v4 行动顺序条：order 事件携带全部 8 个单位的 id/name/slot。
	var sideA, sideB []*Unit
	for i := 0; i < 4; i++ {
		u := mkUnit(int64(10+i), "我"+string(rune('A'+i)), 0, 1000, 1000, 999999)
		u.Slot = i
		sideA = append(sideA, u)
		v := mkUnit(int64(20+i), "敌"+string(rune('A'+i)), 1, 1000, 1000, 999999)
		v.Slot = i
		sideB = append(sideB, v)
	}
	bt := NewBattle(7, sideA, sideB, config.BattleData{Formula: cfg})
	bt.SetRng(flatRng(0.0))
	for _, u := range bt.Units {
		bt.SubmitCmd(u.ID, Cmd{Type: "attack"})
	}
	found := false
	for _, e := range bt.ResolveTurn() {
		if e.Type == "order" {
			found = true
			ids, _ := e.Flags["ids"].([]int64)
			if len(ids) != 8 {
				t.Fatalf("order 应含 8 个单位, 实得 %d", len(ids))
			}
		}
	}
	if !found {
		t.Fatal("4v4 回合缺少 order 行动顺序事件")
	}
}

func TestDOTCanEndBattle(t *testing.T) {
	a := mkUnit(1, "甲", 0, 1000, 100, 99999)
	d := mkUnit(2, "乙", 1, 100, 100, 50)
	b := deterministicBattle(formulaCfg(), a, d)
	d.ShockStacks, d.ShockDOTTurns, d.ShockDOTPower = 1, 3, 200
	b.SubmitCmd(1, Cmd{Type: "defend"})
	b.SubmitCmd(2, Cmd{Type: "attack"})
	ended := false
	for _, e := range b.ResolveTurn() {
		if e.Type == "end" {
			ended = true
		}
	}
	if !ended || !b.Over {
		t.Fatal("DOT 致死必须触发终局（回合末统一检查）")
	}
}

func TestSeedReplayWithEffects(t *testing.T) {
	run := func(seed int64) []Event {
		cfg := config.BattleData{Formula: formulaCfg()}
		a := mkUnit(1, "甲", 0, 1000, 100, 999999)
		a.MP = 999999
		a.Skills = []config.SkillDef{shockSkill()}
		d := mkUnit(2, "乙", 1, 1000, 100, 999999)
		d.ReflectPct = 900
		b := NewBattle(seed, []*Unit{a}, []*Unit{d}, cfg)
		var evs []Event
		for i := 0; i < 6 && !b.Over; i++ {
			b.SubmitCmd(1, Cmd{Type: "skill", SkillID: "s"})
			b.SubmitCmd(2, Cmd{Type: "attack"})
			evs = append(evs, b.ResolveTurn()...)
		}
		return evs
	}
	toJSON := func(e []Event) string { raw, _ := json.Marshal(e); return string(raw) }
	if toJSON(run(20261109)) != toJSON(run(20261109)) {
		t.Fatal("含感电/反伤的战斗同种子重演不一致（复盘口径）")
	}
	if toJSON(run(20261109)) == toJSON(run(777)) {
		t.Fatal("不同种子事件流不应完全一致")
	}
}

// TestDamageFlags 钉死 V1.28 飘字前缀的数据来源：只登记真实判定的特效，
// 吸血等 M2c 词条未实现，不得伪造 flag。
func TestDamageFlags(t *testing.T) {
	if got := damageFlags(damageOutcome{value: 10}, false); got != nil {
		t.Fatalf("无特效不应产生 flags: %+v", got)
	}
	got := damageFlags(damageOutcome{value: 10, crit: true, combo: true}, true)
	for _, k := range []string{"crit", "combo", "magic"} {
		if v, ok := got[k].(bool); !ok || !v {
			t.Fatalf("缺 flag %s: %+v", k, got)
		}
	}
	if _, ok := got["lifesteal"]; ok {
		t.Fatal("吸血未实现，不得伪造 lifesteal flag")
	}
}

// TestSealHitEntersControlFormula 封印命中必须真正进公式（V1.29 套装符观 +10% 的落点）：
// 基础 70% × (1−10% 抗性) = 63%，随机 65% 时无封印命中应失败；+10% 后 72% 应命中。
func TestSealHitEntersControlFormula(t *testing.T) {
	cfg := formulaCfg()
	b := &Battle{F: cfg, rng: flatRng(0.65), Turn: 1}
	tgt := mkUnit(2, "乙", 1, 100, 100, 1000)
	tgt.BaseStats.SealRes = 1000 // 10% 抗性
	caster := mkUnit(1, "甲", 0, 100, 100, 1000)
	if b.controlRoll("封技能", 70, nil, tgt, 0) {
		t.Fatal("63% 成功率在随机 65% 时不应命中")
	}
	tgt.ControlCount = 0
	caster.BaseStats.SealHit = 1000 // 封印命中 +10%（万分制）
	if !b.controlRoll("封技能", 70, caster, tgt, 0) {
		t.Fatal("封印命中 +10% 后 (70+10)%×0.9=72% 应命中 65% 随机")
	}
}
