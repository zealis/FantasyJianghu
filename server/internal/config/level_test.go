package config

import "testing"

func TestLevelCurve(t *testing.T) {
	d := LevelData{MaxLevel: 89, ExpBase: 100, ExpExponent: 1.5,
		Gates: []int{29, 39, 49, 59, 69, 79}, PointsPerLevel: 4}
	if got := d.ExpToNext(1); got != 100 {
		t.Fatalf("ExpToNext(1) = %d, 期望 100", got)
	}
	if got := d.ExpToNext(2); got != 283 {
		t.Fatalf("ExpToNext(2) = %d, 期望 283", got)
	}
	if got := d.ExpToNext(89); got != 0 {
		t.Fatalf("满级 ExpToNext 应 0: %d", got)
	}
	if !d.IsGate(29) || d.IsGate(30) {
		t.Fatalf("关口判定错误")
	}
}

func TestValidateLevelsRejects(t *testing.T) {
	valid := func() LevelData {
		return LevelData{MaxLevel: 89, ExpBase: 100, ExpExponent: 1.5,
			Gates: []int{29, 39, 49, 59, 69, 79}, PointsPerLevel: 4}
	}
	cases := []struct {
		name   string
		mutate func(d *LevelData)
	}{
		{"满级越界", func(d *LevelData) { d.MaxLevel = 90 }},
		{"经验基数非正", func(d *LevelData) { d.ExpBase = 0 }},
		{"指数非正", func(d *LevelData) { d.ExpExponent = 0 }},
		{"关口非递增", func(d *LevelData) { d.Gates = []int{39, 29} }},
		{"关口个位非9", func(d *LevelData) { d.Gates = []int{28} }},
		{"关口超满级", func(d *LevelData) { d.Gates = []int{89} }},
		{"属性点负", func(d *LevelData) { d.PointsPerLevel = -1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := valid()
			c.mutate(&d)
			if err := validateLevels(d); err == nil {
				t.Fatalf("应拒绝但通过")
			}
		})
	}
	if err := validateLevels(valid()); err != nil {
		t.Fatalf("合法配置应通过: %v", err)
	}
}