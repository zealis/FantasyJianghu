package battle

import (
	"testing"

	"fantasy-jianghu/server/internal/config"
)

func TestDerivedStatsWithPoints(t *testing.T) {
	cfg := config.BattleData{
		Classes: []config.BattleClass{{Class: "物理", Factions: []string{"断岳剑宗"},
			Anchors: map[string]config.AnchorStat{"20": {HP: 1000, MP: 480, PAtk: 100, MAtk: 50, PDef: 60, MDef: 50, Speed: 100}}}},
		ClassBook: config.ClassBook{LowCurveExp: 1.0, LowCurveBaseLv: 20},
	}
	gains := map[string]map[string]int{
		"str": {"patk": 2}, "con": {"hp": 10, "pdef": 1}, "agi": {"speed": 1, "dodge": 10},
	}
	base, err := DerivedStatsWithPoints(10, "断岳剑宗", cfg, gains, nil)
	if err != nil {
		t.Fatal(err)
	}
	with, err := DerivedStatsWithPoints(10, "断岳剑宗", cfg, gains, map[string]int{"str": 3, "con": 2, "agi": 1})
	if err != nil {
		t.Fatal(err)
	}
	if with.PAtk != base.PAtk+6 || with.HP != base.HP+20 || with.PDef != base.PDef+2 ||
		with.Speed != base.Speed+1 || with.PDodge != base.PDodge+10 {
		t.Fatalf("点收益叠加错误: base=%+v with=%+v", base, with)
	}
}