package battle

import (
	"encoding/json"
	"testing"

	"fantasy-jianghu/server/internal/config"
)

// TestApplyEquipAndSuitOrder 校验顺序：装备先做数值累加，共鸣百分比再乘上去。
func TestApplyEquipAndSuitOrder(t *testing.T) {
	st := UnitStats{HP: 1000, MP: 500, PAtk: 100, MAtk: 100, PDef: 50, MDef: 50, Speed: 100}
	equip := map[string]int{"hp": 200, "patk": 40}
	pct := map[string]float64{
		"hp": 5, "mp": 5, "patk": 5, "matk": 5, "pdef": 5, "mdef": 5, "speed": 5, // 一档全属性 +5%
	}
	got := applyEquipAndSuit(st, equip, pct)
	// HP:(1000+200)*1.05 = 1260；物攻:(100+40)*1.05 = 147
	if got.HP != 1260 {
		t.Fatalf("HP 应为 (1000+200)*1.05=1260，实际 %d", got.HP)
	}
	if got.PAtk != 147 {
		t.Fatalf("物攻应为 (100+40)*1.05=147，实际 %d", got.PAtk)
	}
	if got.MP != 525 || got.Speed != 105 {
		t.Fatalf("无装备项也应吃全属性：mp=%d speed=%d", got.MP, got.Speed)
	}
}

// TestApplySuitRateAttrs 校验概率型按百分点相加（万分制），不与数值型混用乘算。
func TestApplySuitRateAttrs(t *testing.T) {
	st := UnitStats{PCrit: 500, MCrit: 500, PCombo: 0, SealHit: 0, PDodge: 0}
	got := applyEquipAndSuit(st, nil, map[string]float64{
		"pcrit": 6.5, "mcrit": 6.5, "pcombo": 3, "sealhit": 10, "pdodge": 3,
	})
	if got.PCrit != 1150 || got.MCrit != 1150 {
		t.Fatalf("暴击/法暴应为 500+650=1150 万分制，实际 %d/%d", got.PCrit, got.MCrit)
	}
	if got.PCombo != 300 || got.SealHit != 1000 || got.PDodge != 300 {
		t.Fatalf("连击/封印命中/闪避万分制不符: %d/%d/%d", got.PCombo, got.SealHit, got.PDodge)
	}
}

// TestApplySuitNeverLowersStats 防御性：百分比为 0 或负时不降低属性。
func TestApplySuitNeverLowersStats(t *testing.T) {
	st := UnitStats{HP: 100, PAtk: 10}
	got := applyEquipAndSuit(st, nil, map[string]float64{"hp": 0, "patk": 0})
	if got.HP != 100 || got.PAtk != 10 {
		t.Fatalf("零加成不应改变属性: %+v", got)
	}
}

// TestSuitResonanceDrivesStats 端到端：真实套装表 + 五件完美 → 符观封印命中 10%。
func TestSuitResonanceDrivesStats(t *testing.T) {
	d := config.SuitData{}
	raw := `{"all_attrs":["hp","mp","patk","matk","pdef","mdef","speed"],
"quality_order":{"凡品":1,"良品":2,"精品":3,"极品":4,"完美":5},
"tier1_all_pct":[1,2,3,4,5],"tier2_main_pct":[5,6,7,8,9],
"systems":{"封印":{"main":["speed"],"sub":"sealhit","sub_pct":[3,4,5,6,7]}},
"faction_tweaks":{"缚灵符观":{"attr":"sealhit","pct":[1,1.5,2,2.5,3]}}}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("解析测试套装配置: %v", err)
	}
	res := d.SuitResonance([]string{"完美", "完美", "完美", "完美", "完美"}, "缚灵符观", "封印")
	if res.Percent["sealhit"] != 10 {
		t.Fatalf("符观完美档封印命中应为 10%%，实际 %v", res.Percent["sealhit"])
	}
	st := applyEquipAndSuit(UnitStats{Speed: 200}, nil, res.Percent)
	if st.SealHit != 1000 {
		t.Fatalf("封印命中应折算为 1000 万分制，实际 %d", st.SealHit)
	}
	// 速度：主项 9% + 全属性 5% = 14% → 200*1.14 = 228
	if st.Speed != 228 {
		t.Fatalf("速度应为 200*1.14=228，实际 %d", st.Speed)
	}
}
