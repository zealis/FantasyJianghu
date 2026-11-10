package battle

import (
	"math"
	"math/rand"

	"fantasy-jianghu/server/internal/config"
)

// Cmd 为参战者指令（§2.1：普攻/秘籍/防御/逃跑；物品/法宝留后续）。
// TargetID 为 4v4 目标选择；0 表示服务端默认（存活敌方首个）。
type Cmd struct {
	Type     string // attack|skill|defend|flee
	SkillID  string
	TargetID int64
}

// Buff 为增益/削弱（万分制；回合末递减）。
type Buff struct {
	Stat  string // patk_pct|matk_pct|pdef_pct|mdef_pct|speed_pct
	Value int
	Turns int
}

// Unit 为战斗单位。
type Unit struct {
	ID              int64
	Name            string
	Side            int // 0 我方 1 敌方
	Level           int
	Faction         string
	BaseStats       UnitStats // 派生基础（不含 Buff）
	Stats           UnitStats // 当前生效快照（结算时重算）
	HP, MP          int
	Skills          []config.SkillDef
	Buffs           []Buff
	SealedTurns     int // 封技能剩余回合
	HardTurns       int // 硬控剩余回合（失去行动）
	LastHardEndTurn int // 最近一次硬控结束回合（免疫窗）
	ControlCount    int // 连控次数（递减）
	Defending       bool
	Hosted          bool // 本回合被托管（战报标记）
	Alive           bool
	Slot            int // 4v4 站位槽（0..3，客户端按此布局）
	ShockStacks     int // M2a-2 感电层数（上限取 Effects.Shock.MaxStacks）
	ShockDOTTurns   int // 感电 DOT 剩余回合
	ShockDOTPower   int // 每回合 DOT 基准（施加者法攻 × 层数 × 系数，施加时快照）
	ReflectPct      int // 单位自身反伤万分制（怪物/法宝给定值；数值规范 §2.4）
	ReflectStacks   int // 金刚印层数（心法被动，受击叠层，上限取 Stats.ReflectMaxStacks）
}

// Event 为战报事件（客户端播放的最小单元）。
type Event struct {
	Turn   int            `json:"turn"`
	Seq    int            `json:"seq"`
	Type   string         `json:"type"` // damage|heal|miss|control|buff|defend|flee|flee_fail|fallback|act|death|turn_end|end|shock|dot|reflect|order
	Actor  int64          `json:"actor,omitempty"`
	Target int64          `json:"target,omitempty"`
	Value  int            `json:"value,omitempty"`
	Flags  map[string]any `json:"flags,omitempty"`
}

// rngSrc 为可注入随机源（生产 rand.Rand；测试 mock）。
type rngSrc interface{ Float64() float64 }

// Battle 为一场战斗（纯状态机：输入指令 → 输出事件流；种子驱动可重演）。
type Battle struct {
	Seed  int64
	rng   rngSrc
	F     config.BattleFormula
	Units []*Unit
	Turn  int
	cmds  map[int64]Cmd
	seq   int
	Over  bool
}

// NewBattle 以种子创建战斗（同种子+同指令流 → 同事件流，复盘口径）。
func NewBattle(seed int64, sideA, sideB []*Unit, cfg config.BattleData) *Battle {
	for _, u := range append(append([]*Unit{}, sideA...), sideB...) {
		u.HP = u.BaseStats.HP
		u.MP = u.BaseStats.MP
		u.Alive = true
		u.Stats = u.BaseStats
		// Slot/ReflectPct 属于单位定义，保留；感电状态属于对局内状态，重演前清零。
		u.ShockStacks, u.ShockDOTTurns, u.ShockDOTPower = 0, 0, 0
		u.ReflectStacks = 0
	}
	return &Battle{
		Seed:  seed,
		rng:   rand.New(rand.NewSource(seed)),
		F:     cfg.Formula,
		Units: append(append([]*Unit{}, sideA...), sideB...),
	}
}

// SetRng 注入确定性随机源（测试用；生产为 seed 驱动）。
func (b *Battle) SetRng(r rngSrc) { b.rng = r }

func (b *Battle) f64() float64 { return b.rng.Float64() }

func (b *Battle) emit(evs []Event, e Event) []Event {
	b.seq++
	e.Seq = b.seq
	e.Turn = b.Turn
	return append(evs, e)
}

func (b *Battle) unit(id int64) *Unit {
	for _, u := range b.Units {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (b *Battle) alive(side int) []*Unit {
	var out []*Unit
	for _, u := range b.Units {
		if u.Alive && u.Side == side {
			out = append(out, u)
		}
	}
	return out
}

// effectiveStats 重算含 Buff 的生效属性（§1.3 管线：扁平/百分比加成）。
func effectiveStats(u *Unit) UnitStats {
	s := u.BaseStats
	for _, bf := range u.Buffs {
		switch bf.Stat {
		case "patk_pct":
			s.PAtk = int(math.Round(float64(s.PAtk) * (1 + float64(bf.Value)/10000)))
		case "matk_pct":
			s.MAtk = int(math.Round(float64(s.MAtk) * (1 + float64(bf.Value)/10000)))
		case "pdef_pct":
			s.PDef = int(math.Round(float64(s.PDef) * (1 + float64(bf.Value)/10000)))
		case "mdef_pct":
			s.MDef = int(math.Round(float64(s.MDef) * (1 + float64(bf.Value)/10000)))
		case "speed_pct":
			s.Speed = int(math.Round(float64(s.Speed) * (1 + float64(bf.Value)/10000)))
		case "reflect_add":
			// 绝学不动明王：反伤 +25%（按百分点相加，万分制），与心法/共鸣来源并列。
			s.Reflect += bf.Value
		}
	}
	return s
}

// hitChance 物理命中率（首版换算：基线 + 万分差×0.0001，钳 60%–100%，公式表 §3）。
func hitChance(pHit, pDodge int, f config.BattleFormula) float64 {
	rate := f.Hit.PhysicalBase + float64(pHit-pDodge)*0.0001
	if rate > f.Hit.ClampHigh {
		return f.Hit.ClampHigh
	}
	if rate < f.Hit.ClampLow {
		return f.Hit.ClampLow
	}
	return rate
}

// damageOutcome 为单段伤害结算结果（crit 供客户端飘字分色）。
type damageOutcome struct {
	value int
	crit  bool
	combo bool
}

// damageFlags 把本段已判定的特效打包成战报 flags，供客户端飘字加单字前缀
// （V1.28 UI 规范 §5 演出反馈）。只登记真实发生的效果：吸血等词条 M2c 未实现，不伪造数据。
func damageFlags(out damageOutcome, magic bool) map[string]any {
	var flags map[string]any
	if out.crit || out.combo || magic {
		flags = map[string]any{}
	}
	if out.crit {
		flags["crit"] = true
	}
	if out.combo {
		flags["combo"] = true
	}
	if magic {
		flags["magic"] = true
	}
	return flags
}

// decayDmg 长局衰减伤害修正（公式表 §6"累计"口径，全部系数来自 formula.json decay 段）。
func (b *Battle) decayDmg(turn int) float64 {
	v := 1.0
	for _, s := range b.F.Decay {
		if turn < s.From {
			continue
		}
		if s.Dmg > 0 {
			v = s.Dmg
		}
		if s.DmgStep != 0 {
			hi := turn
			if s.To > 0 && hi > s.To {
				hi = s.To
			}
			v += s.DmgStep * float64(hi-s.From+1)
		}
	}
	return v
}

// decayHeal 长局衰减治疗修正（下限取自 decay 段的 heal_floor）。
func (b *Battle) decayHeal(turn int) float64 {
	v := 1.0
	floor := 0.0
	for _, s := range b.F.Decay {
		if turn < s.From {
			continue
		}
		if s.Heal > 0 {
			v = s.Heal
		}
		if s.HealStep != 0 {
			hi := turn
			if s.To > 0 && hi > s.To {
				hi = s.To
			}
			v += s.HealStep * float64(hi-s.From+1)
		}
		if s.HealFloor > 0 {
			floor = s.HealFloor
		}
	}
	if floor > 0 && v < floor {
		v = floor
	}
	return v
}

// damageRoll 单段伤害（公式表 §1 精确实现）：
// 攻击²÷(攻击+有效防御) × 倍率 × 乘区(≤cap) × K × 压制 × (1−减伤) × 浮动 × 长局衰减；下限 1。
// 随机消耗顺序：命中 → 暴击 → 连击 → 浮动。
func (b *Battle) damageRoll(atk, tgt *Unit, mult float64, levelDiff int, _ []Buff) int {
	return b.damageRollTyped(atk, tgt, mult, levelDiff, "physical", false).value
}

// damageRollTyped 返回伤害数值与暴击标记；defendHalve 让防御指令对普攻/秘籍统一生效。
func (b *Battle) damageRollTyped(atk, tgt *Unit, mult float64, levelDiff int, atkType string, defendHalve bool) damageOutcome {
	a := effectiveStats(atk)
	d := effectiveStats(tgt)
	atkVal := float64(a.PAtk)
	defVal := float64(d.PDef)
	if atkType == "magic" {
		atkVal = float64(a.MAtk)
		defVal = float64(d.MDef)
	}
	// 命中判定（物理走命中钳制；法术走目标法术闪避百分比；DOT 不判定——M2a 无 DOT）。
	if atkType == "physical" {
		if b.f64() >= hitChance(a.PHit, d.PDodge, b.F) {
			return damageOutcome{} // miss
		}
	} else if b.f64() < float64(d.MDodge)/10000.0 {
		return damageOutcome{}
	}
	// M2a-2 感电易伤：每层受雷法伤害 +4%（配置驱动，公式表 §3 命中后结算）。
	if atkType == "magic" && tgt.ShockStacks > 0 && b.F.Effects.Shock.MaxStacks > 0 {
		mult *= 1 + float64(tgt.ShockStacks*b.F.Effects.Shock.DmgTakenPctPerStack)/10000.0
	}
	// 有效防御（§1：防御×(1+修正)×(1−无视)×(1−破甲×3%)；M2a 无破甲/无视来源）。
	effDef := defVal
	raw := atkVal * atkVal / (atkVal + effDef)
	// 压制（§2.5）。
	sup := 1.0 + float64(levelDiff)*b.F.Damage.LevelDiffPerLevel
	if sup > 1+b.F.Damage.LevelDiffClamp {
		sup = 1 + b.F.Damage.LevelDiffClamp
	}
	if sup < 1-b.F.Damage.LevelDiffClamp {
		sup = 1 - b.F.Damage.LevelDiffClamp
	}
	// 乘区合并（暴击×连击），上限 2.25（§6 红线 1 执行值）。
	merged := 1.0
	crit := false
	combo := false
	if atkType == "physical" || atkType == "magic" {
		critRate := float64(a.PCrit) / 10000.0
		if atkType == "magic" {
			critRate = float64(a.MCrit) / 10000.0
		}
		if b.f64() < critRate {
			crit = true
			critB := b.F.Crit.Damage - 1
			if atk.Faction == "断岳剑宗" {
				critB += b.F.Crit.DuanyueBonus
			}
			merged *= 1 + critB
		}
		if b.f64() < float64(a.PCombo)/10000.0 {
			combo = true
			merged *= 1 + b.F.Combo.ExtraMult
		}
		if merged > b.F.MultiplierCap {
			merged = b.F.MultiplierCap
		}
	}
	floatF := b.F.Damage.FloatLow + (b.F.Damage.FloatHigh-b.F.Damage.FloatLow)*b.f64()
	v := raw * mult * merged * b.F.Damage.K * sup * floatF * b.decayDmg(b.Turn)
	if defendHalve {
		// 防御指令减伤 ×0.5（助手定案首版，登记待公式表补充；普攻与秘籍同口径）。
		v *= 0.5
	}
	out := int(math.Floor(v))
	if out < b.F.Damage.MinDamage {
		out = b.F.Damage.MinDamage
	}
	return damageOutcome{value: out, crit: crit, combo: combo}
}

// controlRoll 控制结算（公式表 §4）：(基础 + 封印命中)×(1−抗性)×压制×0.7^连控；
// 硬控结束 1 回合内免疫同类（封技能不属硬控免疫范围）。
//
// 封印命中（SealHit，万分制）此前只存在于属性表、未进入公式，导致套装/词条给的封印命中空转；
// 这里按文档口径把施法者的 SealHit 加到基础成功率上（万分制 → 概率），再乘抗性与压制。
func (b *Battle) controlRoll(ctrlType string, baseRate float64, caster, tgt *Unit, levelDiff int) bool {
	if isHard(ctrlType, b.F.Control.HardTypes) && tgt.LastHardEndTurn > 0 && b.Turn-tgt.LastHardEndTurn <= b.F.Control.ImmuneTurns {
		return false
	}
	sup := 1.0 + float64(levelDiff)*b.F.Damage.LevelDiffPerLevel
	sealHit := 0
	if caster != nil {
		sealHit = effectiveStats(caster).SealHit
	}
	rate := (baseRate/100.0 + float64(sealHit)/10000.0) *
		(1 - float64(effectiveStats(tgt).SealRes)/10000.0) * sup *
		math.Pow(b.F.Control.ComboDecay, float64(tgt.ControlCount))
	if b.f64() < rate {
		tgt.ControlCount++
		return true
	}
	tgt.ControlCount = 0
	return false
}

func isHard(t string, hard []string) bool {
	for _, h := range hard {
		if h == t {
			return true
		}
	}
	return false
}

// ---- M2a-2：感电 / DOT / 反伤 / 多目标 ----

func actorID(u *Unit) int64 {
	if u == nil {
		return 0
	}
	return u.ID
}

// reflectRate 反伤系数合计（万分制），来源四类并列相加（数值规范 §9.7）：
//  1. 心法金刚印：受击叠层 × 每层系数（门派身份，1 级裸装即有）；
//  2. 绝学不动明王：Buff `reflect_add`（3 回合）；
//  3. 品质共鸣微调（禅院 +1%~+3%）：已在 Stats.Reflect 里；
//  4. 法宝昆仑镜概率反弹：待法宝系统落地，届时并入 Stats.Reflect。
//
// 单位自身 ReflectPct（怪物/心法层的等效给定值）同样计入。
func (b *Battle) reflectRate(u *Unit) int {
	if u == nil {
		return 0
	}
	st := effectiveStats(u)
	rate := u.ReflectPct + st.Reflect + u.ReflectStacks*st.ReflectPerStack
	// 第 ⑤ 步上限仍以公式配置为准（反伤 ≤50%）。
	if cap := b.F.CoeffCaps.ReflectPct; cap > 0 && rate > cap {
		rate = cap
	}
	return rate
}

// applyDamage 统一扣血、战报、死亡与反伤。
// 反伤口径（数值规范 §2.4 + §9.7）：
//   - 在 defender **实际掉血量确定后**触发，反伤 = 实际伤害 × 反伤系数；
//   - 独立战报事件（reflect）作用于攻击者，不触发暴击/连击二次判定、不触发吸血、不再递归反伤；
//   - 可致死；defender 本击阵亡也照常结算（同归于尽合法）。
func (b *Battle) applyDamage(evs []Event, actor, target *Unit, dmg int, flags map[string]any) []Event {
	if dmg <= 0 || target == nil || !target.Alive {
		return evs
	}
	target.HP -= dmg
	// 金刚印：受击即叠 1 层（上限取心法配置），本击的反伤就按叠层后的系数算——
	// 这是「禅院 1 级裸装挨打就有反伤」的落点，装备与共鸣都不参与。
	if st := effectiveStats(target); st.ReflectPerStack > 0 && target.ReflectStacks < st.ReflectMaxStacks {
		target.ReflectStacks++
	}
	evs = b.emit(evs, Event{Type: "damage", Actor: actorID(actor), Target: target.ID, Value: dmg, Flags: flags})
	if target.HP <= 0 {
		target.HP = 0
		target.Alive = false
		evs = b.emit(evs, Event{Type: "death", Target: target.ID})
	}
	// 反伤放在死亡事件之后结算：先确认 defender 掉了多少血，再回弹给攻击者。
	if rate := b.reflectRate(target); rate > 0 && actor != nil && actor.Alive {
		rd := int(math.Floor(float64(dmg) * float64(rate) / 10000.0))
		if rd < b.F.Effects.Reflect.MinDamage {
			rd = b.F.Effects.Reflect.MinDamage
		}
		actor.HP -= rd
		evs = b.emit(evs, Event{Type: "reflect", Actor: target.ID, Target: actor.ID, Value: rd,
			Flags: map[string]any{
				"rate":   rate,
				"stacks": target.ReflectStacks,
				"max":    effectiveStats(target).ReflectMaxStacks,
			}})
		if actor.HP <= 0 {
			actor.HP = 0
			actor.Alive = false
			evs = b.emit(evs, Event{Type: "death", Target: actor.ID})
		}
	}
	return evs
}

// maxHP 取当前生命上限（Buff 不改上限，取 BaseStats.HP）。
func maxHP(u *Unit) int { return u.BaseStats.HP }

// applyHeal 治疗结算钩子（数值规范 §9.7）：
// 最终治疗 = 基础治疗 × (1 + **施术者**治疗加成)，随后再乘长局衰减乘子（顺序不得颠倒）。
// 溢出治疗截断至生命上限。治疗秘籍（玄水宫）尚未入配置包，本钩子先就位，
// 秘籍表 V1.1 的玄水治疗技能落地时直接调用即可，系数无需再改。
func (b *Battle) applyHeal(evs []Event, healer, target *Unit, base int, flags map[string]any) []Event {
	if base <= 0 || target == nil || !target.Alive {
		return evs
	}
	bonus := 0
	if healer != nil {
		bonus = effectiveStats(healer).HealBonus
	}
	amount := int(math.Floor(float64(base) * (1 + float64(bonus)/10000.0)))
	// 衰减在加成之后：先放大治疗，再乘回合衰减（文档 §9.7 明确顺序）。
	amount = int(math.Floor(float64(amount) * b.decayHeal(b.Turn)))
	if amount < 1 {
		amount = 1
	}
	before := target.HP
	if hp := before + amount; hp > maxHP(target) {
		target.HP = maxHP(target)
	} else {
		target.HP = hp
	}
	healed := target.HP - before
	f := flags
	if f == nil {
		f = map[string]any{}
	}
	f["bonus"] = bonus
	f["overheal"] = amount - healed
	return b.emit(evs, Event{Type: "heal", Actor: actorID(healer), Target: target.ID, Value: healed, Flags: f})
}

// applyShock 法术命中叠感电并刷新 DOT 快照（DOT 按施加者法攻在施加时定格，保证可重演）。
func (b *Battle) applyShock(evs []Event, actor, target *Unit) []Event {
	sh := b.F.Effects.Shock
	if sh.MaxStacks <= 0 || actor == nil || target == nil {
		return evs
	}
	if target.ShockStacks < sh.MaxStacks {
		target.ShockStacks++
	}
	target.ShockDOTTurns = sh.DOTTurns
	target.ShockDOTPower = int(math.Floor(
		float64(effectiveStats(actor).MAtk) * float64(target.ShockStacks*sh.DOTPctOfMAtkPerStack) / 10000.0))
	return b.emit(evs, Event{Type: "shock", Actor: actor.ID, Target: target.ID,
		Flags: map[string]any{"stacks": target.ShockStacks, "max": sh.MaxStacks}})
}

// checkOver 全灭终局检查（供单位行动后与回合末 DOT 后共用）。
func (b *Battle) checkOver(evs []Event) ([]Event, bool) {
	if len(b.alive(0)) == 0 || len(b.alive(1)) == 0 {
		b.Over = true
		winner := 0
		if len(b.alive(0)) == 0 {
			winner = 1
		}
		return b.emit(evs, Event{Type: "end", Flags: map[string]any{"winner": winner}}), true
	}
	return evs, false
}

// targetsFor 按指令目标与秘籍目标数取存活敌方；指定目标排首位，其余按站位序补足。
func (b *Battle) targetsFor(u *Unit, cmd Cmd, n int) []*Unit {
	foes := b.alive(1 - u.Side)
	if cmd.TargetID != 0 {
		for i, f := range foes {
			if f.ID == cmd.TargetID {
				var reordered []*Unit
				reordered = append(reordered, f)
				for j, other := range foes {
					if j != i {
						reordered = append(reordered, other)
					}
				}
				foes = reordered
				break
			}
		}
	}
	if n < 1 {
		n = 1
	}
	if len(foes) > n {
		foes = foes[:n]
	}
	return foes
}

// executeSkill 执行秘籍：真气不足回退普攻；支持多目标（targets 由 targetsFor 给出）。
func (b *Battle) executeSkill(actor *Unit, targets []*Unit, skill config.SkillDef, levelDiff int) []Event {
	var evs []Event
	if len(targets) == 0 {
		return evs
	}
	if actor.MP < skill.MPCost {
		return b.executeAttack(actor, targets[0], true)
	}
	actor.MP -= skill.MPCost
	evs = b.emit(evs, Event{Type: "act", Actor: actor.ID, Target: targets[0].ID,
		Flags: map[string]any{"skill": skill.Name, "targets": len(targets)}})
	// 已实现特效走真实结算；其余按配置登记延后（validateBattle 保证未实现项不会静默进包）。
	if skill.Special != "" && skill.Special != "shock_bonus_10" {
		evs = b.emit(evs, Event{Type: "flag", Actor: actor.ID,
			Flags: map[string]any{"special_deferred": skill.Special}})
	}
	// 蓄力伤害钩子（数值规范 §9.7）：消耗星辉的攻击秘籍，该次伤害 × (1 + 蓄力伤害系数)。
	// 系数取施法者角色级属性（品质共鸣微调/心法/法宝在管线内已累加）。
	chargeMult := 1.0
	if skill.ConsumesStar && skill.AttackType != "" {
		chargeMult += float64(effectiveStats(actor).ChargeDmg) / 10000.0
	}
	for _, tgt := range targets {
		if !tgt.Alive {
			continue
		}
		mult := chargeMult
		if skill.Special == "shock_bonus_10" && tgt.ShockStacks > 0 {
			mult += float64(b.F.Effects.Shock.BonusVsShockedPct) / 10000.0
		}
		for _, seg := range skill.Segments {
			out := b.damageRollTyped(actor, tgt, seg.Mult/100.0*mult, levelDiff, skill.AttackType, tgt.Defending)
			if out.value == 0 {
				evs = b.emit(evs, Event{Type: "miss", Actor: actor.ID, Target: tgt.ID})
				continue
			}
			evs = b.applyDamage(evs, actor, tgt, out.value, damageFlags(out, skill.AttackType == "magic"))
			// 法术命中叠感电（公式表 §3：DOT 自身不走命中/闪避）。
			if tgt.Alive && skill.AttackType == "magic" {
				evs = b.applyShock(evs, actor, tgt)
			}
			if !tgt.Alive {
				break
			}
		}
	}
	if skill.Control != nil && targets[0].Alive {
		target := targets[0]
		if b.controlRoll(skill.Control.Type, skill.Control.Rate, actor, target, levelDiff) {
			duration := skill.Control.Duration
			if isHard(skill.Control.Type, b.F.Control.HardTypes) {
				target.HardTurns = duration // 硬控：失去行动
			} else {
				target.SealedTurns = duration // 封技能：仍可普攻/防御
			}
			evs = b.emit(evs, Event{Type: "control", Actor: actor.ID, Target: target.ID,
				Flags: map[string]any{"type": skill.Control.Type, "duration": duration}})
		} else {
			evs = b.emit(evs, Event{Type: "control_fail", Actor: actor.ID, Target: target.ID})
		}
	}
	if skill.BuffSelf != nil {
		buffTargets := []*Unit{actor}
		if skill.BuffGroup {
			buffTargets = b.alive(actor.Side)
		}
		for _, t := range buffTargets {
			t.Buffs = append(t.Buffs, Buff{Stat: skill.BuffSelf.Stat, Value: skill.BuffSelf.Value, Turns: skill.BuffSelf.Turns})
			evs = b.emit(evs, Event{Type: "buff", Actor: actor.ID, Target: t.ID,
				Flags: map[string]any{"stat": skill.BuffSelf.Stat, "value": skill.BuffSelf.Value, "turns": skill.BuffSelf.Turns}})
		}
	}
	return evs
}

// executeAttack 普攻（倍率 100%，单目标）。
func (b *Battle) executeAttack(actor, target *Unit, fallback bool) []Event {
	var evs []Event
	if fallback {
		evs = b.emit(evs, Event{Type: "fallback", Actor: actor.ID,
			Flags: map[string]any{"回退": true, "reason": "mp_short"}})
	}
	if actor.Hosted {
		evs = b.emit(evs, Event{Type: "fallback", Actor: actor.ID,
			Flags: map[string]any{"回退": true, "reason": "hosted"}})
		actor.Hosted = false
	}
	out := b.damageRollTyped(actor, target, 1.0, actor.Level-target.Level, "physical", target.Defending)
	if out.value == 0 {
		return b.emit(evs, Event{Type: "miss", Actor: actor.ID, Target: target.ID})
	}
	return b.applyDamage(evs, actor, target, out.value, damageFlags(out, false))
}

// SubmitCmd 登记指令。
func (b *Battle) SubmitCmd(unitID int64, cmd Cmd) {
	if b.cmds == nil {
		b.cmds = map[int64]Cmd{}
	}
	b.cmds[unitID] = cmd
}

// cmdPriority 指令分层（§2.1 动作优先级：防御>物品>法宝>秘籍>普攻>逃跑）。
func cmdPriority(c Cmd) int {
	switch c.Type {
	case "defend":
		return 0
	case "item":
		return 1
	case "fabao":
		return 2
	case "skill":
		return 3
	case "attack":
		return 4
	case "flee":
		return 5
	}
	return 9
}

// ResolveTurn 结算一回合：分层→速度序（同速 ±3% 浮动）→逐单位执行→回合末。
func (b *Battle) ResolveTurn() []Event {
	b.Turn++
	var evs []Event
	// §1.3 属性结算管线：回合结算前重算生效快照，让 Buff 对命中等抗性字段生效。
	for _, u := range b.Units {
		u.Stats = effectiveStats(u)
	}
	// 行动序：优先级分层，层内有效速度降序（同速 ±3% 浮动判定，公式表 §7）。
	type order struct {
		u   *Unit
		pri int
		spd float64
	}
	var ord []order
	for _, u := range b.Units {
		if !u.Alive {
			continue
		}
		cmd := b.cmds[u.ID]
		sp := float64(effectiveStats(u).Speed) * (1 + (b.f64()*2-1)*b.F.SpeedJitter)
		ord = append(ord, order{u: u, pri: cmdPriority(cmd), spd: sp})
	}
	for i := 0; i < len(ord); i++ {
		for j := i + 1; j < len(ord); j++ {
			if ord[j].pri < ord[i].pri || (ord[j].pri == ord[i].pri && ord[j].spd > ord[i].spd) {
				ord[i], ord[j] = ord[j], ord[i]
			}
		}
	}
	// 行动顺序条（UI 规范 §9：顶部顺序头像列 + 当前行动者标记）。
	ids := make([]int64, 0, len(ord))
	names := make([]string, 0, len(ord))
	slots := make([]int, 0, len(ord))
	for _, o := range ord {
		ids = append(ids, o.u.ID)
		names = append(names, o.u.Name)
		slots = append(slots, o.u.Slot)
	}
	evs = b.emit(evs, Event{Type: "order", Flags: map[string]any{
		"ids": ids, "names": names, "slots": slots, "turn": b.Turn,
	}})

	var over bool
	for _, o := range ord {
		u := o.u
		if !u.Alive {
			continue
		}
		cmd := b.cmds[u.ID]
		// 硬控：失去行动。
		if u.HardTurns > 0 {
			u.HardTurns--
			if u.HardTurns == 0 {
				u.LastHardEndTurn = b.Turn // 免疫窗从硬控真正结束的回合起算
			}
			// 被控单位不得残留上一回合的防御/托管标记。
			u.Defending = false
			u.Hosted = false
			evs = b.emit(evs, Event{Type: "stunned", Actor: u.ID})
			continue
		}
		u.Defending = false
		targets := b.targetsFor(u, cmd, 1)
		if len(targets) == 0 {
			break
		}
		switch cmd.Type {
		case "defend":
			u.Defending = true
			evs = b.emit(evs, Event{Type: "defend", Actor: u.ID})
		case "flee":
			// 逃跑：速度档基线 50%（助手定案首版；Boss/被控为 0 的规则留后续）。
			if b.f64() < 0.5 {
				u.Alive = false
				evs = b.emit(evs, Event{Type: "flee", Actor: u.ID})
			} else {
				evs = b.emit(evs, Event{Type: "flee_fail", Actor: u.ID})
			}
		case "skill":
			var skill *config.SkillDef
			for i := range u.Skills {
				if u.Skills[i].ID == cmd.SkillID && u.Level >= u.Skills[i].LevelReq {
					skill = &u.Skills[i]
					break
				}
			}
			if skill == nil || u.SealedTurns > 0 {
				evs = append(evs, b.executeAttack(u, targets[0], u.SealedTurns > 0)...)
			} else {
				evs = append(evs, b.executeSkill(u, b.targetsFor(u, cmd, skill.Targets), *skill, u.Level-targets[0].Level)...)
			}
		default: // attack
			evs = append(evs, b.executeAttack(u, targets[0], false)...)
		}
		evs, over = b.checkOver(evs)
		if over {
			return evs
		}
		if cmd.Type == "flee" && !u.Alive {
			b.Over = true
			evs = b.emit(evs, Event{Type: "end", Flags: map[string]any{"winner": 1 - u.Side, "fled": u.ID}})
			return evs
		}
	}
	// 回合末：Buff 递减、封技能回合递减（与指令类型无关）、过期移除。
	for _, u := range b.Units {
		var keep []Buff
		for _, bf := range u.Buffs {
			bf.Turns--
			if bf.Turns > 0 {
				keep = append(keep, bf)
			}
		}
		u.Buffs = keep
		if u.SealedTurns > 0 {
			u.SealedTurns--
		}
	}
	// 感电 DOT 结算（公式表 §3：不走命中/闪避；长局衰减同口径；下限取配置）。
	for _, u := range b.Units {
		if !u.Alive || u.ShockDOTTurns <= 0 || u.ShockDOTPower <= 0 {
			continue
		}
		dmg := int(math.Floor(float64(u.ShockDOTPower) * b.decayDmg(b.Turn)))
		if dmg < b.F.Effects.Dot.MinDamage {
			dmg = b.F.Effects.Dot.MinDamage
		}
		u.HP -= dmg
		evs = b.emit(evs, Event{Type: "dot", Target: u.ID, Value: dmg,
			Flags: map[string]any{"stacks": u.ShockStacks}})
		if u.HP <= 0 {
			u.HP = 0
			u.Alive = false
			evs = b.emit(evs, Event{Type: "death", Target: u.ID})
		}
		u.ShockDOTTurns--
		if u.ShockDOTTurns == 0 {
			u.ShockStacks, u.ShockDOTPower = 0, 0
		}
	}
	evs, over = b.checkOver(evs)
	if over {
		return evs
	}
	evs = b.emit(evs, Event{Type: "turn_end", Flags: map[string]any{"turn": b.Turn, "next_turn": b.Turn + 1}})
	if b.Turn >= b.F.MaxTurns && !b.Over {
		b.Over = true
		evs = b.emit(evs, Event{Type: "end", Flags: map[string]any{"winner": -1, "reason": "max_turns"}})
	}
	return evs
}
