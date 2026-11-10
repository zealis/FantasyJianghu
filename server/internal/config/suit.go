package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// 套装加成配置域（V1.29 转正的《套装加成配置表》V1.0 + 数值规范 §9.2）。
//
// 属性键口径（数值规范 §1.2）：
//   - 数值型（hp/mp/patk/matk/pdef/mdef/speed）：按百分比乘算；
//   - 概率型（pdodge/mcrit/pcrit/pcombo/sealhit）：万分制，按百分点相加。
var suitNumericAttrs = map[string]bool{
	"hp": true, "mp": true, "patk": true, "matk": true, "pdef": true, "mdef": true, "speed": true,
}

var suitRateAttrs = map[string]bool{
	"pdodge": true, "mcrit": true, "pcrit": true, "pcombo": true, "sealhit": true,
}

// 战斗引擎暂无角色级系数的三项：蓄力秘籍伤害、反伤率、治疗加成。
// 配置里照表登记，但必须标 implemented=false，不许假装生效（不伪造数值）。
var suitPendingAttrs = map[string]bool{
	"charge_dmg": true, "reflect": true, "heal_bonus": true,
}

// SuitSystem 为一个系别的主项/副项定义（物理/法术/封印/肉盾）。
type SuitSystem struct {
	Main   []string  `json:"main"`
	Sub    string    `json:"sub"`
	SubPct []float64 `json:"sub_pct"`
}

// FactionTweak 为八派微调（同档同幅、属性分流）。
type FactionTweak struct {
	Attr        string    `json:"attr"`
	Pct         []float64 `json:"pct"`
	Implemented *bool     `json:"implemented"` // 省略视为 true；false 表示引擎尚未接入
}

// ImplementedAttr 报告该微调是否已能真正生效。
func (t FactionTweak) ImplementedAttr() bool {
	if t.Implemented == nil {
		return true
	}
	return *t.Implemented
}

// SuitData 为套装加成域（suit/suit.json）。
type SuitData struct {
	AllAttrs      []string                `json:"all_attrs"`
	QualityOrder  map[string]int          `json:"quality_order"`
	Tier1AllPct   []float64               `json:"tier1_all_pct"`
	Tier2MainPct  []float64               `json:"tier2_main_pct"`
	Systems       map[string]SuitSystem   `json:"systems"`
	FactionTweaks map[string]FactionTweak `json:"faction_tweaks"`
}

// SuitResult 为品质共鸣结算结果（一档/二档 + 叠加后的各项百分比）。
type SuitResult struct {
	Tier      int                `json:"tier"`     // 0 未激活 / 1 一档（3 件）/ 2 二档（5 件）
	Quality   string             `json:"quality"`  // 触发品质（中文）
	Count     int                `json:"count"`    // 该品质件数
	AllPct    float64            `json:"all_pct"`  // 一档全属性 %
	MainAttrs []string           `json:"main_attrs"` // 二档系别主项属性键
	MainPct   float64            `json:"main_pct"` // 二档系别主项 %
	SubAttr   string             `json:"sub_attr"`
	SubPct    float64            `json:"sub_pct"`
	TweakAttr string             `json:"tweak_attr"`
	TweakPct  float64            `json:"tweak_pct"`
	Percent   map[string]float64 `json:"percent"` // 属性键 → 百分比（同键已叠加）
}

// SuitResonance 按已穿戴五槽的品质计算品质共鸣。
//
// 规则（配置表 §1，不改）：
//   - 同品质 ≥3 件 → 一档（全属性 +tier1_all_pct[序数]）；
//   - 五件全同品质 → 二档（**取代**一档：只结算系别主项 + 副项 + 八派微调，V1.30）；
//   - 混品质只按件数最多的那一档结算，不多档叠加。
//
// qualities 允许含空串（空槽不计件）；category 为门派系别（物理/法术/封印/肉盾）。
func (d SuitData) SuitResonance(qualities []string, faction, category string) SuitResult {
	res := SuitResult{Percent: map[string]float64{}}
	counts := map[string]int{}
	for _, q := range qualities {
		if q == "" {
			continue
		}
		counts[q]++
	}
	// 件数最多者胜出；并列时取品质序数更高的（对玩家更有利，且不会出现两个档同时生效）。
	bestQ, bestN := "", 0
	for q, n := range counts {
		if n > bestN || (n == bestN && d.QualityOrder[q] > d.QualityOrder[bestQ]) {
			bestQ, bestN = q, n
		}
	}
	ord := d.QualityOrder[bestQ]
	if bestN < 3 || ord < 1 || ord > len(d.Tier1AllPct) {
		return res
	}
	idx := ord - 1
	res.Quality, res.Count, res.Tier = bestQ, bestN, 1
	res.AllPct = d.Tier1AllPct[idx]
	for _, a := range d.AllAttrs {
		if suitNumericAttrs[a] {
			res.Percent[a] += res.AllPct
		}
	}
	if bestN < 5 {
		return res
	}
	// 二档**取代**一档（V1.30 定案）：5 件全同品质只结算二档（系别主项 + 副项 + 门派微调），
	// 不再叠加一档的全属性。叠加口径会让长局镜像恶化（数值模拟 report_v129），故不采。
	res.Tier = 2
	res.AllPct = 0
	res.Percent = map[string]float64{}
	if idx < len(d.Tier2MainPct) {
		res.MainPct = d.Tier2MainPct[idx]
	}
	if sys, ok := d.Systems[category]; ok {
		res.MainAttrs = sys.Main
		for _, a := range sys.Main {
			res.Percent[a] += res.MainPct
		}
		if idx < len(sys.SubPct) {
			res.SubAttr = sys.Sub
			res.SubPct = sys.SubPct[idx]
			res.Percent[sys.Sub] += res.SubPct
		}
	}
	if tw, ok := d.FactionTweaks[faction]; ok && idx < len(tw.Pct) && tw.ImplementedAttr() {
		res.TweakAttr = tw.Attr
		res.TweakPct = tw.Pct[idx]
		res.Percent[tw.Attr] += res.TweakPct
	}
	return res
}

// validateSuit 校验套装域（§7.2：非法不打包）。
func validateSuit(d SuitData, factions []FactionInfo) error {
	if len(d.AllAttrs) == 0 {
		return fmt.Errorf("suit.all_attrs 不得为空（一档全属性作用范围）")
	}
	for _, a := range d.AllAttrs {
		if !suitNumericAttrs[a] {
			return fmt.Errorf("suit.all_attrs 含非法属性: %q（只允许数值型）", a)
		}
	}
	if len(d.Tier1AllPct) != 5 || len(d.Tier2MainPct) != 5 {
		return fmt.Errorf("suit.tier1_all_pct/tier2_main_pct 长度必须为 5（凡→完美）")
	}
	for i := 0; i < 5; i++ {
		if d.Tier1AllPct[i] < 0 || d.Tier2MainPct[i] < 0 {
			return fmt.Errorf("suit 分档百分比不得为负（档 %d）", i+1)
		}
		if i > 0 && (d.Tier1AllPct[i] < d.Tier1AllPct[i-1] || d.Tier2MainPct[i] < d.Tier2MainPct[i-1]) {
			return fmt.Errorf("suit 分档百分比必须随品质递增（档 %d）", i+1)
		}
	}
	if len(d.QualityOrder) == 0 {
		return fmt.Errorf("suit.quality_order 不得为空")
	}
	// 系别必须覆盖八派实际使用的 category。
	cats := map[string]bool{}
	for _, f := range factions {
		if f.Category != "" {
			cats[f.Category] = true
		}
	}
	for c := range cats {
		if _, ok := d.Systems[c]; !ok {
			return fmt.Errorf("suit.systems 缺少系别 %q（八派 category 已使用）", c)
		}
	}
	for name, sys := range d.Systems {
		if len(sys.Main) == 0 {
			return fmt.Errorf("suit.systems[%s].main 不得为空", name)
		}
		for _, a := range sys.Main {
			if !suitNumericAttrs[a] {
				return fmt.Errorf("suit.systems[%s].main 含非法属性: %q", name, a)
			}
		}
		if !suitRateAttrs[sys.Sub] && !suitNumericAttrs[sys.Sub] {
			return fmt.Errorf("suit.systems[%s].sub 含非法属性: %q", name, sys.Sub)
		}
		if len(sys.SubPct) != 5 {
			return fmt.Errorf("suit.systems[%s].sub_pct 长度必须为 5", name)
		}
	}
	// 八派微调：逐派必须齐全、幅度不超过同档主项的一半。
	if len(factions) > 0 && len(d.FactionTweaks) != len(factions) {
		return fmt.Errorf("suit.faction_tweaks 须覆盖全部 %d 派，实际 %d", len(factions), len(d.FactionTweaks))
	}
	for _, f := range factions {
		tw, ok := d.FactionTweaks[f.Name]
		if !ok {
			return fmt.Errorf("suit.faction_tweaks 缺门派: %s", f.Name)
		}
		if len(tw.Pct) != 5 {
			return fmt.Errorf("suit.faction_tweaks[%s].pct 长度必须为 5", f.Name)
		}
		implemented := tw.ImplementedAttr()
		if suitPendingAttrs[tw.Attr] {
			if implemented {
				return fmt.Errorf("suit.faction_tweaks[%s] 的 %q 引擎尚未接入，必须标 implemented=false", f.Name, tw.Attr)
			}
		} else if !suitRateAttrs[tw.Attr] && !suitNumericAttrs[tw.Attr] {
			return fmt.Errorf("suit.faction_tweaks[%s].attr 非法: %q", f.Name, tw.Attr)
		}
		for i := 0; i < 5; i++ {
			if tw.Pct[i] < 0 {
				return fmt.Errorf("suit.faction_tweaks[%s].pct 不得为负", f.Name)
			}
			if half := d.Tier2MainPct[i] / 2; tw.Pct[i] > half {
				return fmt.Errorf("suit.faction_tweaks[%s] 档 %d 幅度 %.2f%% 超过主项一半 %.2f%%",
					f.Name, i+1, tw.Pct[i], half)
			}
		}
	}
	return nil
}

// loadSuitDir 读取套装域（suit/suit.json）。
func loadSuitDir(dir string) (SuitData, error) {
	var d SuitData
	raw, err := os.ReadFile(filepath.Join(dir, "suit", "suit.json"))
	if err != nil {
		return d, fmt.Errorf("读 suit/suit.json: %w", err)
	}
	if err := unmarshalStrict(raw, &d); err != nil {
		return d, fmt.Errorf("解析 suit/suit.json: %w", err)
	}
	return d, nil
}

// ---------- 宗派战胜场加成（V1.29 新增，数值规范 §9.6） ----------

// SectWarTier 为一个胜场档位。
type SectWarTier struct {
	Wins   int     `json:"wins"`
	AllPct float64 `json:"all_pct"`
}

// SectWarData 为宗派战胜场加成域（sect_war/sect_war.json）。
type SectWarData struct {
	Tiers          []SectWarTier `json:"tiers"`
	ResetPerSeason bool          `json:"reset_per_season"`
}

// SectWarBonus 取当前累计胜场对应的最高档（只取最高档、不叠加）。
// 返回档位序号（1 起，0=未达标）与全属性百分比。
func (d SectWarData) SectWarBonus(wins int) (int, float64) {
	tier, pct := 0, 0.0
	for _, t := range d.Tiers {
		if wins >= t.Wins {
			tier++
			pct = t.AllPct
		}
	}
	return tier, pct
}

// validateSectWar 校验宗派战胜场域。
func validateSectWar(d SectWarData) error {
	if len(d.Tiers) == 0 {
		return fmt.Errorf("sect_war.tiers 不得为空")
	}
	prev := 0
	for i, t := range d.Tiers {
		if t.Wins <= prev {
			return fmt.Errorf("sect_war.tiers 胜场必须严格递增（档 %d）", i+1)
		}
		if t.AllPct < 0 {
			return fmt.Errorf("sect_war.tiers[%d].all_pct 不得为负", i+1)
		}
		prev = t.Wins
	}
	return nil
}

// loadSectWarDir 读取宗派战胜场域（sect_war/sect_war.json）。
func loadSectWarDir(dir string) (SectWarData, error) {
	var d SectWarData
	raw, err := os.ReadFile(filepath.Join(dir, "sect_war", "sect_war.json"))
	if err != nil {
		return d, fmt.Errorf("读 sect_war/sect_war.json: %w", err)
	}
	if err := unmarshalStrict(raw, &d); err != nil {
		return d, fmt.Errorf("解析 sect_war/sect_war.json: %w", err)
	}
	return d, nil
}
