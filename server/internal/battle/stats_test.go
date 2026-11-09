package battle

import (
	"math"
	"testing"

	"fantasy-jianghu/server/internal/config"
)

func testBattleCfg(t *testing.T) config.BattleData {
	t.Helper()
	// 从真实打包产物加载（保证与配置管线同源）。
	dir := packBattleForStats(t)
	data, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return data.Battle
}

func packBattleForStats(t *testing.T) string {
	t.Helper()
	// 复用 configtool 打包真实源。
	return packViaTool(t)
}

func TestDerivedStatsAnchors(t *testing.T) {
	cfg := testBattleCfg(t)
	// Lv20 物理输出（断岳剑宗）锚点逐项等于数值规范 §5.2 表值。
	s, err := DerivedStats(20, "断岳剑宗", cfg)
	if err != nil {
		t.Fatalf("DerivedStats: %v", err)
	}
	if s.HP != 4515 || s.MP != 2902 || s.PAtk != 1422 || s.MAtk != 582 || s.PDef != 376 || s.MDef != 376 || s.Speed != 1089 {
		t.Fatalf("Lv20 物理锚点不一致: %+v", s)
	}
}

func TestDerivedStatsInterpolation(t *testing.T) {
	cfg := testBattleCfg(t)
	// Lv45 = Lv20/Lv70 线性中点（物理 HP (4515+27741)/2 = 16128）。
	s, err := DerivedStats(45, "断岳剑宗", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.HP != 16128 {
		t.Fatalf("Lv45 插值 HP = %d, 期望 16128", s.HP)
	}
}

func TestDerivedStatsTankDerive(t *testing.T) {
	cfg := testBattleCfg(t)
	// 金刚禅院 = 物理锚点 × 系数（HP 1.30 / 攻 0.75 / 防 1.25 / 速 0.85）。
	s, err := DerivedStats(20, "金刚禅院", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.HP != int(math.Round(4515*1.30)) {
		t.Fatalf("金刚 HP = %d, 期望 %d", s.HP, int(math.Round(4515*1.30)))
	}
	if s.PAtk != int(math.Round(1422*0.75)) {
		t.Fatalf("金刚 PAtk = %d", s.PAtk)
	}
	if s.Speed != int(math.Round(1089*0.85)) {
		t.Fatalf("金刚 Speed = %d", s.Speed)
	}
}

func TestDerivedStatsLowLevelCurve(t *testing.T) {
	cfg := testBattleCfg(t)
	// Lv1 = Lv20 锚点 × (1/20)^low_curve_exp。M2b 起指数为 1.0（线性外延）：
	// 原 1.5 会让 Lv1 只有 Lv20 的 1.1%（HP 57），新手连训练木桩都打不过。
	s, err := DerivedStats(1, "断岳剑宗", cfg)
	if err != nil {
		t.Fatal(err)
	}
	exp := cfg.ClassBook.LowCurveExp
	base := float64(cfg.ClassBook.LowCurveBaseLv)
	if exp <= 0 || base <= 0 {
		t.Fatalf("低级外延参数缺失: exp=%v base=%v", exp, base)
	}
	want := int(math.Round(4515 * math.Pow(1.0/base, exp)))
	if want < 1 {
		t.Fatalf("低级外延应为正，得 %d", want)
	}
	if s.HP != want {
		t.Fatalf("Lv1 HP = %d, 期望 %d（exp=%v base=%v）", s.HP, want, exp, base)
	}
}

func TestDerivedStatsUnknownFaction(t *testing.T) {
	cfg := testBattleCfg(t)
	if _, err := DerivedStats(20, "无名门派", cfg); err == nil {
		t.Fatal("未知门派应报错")
	}
}

// TestLowLevelStarterViable 防止低等级曲线回退成"新手必败"：
// Lv1 玩家属性必须落在可打区间，且与训练木桩人对打能取胜（M2b 主线首战）。
func TestLowLevelStarterViable(t *testing.T) {
	cfg := testBattleCfg(t)
	st, err := DerivedStats(1, "惊雷殿", cfg)
	if err != nil {
		t.Fatalf("DerivedStats: %v", err)
	}
	// 低等级外延为线性（low_curve_exp=1.0）：Lv1 = Lv20 的 5%。
	if st.HP < 200 || st.MAtk < 50 || st.PDef < 10 {
		t.Fatalf("Lv1 属性过低（新手不可玩）: %+v", st)
	}
	var mon *config.MonsterDef
	for i := range cfg.Monsters {
		if cfg.Monsters[i].ID == "muzhuang" {
			mon = &cfg.Monsters[i]
		}
	}
	if mon == nil {
		t.Fatal("配置缺木桩人")
	}
	player := &Unit{ID: 1, Name: "雷震子", Side: 0, Level: 1, Faction: "惊雷殿",
		BaseStats: st, Stats: st, HP: st.HP, MP: st.MP, Alive: true}
	enemy := monsterUnit(mon, 2, 0)
	b := NewBattle(7, []*Unit{player}, []*Unit{enemy}, cfg)
	for turn := 0; turn < 40 && !b.Over; turn++ {
		b.SubmitCmd(player.ID, Cmd{Type: "attack"})
		b.SubmitCmd(enemy.ID, Cmd{Type: "attack"})
		b.ResolveTurn()
	}
	if !b.Over {
		t.Fatalf("40 回合仍未分胜负（木桩人 HP=%d 玩家攻击=%d）", enemy.HP, st.PAtk)
	}
	if !player.Alive {
		t.Fatalf("Lv1 角色败给训练木桩人（玩家 HP=%d，木桩攻=%d）——请检查 low_curve_exp 与木桩人数值", player.HP, mon.Stats.PAtk)
	}
}
