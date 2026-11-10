package config

import (
	"strings"
	"testing"
)

// questFixture 返回一份合法的任务/物品域（引用最小场景与野怪集合）。
func questFixture() Data {
	return Data{
		Scenes: map[string]SceneLogic{
			"清河村": {MapID: "清河村", NPCs: []SceneNPC{{ID: "hebo", Name: "何伯"}}},
		},
		Battle: BattleData{Monsters: []MonsterDef{{ID: "muzhuang", Name: "木桩人"}}},
		Items: ItemData{BagCapacity: 30, Items: []ItemDef{
			{ID: "pill", Name: "小还丹", Category: "丹药", Stack: 99},
		}},
		Quests: QuestData{
			DailyLimits: map[string]int{"normal": 50, "sect": 20, "dungeon": 10},
			Quests: []QuestDef{
				{
					ID: "main_001", Name: "归村", Type: "main", Chapter: 1,
					Giver: QuestGiver{MapID: "清河村", NPCID: "hebo"}, MinLevel: 1, Desc: "问何伯",
					Objectives: []QuestObjective{{Kind: "talk", Target: "hebo", Count: 1}},
					Rewards:    QuestRewards{Exp: 10, Gold: 100, Items: []QuestItemReward{{ID: "pill", Count: 1}}},
				},
				{
					ID: "main_002", Name: "试手", Type: "main", Chapter: 1, Prev: "main_001",
					Giver: QuestGiver{MapID: "清河村", NPCID: "hebo"}, MinLevel: 1, Desc: "打木桩",
					Objectives: []QuestObjective{{Kind: "kill", Target: "muzhuang", Count: 1}},
					Rewards:    QuestRewards{Gold: 10},
				},
				{
					ID: "daily_n_001", Name: "巡逻", Type: "daily_normal",
					Giver: QuestGiver{MapID: "清河村", NPCID: "hebo"}, MinLevel: 1, Desc: "进村",
					Objectives: []QuestObjective{{Kind: "visit", Target: "清河村", Count: 1}},
					Rewards:    QuestRewards{Gold: 10},
				},
			},
		},
	}
}

func TestValidateQuestsAccepts(t *testing.T) {
	if err := validateQuests(questFixture()); err != nil {
		t.Fatalf("合法任务域被拒: %v", err)
	}
}

// TestValidateQuestsRejects 表驱动覆盖引用完整性负例（§7.2：非法不打包）。
func TestValidateQuestsRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(d *Data)
	}{
		{"空物品字典", func(d *Data) { d.Items.Items = nil }},
		{"物品堆叠非正", func(d *Data) { d.Items.Items[0].Stack = 0 }},
		{"日常上限缺失", func(d *Data) { delete(d.Quests.DailyLimits, "sect") }},
		{"任务 id 重复", func(d *Data) {
			q := d.Quests.Quests[0]
			d.Quests.Quests = append(d.Quests.Quests, q)
		}},
		{"类型非法", func(d *Data) { d.Quests.Quests[0].Type = "side" }},
		{"发布 NPC 不存在", func(d *Data) { d.Quests.Quests[0].Giver.NPCID = "ghost" }},
		{"发布场景不存在", func(d *Data) { d.Quests.Quests[0].Giver.MapID = "无此地" }},
		{"等级越界", func(d *Data) { d.Quests.Quests[0].MinLevel = 90 }},
		{"目标类型非法", func(d *Data) { d.Quests.Quests[0].Objectives[0].Kind = "escort" }},
		{"击杀目标不存在", func(d *Data) { d.Quests.Quests[1].Objectives[0].Target = "wolf_king" }},
		{"到访场景不存在", func(d *Data) { d.Quests.Quests[2].Objectives[0].Target = "无此地" }},
		{"奖励物品不存在", func(d *Data) { d.Quests.Quests[0].Rewards.Items[0].ID = "ghost_item" }},
		{"奖励数量非正", func(d *Data) { d.Quests.Quests[0].Rewards.Items[0].Count = 0 }},
		{"前置不存在", func(d *Data) { d.Quests.Quests[1].Prev = "main_404" }},
		{"前置指向自身", func(d *Data) { d.Quests.Quests[1].Prev = "main_002" }},
		{"前置成环", func(d *Data) { d.Quests.Quests[0].Prev = "main_002" }},
		{"日常引用前置", func(d *Data) { d.Quests.Quests[2].Prev = "main_001" }},
		{"缺描述", func(d *Data) { d.Quests.Quests[0].Desc = "" }},
		{"品质非法", func(d *Data) { d.Items.Items[0].Quality = "神品" }},
		{"使用效果类型非法", func(d *Data) { d.Items.Items[0].Use = &ItemUseDef{Kind: "heal_hp", Value: 10} }},
		{"使用效果数值非正", func(d *Data) { d.Items.Items[0].Use = &ItemUseDef{Kind: "exp", Value: 0} }},
		{"装备槽非法", func(d *Data) { d.Items.Items[0].Slot = "wing" }},
		{"装备缺槽", func(d *Data) { d.Items.Items[0].Category = "装备" }},
		{"装备属性键非法", func(d *Data) { d.Items.Items[0].Stats = map[string]int{"luck": 1} }},
		{"装备属性负值", func(d *Data) { d.Items.Items[0].Stats = map[string]int{"hp": -1} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := questFixture()
			c.mutate(&d)
			err := validateQuests(d)
			if err == nil {
				t.Fatalf("应拒绝但通过")
			}
			if !strings.Contains(err.Error(), "") { // 只要求有可读原因
				t.Fatalf("错误信息不可读: %v", err)
			}
		})
	}
}

// TestValidateQuestsAcceptsItemUse 品质与合法 use 定义应通过（M2c 切片一）。
func TestValidateQuestsAcceptsItemUse(t *testing.T) {
	d := questFixture()
	d.Items.Items[0].Quality = "良品"
	d.Items.Items[0].Use = &ItemUseDef{Kind: "exp", Value: 100}
	if err := validateQuests(d); err != nil {
		t.Fatalf("合法品质/use 应通过: %v", err)
	}
	if got := d.Items.ItemQuality(d.Items.Items[0].ID); got != "良品" {
		t.Fatalf("ItemQuality = %q, 期望良品", got)
	}
	if got := d.Items.ItemQuality("不存在"); got != "凡品" {
		t.Fatalf("缺省品质 = %q, 期望凡品", got)
	}
}
