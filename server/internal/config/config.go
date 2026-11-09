// Package config 为版本化配置包的加载与校验（开发技术规范 §7）。
// 配置非法时不使用默认值回退：调用方必须把 Load 失败视为启动失败。
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppSettings 为运行口径（数值全部可溯源：心跳=技术规范 §5.2，
// 3 角色=原资料口径，move_speed=客户端 player.gd，八派=设计文档 §5.3）。
type AppSettings struct {
	HeartbeatSeconds        int            `json:"heartbeat_seconds"`
	HeartbeatMissLimit      int            `json:"heartbeat_miss_limit"`
	MaxCharactersPerAccount int            `json:"max_characters_per_account"`
	MoveSpeed               float64        `json:"move_speed"`
	MoveTolerance           float64        `json:"move_tolerance"`
	Spawn                   Spawn          `json:"spawn"`
	Factions                []string       `json:"factions"`
	SceneCapacity           map[string]int `json:"scene_capacity"` // V1.27 分线阈值：城镇 50 / 野外与驻地 30 / 活动按活动配置
}

type Spawn struct {
	MapID string  `json:"map_id"`
	X     float64 `json:"x"`
}

// SceneLogic 为"场景图与路径/热区逻辑数据分离"的服务端逻辑数据（§4 world）。
type SceneLogic struct {
	MapID     string          `json:"map_id"`
	Type      string          `json:"type"` // town|wild|sect|activity（V1.27 分线类型）
	Texture   string          `json:"texture"`
	Width     float64         `json:"width"`
	Height    float64         `json:"height"`
	GroundY   float64         `json:"ground_y"`
	Walk      WalkRange       `json:"walk"`
	Camera    CameraRange     `json:"camera"`
	NPCs      []SceneNPC      `json:"npcs"`
	Buildings []SceneBuilding `json:"buildings"`
	Dialogues map[string]any  `json:"dialogues"`
}

type WalkRange struct {
	XMin float64 `json:"x_min"`
	XMax float64 `json:"x_max"`
}

type CameraRange struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

type SceneNPC struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Dialogue string  `json:"dialogue"`
}

type SceneBuilding struct {
	Name string  `json:"name"`
	X    float64 `json:"x"`
}

// FactionInfo 为八派明细（GDD §5.3 定案表逐字转录，宗门卡片唯一来源）。
type FactionInfo struct {
	Name      string `json:"name"`      // 断岳剑宗
	Category  string `json:"category"`  // 物理|法术|封印|肉盾
	Color     string `json:"color"`     // 朱橙|紫|青|绿（系别色标）
	Residence string `json:"residence"` // 驻地（如 高天原·断剑崖）
	Weapon    string `json:"weapon"`    // 剑|枪|杖|爪
	Feature   string `json:"feature"`   // 特色一句话
	Secret    string `json:"secret"`    // 代表秘籍
}

// Data 为服务端启动时加载的完整配置。
type Data struct {
	Version  string
	App      AppSettings
	Scenes   map[string]SceneLogic // key = map_id
	Factions []FactionInfo         // 八派明细（factions.json，顺序即展示顺序）
	Battle   BattleData            // 战斗域（formula/classes/skills/monsters）
	Quests   QuestData             // 任务域（GDD §9：主线链 + 三类日常）
	Items    ItemData              // 物品字典最小集（任务奖励引用 + 背包容量）
}

// LineCapacity 按场景类型取每线人数上限（V1.27 阈值）；未知类型返回 0。
func (d Data) LineCapacity(sceneType string) int {
	return d.App.SceneCapacity[sceneType]
}

// ManifestFile/File 为配置包清单（版本号与哈希，§7.3）。
type ManifestFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int    `json:"size"`
}

type Manifest struct {
	Version     string         `json:"version"`
	GeneratedAt int64          `json:"generated_at"`
	Files       []ManifestFile `json:"files"`
}

// Validate 校验配置结构与引用完整性（configtool 打包与 Load 复核同规则）。
func Validate(d Data) error {
	if d.App.HeartbeatSeconds <= 0 {
		return fmt.Errorf("heartbeat_seconds 必须为正")
	}
	if d.App.HeartbeatMissLimit <= 0 {
		return fmt.Errorf("heartbeat_miss_limit 必须为正")
	}
	if d.App.MaxCharactersPerAccount <= 0 || d.App.MaxCharactersPerAccount > 3 {
		return fmt.Errorf("max_characters_per_account 必须在 1..3（原资料口径 ≤3）")
	}
	if d.App.MoveSpeed <= 0 {
		return fmt.Errorf("move_speed 必须为正")
	}
	if d.App.MoveTolerance < 0 {
		return fmt.Errorf("move_tolerance 不得为负")
	}
	seen := map[string]bool{}
	for _, f := range d.App.Factions {
		if f == "" {
			return fmt.Errorf("factions 含空名")
		}
		if seen[f] {
			return fmt.Errorf("factions 重复: %s", f)
		}
		seen[f] = true
	}
	if len(d.App.Factions) == 0 {
		return fmt.Errorf("factions 不得为空")
	}
	// 分线容量（V1.27）：四类型齐备且为正。
	for _, t := range []string{"town", "wild", "sect", "activity"} {
		if d.App.SceneCapacity[t] <= 0 {
			return fmt.Errorf("scene_capacity[%s] 必须为正", t)
		}
	}
	// 场景类型枚举。
	validTypes := map[string]bool{"town": true, "wild": true, "sect": true, "activity": true}
	for _, s := range d.Scenes {
		if !validTypes[s.Type] {
			return fmt.Errorf("场景 %s 类型非法: %q（须 town/wild/sect/activity）", s.MapID, s.Type)
		}
	}
	// 八派明细（GDD §5.3）：恰 8 项、枚举合法、与 app.json 名单一致。
	validCats := map[string]bool{"物理": true, "法术": true, "封印": true, "肉盾": true}
	validColors := map[string]bool{"朱橙": true, "紫": true, "青": true, "绿": true}
	validWeapons := map[string]bool{"剑": true, "枪": true, "杖": true, "爪": true}
	if len(d.Factions) != 8 {
		return fmt.Errorf("门派明细须恰 8 项，收到 %d", len(d.Factions))
	}
	for _, f := range d.Factions {
		if !seen[f.Name] {
			return fmt.Errorf("门派明细 %q 不在 app.json factions 名单内", f.Name)
		}
		if !validCats[f.Category] {
			return fmt.Errorf("门派 %q 系别非法: %q", f.Name, f.Category)
		}
		if !validColors[f.Color] {
			return fmt.Errorf("门派 %q 色标非法: %q", f.Name, f.Color)
		}
		if !validWeapons[f.Weapon] {
			return fmt.Errorf("门派 %q 武器非法: %q", f.Name, f.Weapon)
		}
		if f.Feature == "" || f.Secret == "" || f.Residence == "" {
			return fmt.Errorf("门派 %q 明细缺字段", f.Name)
		}
	}
	if _, ok := d.Scenes[d.App.Spawn.MapID]; !ok {
		return fmt.Errorf("spawn.map_id %q 不在场景表内", d.App.Spawn.MapID)
	}
	for _, s := range d.Scenes {
		if s.Width <= 0 || s.Height <= 0 {
			return fmt.Errorf("场景 %s 尺寸非法", s.MapID)
		}
		if s.Walk.XMin >= s.Walk.XMax || s.Walk.XMin < 0 || s.Walk.XMax > s.Width {
			return fmt.Errorf("场景 %s walk 区间非法: [%v, %v] / 宽 %v", s.MapID, s.Walk.XMin, s.Walk.XMax, s.Width)
		}
		if s.GroundY < 0 || s.GroundY > s.Height {
			return fmt.Errorf("场景 %s ground_y %v 越界", s.MapID, s.GroundY)
		}
		ids := map[string]bool{}
		for _, n := range s.NPCs {
			if n.ID == "" || ids[n.ID] {
				return fmt.Errorf("场景 %s NPC id 空/重复: %q", s.MapID, n.ID)
			}
			ids[n.ID] = true
		}
	}
	// 任务/物品域引用完整性（须在场景与战斗域校验之后）。
	if err := validateQuests(d); err != nil {
		return err
	}
	return nil
}

// Load 读取并复核配置包：逐文件哈希复核 → 解析 → 结构复校。
// 任何一步失败即返回错误，调用方不得回退默认值。
func Load(dir string) (Data, error) {
	var data Data
	mfBytes, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return data, fmt.Errorf("读 manifest: %w", err)
	}
	var mf Manifest
	if err := json.Unmarshal(mfBytes, &mf); err != nil {
		return data, fmt.Errorf("解析 manifest: %w", err)
	}
	files := map[string]ManifestFile{}
	for _, f := range mf.Files {
		files[f.Name] = f
	}
	for _, f := range mf.Files {
		raw, err := os.ReadFile(filepath.Join(dir, f.Name))
		if err != nil {
			return data, fmt.Errorf("读 %s: %w", f.Name, err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			return data, fmt.Errorf("配置 %s 哈希失配，拒绝加载", f.Name)
		}
	}
	data.Version = mf.Version
	appRaw, err := os.ReadFile(filepath.Join(dir, "app.json"))
	if err != nil {
		return data, fmt.Errorf("读 app.json（清单缺失项）: %w", err)
	}
	var app AppSettings
	if err := json.Unmarshal(appRaw, &app); err != nil {
		return data, fmt.Errorf("解析 app.json: %w", err)
	}
	data.App = app
	facRaw, err := os.ReadFile(filepath.Join(dir, "factions.json"))
	if err != nil {
		return data, fmt.Errorf("读 factions.json（清单缺失项）: %w", err)
	}
	var factions []FactionInfo
	if err := json.Unmarshal(facRaw, &factions); err != nil {
		return data, fmt.Errorf("解析 factions.json: %w", err)
	}
	data.Factions = factions
	battle, err := loadBattleDir(dir)
	if err != nil {
		return data, err
	}
	data.Battle = battle
	quests, err := loadQuestDir(dir)
	if err != nil {
		return data, err
	}
	data.Quests = quests
	items, err := loadItemDir(dir)
	if err != nil {
		return data, err
	}
	data.Items = items
	data.Scenes = map[string]SceneLogic{}
	for name := range files {
		if filepath.Dir(name) != "scenes" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return data, fmt.Errorf("读 %s（清单缺失项）: %w", name, err)
		}
		var s SceneLogic
		if err := json.Unmarshal(raw, &s); err != nil {
			return data, fmt.Errorf("解析 %s: %w", name, err)
		}
		if s.MapID == "" {
			return data, fmt.Errorf("场景 %s 缺 map_id", name)
		}
		data.Scenes[s.MapID] = s
	}
	if err := Validate(data); err != nil {
		return Data{}, fmt.Errorf("配置复核不通过: %w", err)
	}
	return data, nil
}

// 供 configtool 测试复用的编解码小工具。
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
