package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// LevelData 为等级域配置（level/levels.json，GDD §4.2）。
// 经验需求 ExpToNext(L) = round(exp_base * L^exp_exponent)（首版助手定案·慢升级，登记待策划表替换）。
type LevelData struct {
	MaxLevel       int     `json:"max_level"`
	ExpBase        float64 `json:"exp_base"`
	ExpExponent    float64 `json:"exp_exponent"`
	Gates          []int   `json:"gates"` // 突破关口（个位为 9）
	PointsPerLevel int     `json:"points_per_level"`
	PointGains     map[string]map[string]int `json:"point_gains"` // 每点派生收益（str/spi/agi/con → 二级属性）
}

// ExpToNext 取 L 级升 L+1 级所需经验；满级返回 0。
func (d LevelData) ExpToNext(level int) int {
	if level >= d.MaxLevel || level < 1 {
		return 0
	}
	return int(math.Round(d.ExpBase * math.Pow(float64(level), d.ExpExponent)))
}

// IsGate 判断 level 是否为突破关口（离开该级需手动突破）。
func (d LevelData) IsGate(level int) bool {
	for _, g := range d.Gates {
		if g == level {
			return true
		}
	}
	return false
}

// validateLevels 校验等级域（§7.2：非法不打包）。
func validateLevels(d LevelData) error {
	if d.MaxLevel < 1 || d.MaxLevel > 89 {
		return fmt.Errorf("max_level 必须在 1..89（GDD §4.2 满级 89）")
	}
	if d.ExpBase <= 0 {
		return fmt.Errorf("exp_base 必须为正")
	}
	if d.ExpExponent <= 0 {
		return fmt.Errorf("exp_exponent 必须为正")
	}
	if d.PointsPerLevel < 0 {
		return fmt.Errorf("points_per_level 不得为负")
	}
	validPrimary := map[string]bool{"str": true, "spi": true, "agi": true, "con": true}
	validSecondary := map[string]bool{"hp": true, "mp": true, "patk": true, "matk": true,
		"pdef": true, "mdef": true, "speed": true, "dodge": true}
	for primary, gains := range d.PointGains {
		if !validPrimary[primary] {
			return fmt.Errorf("point_gains 一级属性非法: %q", primary)
		}
		for sec, v := range gains {
			if !validSecondary[sec] {
				return fmt.Errorf("point_gains[%s] 二级属性非法: %q", primary, sec)
			}
			if v < 0 {
				return fmt.Errorf("point_gains[%s][%s] 不得为负", primary, sec)
			}
		}
	}
	prev := 0
	for _, g := range d.Gates {
		if g <= prev || g >= d.MaxLevel {
			return fmt.Errorf("gates 必须递增且小于 max_level: %v", d.Gates)
		}
		if g%10 != 9 {
			return fmt.Errorf("突破关口个位必须为 9（GDD §4.2）: %d", g)
		}
		prev = g
	}
	return nil
}

// loadLevelDir 读取等级域（level/levels.json）。
func loadLevelDir(dir string) (LevelData, error) {
	var lv LevelData
	raw, err := os.ReadFile(filepath.Join(dir, "level", "levels.json"))
	if err != nil {
		return lv, fmt.Errorf("读 level/levels.json: %w", err)
	}
	if err := unmarshalStrict(raw, &lv); err != nil {
		return lv, fmt.Errorf("解析 level/levels.json: %w", err)
	}
	return lv, nil
}