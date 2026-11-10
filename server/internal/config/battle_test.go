package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packBattleSource 打包含战斗域配置的合法源。
func packBattleSource(t *testing.T) string {
	t.Helper()
	src := packValidSource(t) // 复用 M1.5 夹具（app/factions/scenes）
	battleDir := filepath.Join(t.TempDir(), "battle")
	if err := os.MkdirAll(battleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"formula.json", "classes.json", "skills_leidian.json", "monsters.json"} {
		raw, err := os.ReadFile(filepath.Join(projectBattleSrc(t), f))
		if err != nil {
			t.Fatalf("读战斗源 %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(battleDir, f), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 将 battle 目录并入打包源（configtool 支持子目录打包）。
	if err := copyDir(battleDir, filepath.Join(src, "battle")); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := Pack(src, out, "1.1.0"); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return filepath.Join(out, "1.1.0")
}

func projectBattleSrc(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "configs", "source", "battle"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func TestBattleConfigLoad(t *testing.T) {
	dir := packBattleSource(t)
	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b := data.Battle
	if b.Formula.Damage.K != 1.0 {
		t.Fatalf("K = %v, 期望 1.0（回归锁定）", b.Formula.Damage.K)
	}
	if b.Formula.MultiplierCap != 2.25 {
		t.Fatalf("乘区上限 = %v", b.Formula.MultiplierCap)
	}
	if b.Formula.Control.ComboDecay != 0.70 || b.Formula.Control.ImmuneTurns != 1 {
		t.Fatalf("控制参数不一致: %+v", b.Formula.Control)
	}
	// 秘籍谱按门派分文件：八派各 10 本（含 1 级心法），合并后按 Faction 归属。
	byFaction := map[string]int{}
	heart := map[string]int{}
	for _, sk := range b.Skills {
		byFaction[sk.Faction]++
		if sk.Category == "心法" && sk.LevelReq <= 1 {
			heart[sk.Faction]++
		}
	}
	if len(byFaction) != 8 {
		t.Fatalf("应有八派秘籍谱，实际 %d 派: %+v", len(byFaction), byFaction)
	}
	for f, n := range byFaction {
		if n != 10 {
			t.Fatalf("门派 %s 秘籍 = %d 本，期望 10 本", f, n)
		}
		if heart[f] != 1 {
			t.Fatalf("门派 %s 必须有且只有 1 本 1 级心法，实际 %d", f, heart[f])
		}
	}
	if len(b.Skills) != 80 {
		t.Fatalf("合并后秘籍 = %d 本, 期望 80（八派 × 10）", len(b.Skills))
	}
	tianlei := b.SkillByID("tianlei_po")
	if tianlei == nil || len(tianlei.Segments) != 1 || tianlei.Segments[0].Mult != 160 {
		t.Fatalf("天雷破解析不一致: %+v", tianlei)
	}
	if len(b.Monsters) != 4 {
		t.Fatalf("野怪 = %d, 期望 4（M2a-2 增荆棘武士）", len(b.Monsters))
	}
	// M2a-2 效果/演练参数逐项断言（配置驱动，禁止零值静默放行）。
	if b.Formula.Effects.Shock.MaxStacks != 3 ||
		b.Formula.Effects.Shock.DmgTakenPctPerStack != 400 ||
		b.Formula.Effects.Shock.DOTPctOfMAtkPerStack != 300 ||
		b.Formula.Effects.Shock.DOTTurns != 3 ||
		b.Formula.Effects.Shock.BonusVsShockedPct != 1000 {
		t.Fatalf("感电参数不一致: %+v", b.Formula.Effects.Shock)
	}
	if b.Formula.Effects.Reflect.PerStackPct != 300 || b.Formula.Effects.Reflect.MaxPct != 1500 {
		t.Fatalf("反伤参数不一致: %+v", b.Formula.Effects.Reflect)
	}
	if b.Formula.Drill.Allies != 3 || len(b.Formula.Drill.Enemies) != 4 {
		t.Fatalf("4v4 演练阵容不一致: %+v", b.Formula.Drill)
	}
	if jingji := func() *MonsterDef {
		for i := range b.Monsters {
			if b.Monsters[i].ID == "jingji_wushi" {
				return &b.Monsters[i]
			}
		}
		return nil
	}(); jingji == nil || jingji.ReflectPct != 900 {
		t.Fatalf("荆棘武士反伤未入配置: %+v", jingji)
	}
}

func TestBattleValidateRejects(t *testing.T) {
	dir := packBattleSource(t)
	// K 改动（回归锁定值）→ 拒载。
	if err := tweakBattleFile(dir, "formula.json", `"K": 1.0`, `"K": 1.5`); err != nil {
		t.Fatal(err)
	}
	refreshBattleManifest(t, dir)
	if _, err := Load(dir); err == nil {
		t.Fatal("K ≠ 1.0 应拒绝加载（公式表回归锁定）")
	}
}

func TestBattleValidateRejectsUnknownSpecial(t *testing.T) {
	dir := packBattleSource(t)
	if err := tweakBattleFile(dir, "skills_leidian.json", `"special": "shock_bonus_10"`, `"special": "fire_bonus_10"`); err != nil {
		t.Fatal(err)
	}
	refreshBattleManifest(t, dir)
	if _, err := Load(dir); err == nil {
		t.Fatal("未登记的 special 应拒绝加载（配置驱动，禁止未实现特效静默）")
	}
}

func tweakBattleFile(dir, name, oldS, newS string) error {
	p := filepath.Join(dir, "battle", name)
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	s := string(raw)
	if !contains(s, oldS) {
		return errNotFound
	}
	return os.WriteFile(p, []byte(replaceStr(s, oldS, newS)), 0o644)
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func replaceStr(s, oldS, newS string) string {
	return strings.ReplaceAll(s, oldS, newS)
}

var errNotFound = errNotFoundErr{}

type errNotFoundErr struct{}

func (errNotFoundErr) Error() string { return "pattern not found" }

// refreshBattleManifest 重算 battle 子目录文件的 manifest 哈希。
func refreshBattleManifest(t *testing.T, dir string) {
	t.Helper()
	mfPath := filepath.Join(dir, "manifest.json")
	b, err := os.ReadFile(mfPath)
	if err != nil {
		t.Fatal(err)
	}
	var mf Manifest
	if err := jsonUnmarshal(b, &mf); err != nil {
		t.Fatal(err)
	}
	changed := false
	for i := range mf.Files {
		p := filepath.Join(dir, filepath.FromSlash(mf.Files[i].Name))
		if _, err := os.Stat(p); err != nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		mf.Files[i].SHA256 = sha256Hex(data)
		mf.Files[i].Size = len(data)
		changed = true
	}
	if !changed {
		t.Fatal("manifest 无 battle 文件（打包未收录 battle 域）")
	}
	out, err2 := jsonMarshal(mf)
	if err2 != nil {
		t.Fatal(err2)
	}
	if err := os.WriteFile(mfPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
