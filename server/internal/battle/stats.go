// Package battle 为战斗域（开发技术规范 §8）：属性管线、公式引擎、战斗房间。
// 数值全部来自配置包（formula/classes/skills/monsters），引擎零硬编码数值。
package battle

import (
	"fmt"
	"math"

	"fantasy-jianghu/server/internal/config"
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
}

// DerivedStats 按门派定位与等级派生二级属性（M2a 首版）：
// 锚点线性插值（数值规范 §5.2 逐字）→ 低级幂律外延（ClassBook 参数，助手定案）
// → 肉盾派生系数（§5.3）。登记待《战斗属性计算表》立项回归校准。
func DerivedStats(level int, faction string, cfg config.BattleData) (UnitStats, error) {
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
	return UnitStats{
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
	}, nil
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
