// Package battle 为战斗域（开发技术规范 §8）：属性管线、公式引擎、战斗房间。
// 数值全部来自配置包（formula/classes/skills/monsters），引擎零硬编码数值。
package battle

import (
	"context"
	"fmt"
	"math"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/store"
)

// UnitStats 为战斗二级属性（数值规范 §1.2 完整清单的 M2a 子集）。
type UnitStats struct {
	HP, MP           int
	PAtk, MAtk       int
	PDef, MDef       int
	Speed            int
	PHit             int // 数值字段（进命中公式，不直接等于百分比）
	PDodge, MDodge   int // MDodge 百分比（万分制）
	PCrit, MCrit     int // 万分制
	PCombo           int // 万分制
	SealHit, SealRes int // 万分制
	Flee             int // 万分制

	// 角色级系数（数值规范 §9.7，万分制）：由心法被动、品质共鸣微调、法宝、Buff 在第 ④ 步
	// 按百分点累加，第 ⑤ 步按 Formula.CoeffCaps 裁剪。此前引擎无这三项，套装三项微调只能登记不生效。
	Reflect   int // 反伤系数：受击按实际伤害回弹给攻击者
	HealBonus int // 治疗加成：最终治疗 = 基础 × (1 + 本系数)
	ChargeDmg int // 蓄力伤害：消耗星辉的攻击秘籍伤害 × (1 + 本系数)
}

// DerivedStats 按门派定位与等级派生二级属性（M2a 首版）：
// 锚点线性插值（数值规范 §5.2 逐字）→ 低级幂律外延（ClassBook 参数，助手定案）
// → 肉盾派生系数（§5.3）。登记待《战斗属性计算表》立项回归校准。
func DerivedStats(level int, faction string, cfg config.BattleData) (UnitStats, error) {
	return DerivedStatsWithPoints(level, faction, cfg, nil, nil)
}

// DerivedStatsWithPoints 在基线派生之上叠加一级属性点的扁平派生收益（数值规范 §1.3 ③）。
// gains 为配置 point_gains（str/spi/agi/con → 二级属性），points 为角色已分配点。
func DerivedStatsWithPoints(level int, faction string, cfg config.BattleData,
	gains map[string]map[string]int, points map[string]int) (UnitStats, error) {
	_ = cfg // 占位保持签名稳定；基线取自 cfg.Formula
	class := cfg.ClassByFaction(faction)
	if class == nil {
		return UnitStats{}, fmt.Errorf("未知门派定位: %q", faction)
	}
	base, err := anchorInterpolate(level, class, cfg.ClassBook)
	if err != nil {
		return UnitStats{}, err
	}
	// 肉盾派生系数（§5.3：禅院=物理 HP×1.30/攻×0.75/防×1.25/速×0.85 等）。
	if class.DeriveFrom != "" {
		applyCoeff(&base, class.Coeff)
	}
	st := UnitStats{
		HP: base.HP, MP: base.MP,
		PAtk: base.PAtk, MAtk: base.MAtk,
		PDef: base.PDef, MDef: base.MDef,
		Speed: base.Speed,
		// 命中/闪避/暴击等首版基线（公式配置表 §2/§3 基线值，万分制）。
		PHit:    10000, // 命中公式内再按基线 95% 换算钳制
		PDodge:  0,
		MDodge:  int(cfg.Formula.Hit.MagicDodgeBase * 10000), // 10%
		PCrit:   int(cfg.Formula.Crit.BaseRate * 10000),      // 5%
		MCrit:   int(cfg.Formula.Crit.BaseRate * 10000),
		PCombo:  0,
		SealHit: 0,
		SealRes: int(cfg.Formula.Control.SealResBase * 10000), // 10%
		Flee:    0,
	}
	applyPointGains(&st, gains, points)
	return st, nil
}

// applyPointGains 叠加一级属性点的扁平派生收益（§1.3 ③；dodge 万分制）。
func applyPointGains(st *UnitStats, gains map[string]map[string]int, points map[string]int) {
	if len(gains) == 0 || len(points) == 0 {
		return
	}
	for primary, n := range points {
		if n <= 0 {
			continue
		}
		for sec, per := range gains[primary] {
			v := per * n
			switch sec {
			case "hp":
				st.HP += v
			case "mp":
				st.MP += v
			case "patk":
				st.PAtk += v
			case "matk":
				st.MAtk += v
			case "pdef":
				st.PDef += v
			case "mdef":
				st.MDef += v
			case "speed":
				st.Speed += v
			case "dodge":
				st.PDodge += v
			}
		}
	}
}

func anchorInterpolate(level int, class *config.BattleClass, book config.ClassBook) (config.AnchorStat, error) {
	if class.DeriveFrom != "" {
		// 肉盾无自有锚点：取派生来源定位的插值结果（系数随后应用）。
		src := findClass(book, class.DeriveFrom)
		if src == nil {
			return config.AnchorStat{}, fmt.Errorf("定位 %s 派生来源 %q 不存在", class.Class, class.DeriveFrom)
		}
		return anchorInterpolate(level, src, book)
	}
	keys := sortedLevels(class.Anchors)
	if len(keys) == 0 {
		return config.AnchorStat{}, fmt.Errorf("定位 %s 无锚点", class.Class)
	}
	// 恰好命中锚点直接返回：单锚点配置（只有一个等级档）无法插值，
	// 旧实现会一路掉到函数末尾报错，这里补上直取分支。
	if a, ok := class.Anchors[fmt.Sprint(level)]; ok {
		return a, nil
	}
	// 低级外延：最低锚点 × (level/base)^exp。
	if level < keys[0] {
		base := float64(book.LowCurveBaseLv)
		if base <= 0 {
			base = 20
		}
		exp := book.LowCurveExp
		if exp <= 0 {
			exp = 1.5
		}
		a := class.Anchors[fmt.Sprint(keys[0])]
		f := math.Pow(float64(level)/base, exp)
		return scaleAnchor(a, f), nil
	}
	// 区间内线性插值；越过最高锚点按最近段斜率延伸。
	for i := 0; i < len(keys)-1; i++ {
		lo, hi := keys[i], keys[i+1]
		if level >= lo && level <= hi {
			t := float64(level-lo) / float64(hi-lo)
			return lerpAnchor(class.Anchors[fmt.Sprint(lo)], class.Anchors[fmt.Sprint(hi)], t), nil
		}
	}
	last := keys[len(keys)-1]
	if level > last {
		prev := keys[len(keys)-2]
		t := float64(level-last) / float64(last-prev)
		return lerpAnchor(class.Anchors[fmt.Sprint(last)], extrapolateAnchor(class.Anchors[fmt.Sprint(prev)], class.Anchors[fmt.Sprint(last)], t), 1), nil
	}
	return config.AnchorStat{}, fmt.Errorf("等级 %d 无法插值", level)
}

func findClass(book config.ClassBook, name string) *config.BattleClass {
	for i := range book.Classes {
		if book.Classes[i].Class == name {
			return &book.Classes[i]
		}
	}
	return nil
}

func sortedLevels(m map[string]config.AnchorStat) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		lv := 0
		for _, ch := range k {
			if ch >= '0' && ch <= '9' {
				lv = lv*10 + int(ch-'0')
			}
		}
		out = append(out, lv)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func lerpAnchor(lo, hi config.AnchorStat, t float64) config.AnchorStat {
	lerp := func(a, b int) int { return int(math.Round(float64(a) + (float64(b)-float64(a))*t)) }
	return config.AnchorStat{
		HP: lerp(lo.HP, hi.HP), MP: lerp(lo.MP, hi.MP),
		PAtk: lerp(lo.PAtk, hi.PAtk), MAtk: lerp(lo.MAtk, hi.MAtk),
		PDef: lerp(lo.PDef, hi.PDef), MDef: lerp(lo.MDef, hi.MDef),
		Speed: lerp(lo.Speed, hi.Speed),
	}
}

func extrapolateAnchor(lo, hi config.AnchorStat, t float64) config.AnchorStat {
	// 沿 lo→hi 方向继续延伸 t 个同幅步长。
	d := config.AnchorStat{
		HP: hi.HP - lo.HP, MP: hi.MP - lo.MP,
		PAtk: hi.PAtk - lo.PAtk, MAtk: hi.MAtk - lo.MAtk,
		PDef: hi.PDef - lo.PDef, MDef: hi.MDef - lo.MDef,
		Speed: hi.Speed - lo.Speed,
	}
	out := hi
	out.HP += int(float64(d.HP) * t)
	out.MP += int(float64(d.MP) * t)
	out.PAtk += int(float64(d.PAtk) * t)
	out.MAtk += int(float64(d.MAtk) * t)
	out.PDef += int(float64(d.PDef) * t)
	out.MDef += int(float64(d.MDef) * t)
	out.Speed += int(float64(d.Speed) * t)
	return out
}

func scaleAnchor(a config.AnchorStat, f float64) config.AnchorStat {
	round := func(v int) int { return int(math.Max(1, math.Round(float64(v)*f))) }
	return config.AnchorStat{
		HP: round(a.HP), MP: round(a.MP),
		PAtk: round(a.PAtk), MAtk: round(a.MAtk),
		PDef: round(a.PDef), MDef: round(a.MDef),
		Speed: round(a.Speed),
	}
}

func applyCoeff(a *config.AnchorStat, coeff map[string]float64) {
	mul := func(cur int, key string) int {
		if c, ok := coeff[key]; ok {
			return int(math.Round(float64(cur) * c))
		}
		return cur
	}
	a.HP = mul(a.HP, "hp")
	a.PAtk = mul(a.PAtk, "patk")
	a.MAtk = mul(a.MAtk, "matk")
	a.PDef = mul(a.PDef, "pdef")
	a.MDef = mul(a.MDef, "mdef")
	a.Speed = mul(a.Speed, "speed")
}

// FinalStatsResult 为角色最终二级属性与品质共鸣明细。
type FinalStatsResult struct {
	Stats UnitStats
	Suit  config.SuitResult
}

// LoadFinalStats 计算角色最终二级属性（M2c 属性管线的唯一入口）：
// 门派×等级基线 → 一级属性点收益 → 装备属性累加 → 品质共鸣百分比（V1.29 §9.2/§9.6 口径）。
//
// 此前 4/1 战斗单位只带「基线+点数」，装备与共鸣完全没进战斗——本函数是那处缺陷的修复点。
func LoadFinalStats(ctx context.Context, char *store.Character, equips *store.EquipsRepo, cfg config.Data) (FinalStatsResult, error) {
	var out FinalStatsResult
	base, err := DerivedStatsWithPoints(char.Level, char.Faction, cfg.Battle,
		cfg.Levels.PointGains, char.PointsMap())
	if err != nil {
		return out, err
	}
	equipped, err := equips.List(ctx, char.ID)
	if err != nil {
		return out, err
	}
	sum := map[string]int{}
	quals := make([]string, 0, len(equipped))
	for _, itemID := range equipped {
		if def := cfg.Items.ByID(itemID); def != nil {
			for k, v := range def.Stats {
				sum[k] += v
			}
		}
		quals = append(quals, cfg.Items.ItemQuality(itemID))
	}
	out.Suit = cfg.Suit.SuitResonance(quals, char.Faction, cfg.CategoryOf(char.Faction))
	out.Stats = applyEquipAndSuit(base, sum, out.Suit.Percent, cfg.Battle.Formula.CoeffCaps)
	return out, nil
}

// applyEquipAndSuit 先累加装备的数值型属性，再按百分比应用共鸣加成。
// 数值型按百分比乘算（不降低原值），概率型（万分制）按百分点相加（数值规范 §1.2）。
// §1.3 管线第 ④ 步（百分比加成）结束后，第 ⑤ 步对角色级系数按 caps 裁剪（§9.7）。
func applyEquipAndSuit(st UnitStats, equip map[string]int, pct map[string]float64, caps config.CoeffCaps) UnitStats {
	st.HP += equip["hp"]
	st.MP += equip["mp"]
	st.PAtk += equip["patk"]
	st.MAtk += equip["matk"]
	st.PDef += equip["pdef"]
	st.MDef += equip["mdef"]
	st.Speed += equip["speed"]
	mul := func(v int, key string) int {
		p := pct[key]
		if p == 0 {
			return v
		}
		n := int(math.Round(float64(v) * (1 + p/100.0)))
		if n < v {
			return v
		}
		return n
	}
	st.HP = mul(st.HP, "hp")
	st.MP = mul(st.MP, "mp")
	st.PAtk = mul(st.PAtk, "patk")
	st.MAtk = mul(st.MAtk, "matk")
	st.PDef = mul(st.PDef, "pdef")
	st.MDef = mul(st.MDef, "mdef")
	st.Speed = mul(st.Speed, "speed")
	rate := func(v int, key string) int { return v + int(math.Round(pct[key]*100)) }
	st.PCrit = rate(st.PCrit, "pcrit")
	st.MCrit = rate(st.MCrit, "mcrit")
	st.PCombo = rate(st.PCombo, "pcombo")
	st.SealHit = rate(st.SealHit, "sealhit")
	st.PDodge = rate(st.PDodge, "pdodge")
	// 角色级系数：百分点 → 万分制，再按上限裁剪（§9.7）。
	st.Reflect = rate(st.Reflect, "reflect")
	st.HealBonus = rate(st.HealBonus, "heal_bonus")
	st.ChargeDmg = rate(st.ChargeDmg, "charge_dmg")
	st.Reflect = clampRate(st.Reflect, caps.ReflectPct)
	st.HealBonus = clampRate(st.HealBonus, caps.HealBonusPct)
	return st
}

// clampRate 第 ⑤ 步单项上限裁剪；cap<=0 表示该项不设上限。
func clampRate(v, cap int) int {
	if cap > 0 && v > cap {
		return cap
	}
	return v
}
