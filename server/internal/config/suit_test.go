package config

import (
	"math"
	"testing"
)

// realSuit 读仓库里真实的两张表（V1.29），确保测试跟文档口径同步。
func realSuit(t *testing.T) SuitData {
	t.Helper()
	d, err := loadSuitDir("../../configs/source")
	if err != nil {
		t.Fatalf("读真实套装配置: %v", err)
	}
	return d
}

func realSectWar(t *testing.T) SectWarData {
	t.Helper()
	d, err := loadSectWarDir("../../configs/source")
	if err != nil {
		t.Fatalf("读真实宗派战配置: %v", err)
	}
	return d
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSuitTier1OnlyAllAttrs(t *testing.T) {
	d := realSuit(t)
	// 3 件良品（序数 2）→ 一档全属性 +2%，无主项/副项/微调。
	r := d.SuitResonance([]string{"良品", "良品", "良品", "", ""}, "断岳剑宗", "物理")
	if r.Tier != 1 || r.Count != 3 || !approx(r.AllPct, 2) {
		t.Fatalf("一档结果不符: %+v", r)
	}
	if r.MainPct != 0 || r.SubPct != 0 || r.TweakPct != 0 {
		t.Fatalf("一档不应有主项/副项/微调: %+v", r)
	}
	for _, a := range d.AllAttrs {
		if !approx(r.Percent[a], 2) {
			t.Fatalf("全属性 %s 应为 2%%，实际 %v", a, r.Percent[a])
		}
	}
}

func TestSuitTier2StackWithSub(t *testing.T) {
	d := realSuit(t)
	// 断岳剑宗 5 件完美：全属性 +5%、物攻 +9%、暴击 3.5% + 微调 3% = 6.5%（配置表 §4）。
	r := d.SuitResonance([]string{"完美", "完美", "完美", "完美", "完美"}, "断岳剑宗", "物理")
	if r.Tier != 2 || !approx(r.AllPct, 5) || !approx(r.MainPct, 9) {
		t.Fatalf("二档基础不符: %+v", r)
	}
	if !approx(r.Percent["patk"], 14) {
		t.Fatalf("物攻应为全属性5%%+主项9%%=14%%，实际 %v", r.Percent["patk"])
	}
	if !approx(r.Percent["pcrit"], 6.5) || !approx(r.TweakPct, 3) {
		t.Fatalf("剑宗暴击应为副项3.5%%+微调3%%=6.5%%，实际 %v (tweak=%v)", r.Percent["pcrit"], r.TweakPct)
	}
}

func TestSuitSealHitTenPercent(t *testing.T) {
	d := realSuit(t)
	// 符观完美：封印命中 副项 7% + 微调 3% = 10%（V1.29 定案的重点校准项）。
	r := d.SuitResonance([]string{"完美", "完美", "完美", "完美", "完美"}, "缚灵符观", "封印")
	if !approx(r.Percent["sealhit"], 10) {
		t.Fatalf("符观封印命中应为 10%%，实际 %v", r.Percent["sealhit"])
	}
	if !approx(r.Percent["speed"], 14) {
		t.Fatalf("封印系主项为速度 9%%+全属性5%%=14%%，实际 %v", r.Percent["speed"])
	}
}

func TestSuitZhimengDodgeOnly(t *testing.T) {
	d := realSuit(t)
	// 织梦谷微调只取闪避；速度是主项，不在主项上叠主项（V1.29 口径）。
	r := d.SuitResonance([]string{"完美", "完美", "完美", "完美", "完美"}, "织梦谷", "封印")
	if !approx(r.Percent["pdodge"], 3) {
		t.Fatalf("织梦谷闪避应为 3%%，实际 %v", r.Percent["pdodge"])
	}
	if !approx(r.Percent["sealhit"], 7) {
		t.Fatalf("织梦谷封印命中只吃副项 7%%，实际 %v", r.Percent["sealhit"])
	}
	if !approx(r.Percent["speed"], 14) {
		t.Fatalf("速度应为 9%%+5%%=14%%，实际 %v", r.Percent["speed"])
	}
}

func TestSuitPendingTweaksNotApplied(t *testing.T) {
	d := realSuit(t)
	// 星宫蓄力伤害、禅院反伤、玄水治疗加成引擎未接入，不写进 Percent（不伪造生效）。
	for _, c := range []struct{ faction, attr string }{
		{"璇玑星宫", "charge_dmg"}, {"金刚禅院", "reflect"}, {"玄水宫", "heal_bonus"},
	} {
		cat := "法术"
		if c.faction == "金刚禅院" || c.faction == "玄水宫" {
			cat = "肉盾"
		}
		r := d.SuitResonance([]string{"完美", "完美", "完美", "完美", "完美"}, c.faction, cat)
		if _, has := r.Percent[c.attr]; has {
			t.Fatalf("%s 的 %s 未接入，不应出现在加成里", c.faction, c.attr)
		}
		if r.TweakPct != 0 {
			t.Fatalf("%s 未接入的微调幅度应为 0，实际 %v", c.faction, r.TweakPct)
		}
	}
}

func TestSuitMixedQualityUsesMajority(t *testing.T) {
	d := realSuit(t)
	// 4 件精品 + 1 件完美：只按件数最多的精品结算一档，不能因为有一件完美就跳档。
	r := d.SuitResonance([]string{"精品", "精品", "精品", "精品", "完美"}, "断岳剑宗", "物理")
	if r.Tier != 1 || r.Quality != "精品" || !approx(r.AllPct, 3) {
		t.Fatalf("混品质应按精品一档 +3%%: %+v", r)
	}
}

func TestSuitInactiveBelowThree(t *testing.T) {
	d := realSuit(t)
	r := d.SuitResonance([]string{"完美", "完美", "", "", ""}, "断岳剑宗", "物理")
	if r.Tier != 0 || len(r.Percent) != 0 {
		t.Fatalf("不足 3 件不应激活: %+v", r)
	}
}

func TestSectWarTierPicksHighestOnly(t *testing.T) {
	d := realSectWar(t)
	cases := []struct {
		wins int
		tier int
		pct  float64
	}{
		{0, 0, 0}, {9, 0, 0}, {10, 1, 1}, {29, 1, 1}, {30, 2, 2},
		{59, 2, 2}, {60, 3, 3}, {99, 3, 3}, {100, 4, 4}, {9999, 4, 4},
	}
	for _, c := range cases {
		tier, pct := d.SectWarBonus(c.wins)
		if tier != c.tier || !approx(pct, c.pct) {
			t.Fatalf("%d 胜应为档 %d / +%v%%，实际 档 %d / %v%%",
				c.wins, c.tier, c.pct, tier, pct)
		}
	}
}

func TestValidateSuitRejectsTweakOverHalfMain(t *testing.T) {
	d := realSuit(t)
	tw := d.FactionTweaks["断岳剑宗"]
	tw.Pct = []float64{1, 1.5, 2, 2.5, 6} // 完美档主项 9 的一半是 4.5
	d.FactionTweaks["断岳剑宗"] = tw
	if err := validateSuit(d, eightFactions()); err == nil {
		t.Fatal("微调超主项一半应被拒绝")
	}
}

func TestValidateSuitRejectsMissingFaction(t *testing.T) {
	d := realSuit(t)
	delete(d.FactionTweaks, "玄水宫")
	if err := validateSuit(d, eightFactions()); err == nil {
		t.Fatal("缺门派微调应被拒绝")
	}
}

func TestValidateSuitRejectsPendingMarkedImplemented(t *testing.T) {
	d := realSuit(t)
	tw := d.FactionTweaks["金刚禅院"]
	yes := true
	tw.Implemented = &yes
	d.FactionTweaks["金刚禅院"] = tw
	if err := validateSuit(d, eightFactions()); err == nil {
		t.Fatal("引擎未接入的属性标 implemented=true 应被拒绝")
	}
}

func TestValidateSectWarRejectsUnsorted(t *testing.T) {
	d := realSectWar(t)
	d.Tiers = []SectWarTier{{Wins: 30, AllPct: 2}, {Wins: 10, AllPct: 1}}
	if err := validateSectWar(d); err == nil {
		t.Fatal("胜场未递增应被拒绝")
	}
}

// eightFactions 为校验用八派最小集（名称与真实 factions.json 一致）。
func eightFactions() []FactionInfo {
	return []FactionInfo{
		{Name: "断岳剑宗", Category: "物理"}, {Name: "破军枪府", Category: "物理"},
		{Name: "惊雷殿", Category: "法术"}, {Name: "璇玑星宫", Category: "法术"},
		{Name: "缚灵符观", Category: "封印"}, {Name: "织梦谷", Category: "封印"},
		{Name: "金刚禅院", Category: "肉盾"}, {Name: "玄水宫", Category: "肉盾"},
	}
}
