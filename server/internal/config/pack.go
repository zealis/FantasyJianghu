package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pack 读取 source 目录（app.json + factions.json + scenes/*.json），校验通过后
// 打包为 out/<version>/ 版本化配置包（manifest 带版本号与哈希，§7.3）。
func Pack(source, out, version string) error {
	appRaw, err := os.ReadFile(filepath.Join(source, "app.json"))
	if err != nil {
		return fmt.Errorf("读 app.json: %w", err)
	}
	var app AppSettings
	if err := unmarshalStrict(appRaw, &app); err != nil {
		return fmt.Errorf("解析 app.json: %w", err)
	}
	data := Data{Version: version, App: app, Scenes: map[string]SceneLogic{}}

	// 八派明细（GDD §5.3）。
	facRaw, err := os.ReadFile(filepath.Join(source, "factions.json"))
	if err != nil {
		return fmt.Errorf("读 factions.json: %w", err)
	}
	var factions []FactionInfo
	if err := unmarshalStrictArray(facRaw, &factions); err != nil {
		return fmt.Errorf("解析 factions.json: %w", err)
	}
	data.Factions = factions

	sceneDir := filepath.Join(source, "scenes")
	entries, err := os.ReadDir(sceneDir)
	if err != nil {
		return fmt.Errorf("读 scenes 目录: %w", err)
	}
	sceneRaws := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sceneDir, e.Name()))
		if err != nil {
			return err
		}
		var s SceneLogic
		if err := unmarshalStrict(raw, &s); err != nil {
			return fmt.Errorf("解析 scenes/%s: %w", e.Name(), err)
		}
		if s.MapID == "" {
			return fmt.Errorf("scenes/%s 缺 map_id", e.Name())
		}
		if _, dup := data.Scenes[s.MapID]; dup {
			return fmt.Errorf("场景 map_id 重复: %s", s.MapID)
		}
		data.Scenes[s.MapID] = s
		sceneRaws[e.Name()] = raw
	}

	// 战斗域解析与复核（§7.2：校验不通过不生成配置包）。
	battle, err := loadBattleDirFromData(source)
	if err != nil {
		return err
	}
	data.Battle = battle
	// 任务域与物品字典（GDD §9；引用完整性在 Validate 内统一复核）。
	quests, err := loadQuestDir(source)
	if err != nil {
		return err
	}
	data.Quests = quests
	items, err := loadItemDir(source)
	if err != nil {
		return err
	}
	data.Items = items

	// 结构与引用完整性校验：不通过不生成配置包（§7.2）。
	if err := Validate(data); err != nil {
		return fmt.Errorf("校验不通过: %w", err)
	}

	target := filepath.Join(out, version)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	mf := Manifest{Version: version, GeneratedAt: time.Now().Unix()}
	if err := appendPacked(&mf, target, "app.json", appRaw); err != nil {
		return err
	}
	if err := appendPacked(&mf, target, "factions.json", facRaw); err != nil {
		return err
	}
	// 战斗域（battle/*.json）：结构校验在 data 校验时已随 Load 完成。
	if err := packBattleDir(&mf, target, source); err != nil {
		return err
	}
	// 任务域与物品字典（GDD §9）。
	if err := packConfigFile(&mf, target, source, "quest/quests.json"); err != nil {
		return err
	}
	if err := packConfigFile(&mf, target, source, "item/items.json"); err != nil {
		return err
	}
	for name, raw := range sceneRaws {
		if err := appendPacked(&mf, target, "scenes/"+name, raw); err != nil {
			return err
		}
	}
	return writeJSONFile(filepath.Join(target, "manifest.json"), mf)
}

func unmarshalStrict(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func unmarshalStrictArray(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func appendPacked(mf *Manifest, target, name string, raw []byte) error {
	p := filepath.Join(target, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	mf.Files = append(mf.Files, ManifestFile{
		Name: name, SHA256: hex.EncodeToString(sum[:]), Size: len(raw),
	})
	return nil
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
