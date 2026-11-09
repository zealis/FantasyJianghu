package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ItemDef 为物品字典最小集（M2b：任务奖励的引用目标；完整道具属性与装备 M2c）。
type ItemDef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Stack    int    `json:"stack"` // 单格堆叠上限（背包容量预检按格计算）
	Desc     string `json:"desc"`
}

// ItemData 为物品域配置（item/items.json）。
type ItemData struct {
	BagCapacity int       `json:"bag_capacity"`
	Items       []ItemDef `json:"items"`
}

// ByID 查物品定义；不存在返回 nil（调用方须先经校验）。
func (d ItemData) ByID(id string) *ItemDef {
	for i := range d.Items {
		if d.Items[i].ID == id {
			return &d.Items[i]
		}
	}
	return nil
}

// ItemStack 取单格堆叠上限；未知物品返回 1（配置校验已保证引用存在）。
func (d ItemData) ItemStack(id string) int {
	if it := d.ByID(id); it != nil && it.Stack > 0 {
		return it.Stack
	}
	return 1
}

// QuestGiver 为任务发布 NPC（场景 + NPC id）。
type QuestGiver struct {
	MapID string `json:"map_id"`
	NPCID string `json:"npc_id"`
}

// QuestObjective 为任务目标（kind: talk|kill|visit）。
type QuestObjective struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Count  int    `json:"count"`
}

// QuestItemReward 为物品奖励条目。
type QuestItemReward struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// QuestRewards 为任务奖励清单（§9.1：交付前按完整清单预检背包容量）。
type QuestRewards struct {
	Exp    int64             `json:"exp"`
	Xiuwei int64             `json:"xiuwei"`
	Gold   int64             `json:"gold"`
	Items  []QuestItemReward `json:"items"`
}

// QuestDef 为任务定义（GDD §9.1/§9.2）。
type QuestDef struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Type       string           `json:"type"` // main|daily_normal|daily_sect|daily_dungeon
	Chapter    int              `json:"chapter"`
	Giver      QuestGiver       `json:"giver"`
	MinLevel   int              `json:"min_level"`
	Prev       string           `json:"prev"` // 前置任务 id（空=无前置）
	Desc       string           `json:"desc"`
	Objectives []QuestObjective `json:"objectives"`
	Rewards    QuestRewards     `json:"rewards"`
}

// IsDaily 判断是否日常任务。
func (q QuestDef) IsDaily() bool { return q.Type != "main" }

// DailyCategory 返回日常计数分桶（normal|sect|dungeon）；非日常返回空串。
func (q QuestDef) DailyCategory() string {
	switch q.Type {
	case "daily_normal":
		return "normal"
	case "daily_sect":
		return "sect"
	case "daily_dungeon":
		return "dungeon"
	}
	return ""
}

// QuestData 为任务域配置（quest/quests.json）。
type QuestData struct {
	DailyLimits map[string]int `json:"daily_limits"` // normal|sect|dungeon → 每日次数上限（GDD §9.2）
	Quests      []QuestDef     `json:"quests"`
}

// ByID 查任务定义；不存在返回 nil。
func (q QuestData) ByID(id string) *QuestDef {
	for i := range q.Quests {
		if q.Quests[i].ID == id {
			return &q.Quests[i]
		}
	}
	return nil
}

// DailyLimit 取某类日常的每日上限；缺省 0。
func (q QuestData) DailyLimit(category string) int { return q.DailyLimits[category] }

// validateQuests 校验任务/物品域的结构与引用完整性（打包与加载同规则，§7.2）。
func validateQuests(d Data) error {
	// 物品字典：id 唯一、名称/类别非空、堆叠为正。
	if len(d.Items.Items) == 0 {
		return fmt.Errorf("物品字典不得为空")
	}
	if d.Items.BagCapacity <= 0 {
		return fmt.Errorf("bag_capacity 必须为正")
	}
	itemIDs := map[string]bool{}
	for _, it := range d.Items.Items {
		if it.ID == "" || itemIDs[it.ID] {
			return fmt.Errorf("物品 id 空/重复: %q", it.ID)
		}
		if it.Name == "" || it.Category == "" {
			return fmt.Errorf("物品 %s 缺名称/类别", it.ID)
		}
		if it.Stack <= 0 {
			return fmt.Errorf("物品 %s 堆叠上限必须为正", it.ID)
		}
		itemIDs[it.ID] = true
	}

	// 三类日常上限齐备且为正（GDD §9.2）。
	for _, cat := range []string{"normal", "sect", "dungeon"} {
		if d.Quests.DailyLimit(cat) <= 0 {
			return fmt.Errorf("daily_limits[%s] 必须为正", cat)
		}
	}
	if len(d.Quests.Quests) == 0 {
		return fmt.Errorf("任务表不得为空")
	}

	questIDs := map[string]bool{}
	for _, q := range d.Quests.Quests {
		if q.ID == "" || questIDs[q.ID] {
			return fmt.Errorf("任务 id 空/重复: %q", q.ID)
		}
		questIDs[q.ID] = true
	}
	validTypes := map[string]bool{"main": true, "daily_normal": true, "daily_sect": true, "daily_dungeon": true}
	validKinds := map[string]bool{"talk": true, "kill": true, "visit": true}
	monsters := map[string]bool{}
	for _, m := range d.Battle.Monsters {
		monsters[m.ID] = true
	}
	npcByMap := func(mapID, npcID string) bool {
		s, ok := d.Scenes[mapID]
		if !ok {
			return false
		}
		for _, n := range s.NPCs {
			if n.ID == npcID {
				return true
			}
		}
		return false
	}
	for _, q := range d.Quests.Quests {
		if q.Name == "" || q.Desc == "" {
			return fmt.Errorf("任务 %s 缺名称/描述", q.ID)
		}
		if !validTypes[q.Type] {
			return fmt.Errorf("任务 %s 类型非法: %q", q.ID, q.Type)
		}
		if q.MinLevel < 1 || q.MinLevel > 89 {
			return fmt.Errorf("任务 %s min_level 越界: %d", q.ID, q.MinLevel)
		}
		if !npcByMap(q.Giver.MapID, q.Giver.NPCID) {
			return fmt.Errorf("任务 %s 发布 NPC 不存在: %s/%s", q.ID, q.Giver.MapID, q.Giver.NPCID)
		}
		if len(q.Objectives) == 0 {
			return fmt.Errorf("任务 %s 缺目标", q.ID)
		}
		for _, o := range q.Objectives {
			if !validKinds[o.Kind] {
				return fmt.Errorf("任务 %s 目标类型非法: %q", q.ID, o.Kind)
			}
			if o.Count <= 0 {
				return fmt.Errorf("任务 %s 目标次数必须为正", q.ID)
			}
			switch o.Kind {
			case "kill":
				if !monsters[o.Target] {
					return fmt.Errorf("任务 %s 击杀目标不存在: %s", q.ID, o.Target)
				}
			case "visit":
				if _, ok := d.Scenes[o.Target]; !ok {
					return fmt.Errorf("任务 %s 到访场景不存在: %s", q.ID, o.Target)
				}
			case "talk":
				if !npcByMap(q.Giver.MapID, o.Target) {
					return fmt.Errorf("任务 %s 对话目标不在发布场景: %s", q.ID, o.Target)
				}
			}
		}
		// 奖励：非负、物品引用存在、数量为正。
		if q.Rewards.Exp < 0 || q.Rewards.Xiuwei < 0 || q.Rewards.Gold < 0 {
			return fmt.Errorf("任务 %s 奖励不得为负", q.ID)
		}
		for _, ir := range q.Rewards.Items {
			if !itemIDs[ir.ID] {
				return fmt.Errorf("任务 %s 奖励物品不存在: %s", q.ID, ir.ID)
			}
			if ir.Count <= 0 {
				return fmt.Errorf("任务 %s 奖励物品数量必须为正: %s", q.ID, ir.ID)
			}
		}
		// 前置：必须存在、非自身、同为主线、无环。
		if q.Prev != "" {
			if q.Prev == q.ID {
				return fmt.Errorf("任务 %s 前置指向自身", q.ID)
			}
			p := d.Quests.ByID(q.Prev)
			if p == nil {
				return fmt.Errorf("任务 %s 前置不存在: %s", q.ID, q.Prev)
			}
			if p.Type != "main" || q.Type != "main" {
				return fmt.Errorf("任务 %s 前置仅允许主线之间引用", q.ID)
			}
		}
	}
	// 环检测：沿 prev 链最多走 len(quests) 步。
	for _, q := range d.Quests.Quests {
		seen := map[string]bool{q.ID: true}
		cur := q.Prev
		for steps := 0; cur != "" && steps <= len(d.Quests.Quests); steps++ {
			if seen[cur] {
				return fmt.Errorf("任务前置成环: %s → %s", q.ID, cur)
			}
			seen[cur] = true
			p := d.Quests.ByID(cur)
			if p == nil {
				break
			}
			cur = p.Prev
		}
	}
	return nil
}

// loadQuestDir 读取任务域（quest/quests.json）。
func loadQuestDir(dir string) (QuestData, error) {
	var q QuestData
	raw, err := os.ReadFile(filepath.Join(dir, "quest", "quests.json"))
	if err != nil {
		return q, fmt.Errorf("读 quest/quests.json: %w", err)
	}
	if err := unmarshalStrict(raw, &q); err != nil {
		return q, fmt.Errorf("解析 quest/quests.json: %w", err)
	}
	return q, nil
}

// loadItemDir 读取物品域（item/items.json）。
func loadItemDir(dir string) (ItemData, error) {
	var it ItemData
	raw, err := os.ReadFile(filepath.Join(dir, "item", "items.json"))
	if err != nil {
		return it, fmt.Errorf("读 item/items.json: %w", err)
	}
	if err := unmarshalStrict(raw, &it); err != nil {
		return it, fmt.Errorf("解析 item/items.json: %w", err)
	}
	return it, nil
}

// packConfigFile 打包单个配置文件（清单带哈希）。
func packConfigFile(mf *Manifest, target, source, rel string) error {
	raw, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(rel)))
	if err != nil {
		return fmt.Errorf("读 %s: %w", rel, err)
	}
	return appendPacked(mf, target, rel, raw)
}
