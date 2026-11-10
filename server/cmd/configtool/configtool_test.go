package main

import (
	"fantasy-jianghu/server/internal/config"
	"os"
	"path/filepath"
	"testing"
)

// writeSource 在临时目录生成一份合法（或经 mutate 破坏的）配置源。
func writeSource(t *testing.T, appJSON, sceneJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(appJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "qinghecun.json"), []byte(sceneJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factions.json"), []byte(validFactions()), 0o644); err != nil {
		t.Fatal(err)
	}
	// 任务域与物品字典（M2b 起为打包必填）；等级域（M2c 切片三起必填）。
	for _, d := range []string{"item", "quest", "level"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	items := `{"bag_capacity":20,"items":[{"id":"pill_xiaohuan","name":"小还丹","category":"丹药","stack":99,"desc":"测试丹药"}]}`
	quests := `{"daily_limits":{"normal":50,"sect":20,"dungeon":10},"quests":[
{"id":"main_001","name":"归村","type":"main","chapter":1,
 "giver":{"map_id":"清河村","npc_id":"hebo"},"min_level":1,"prev":"","desc":"测试主线",
 "objectives":[{"kind":"talk","target":"hebo","count":1}],
 "rewards":{"exp":10,"xiuwei":1,"gold":100,"items":[{"id":"pill_xiaohuan","count":1}]}}]}`
	if err := os.WriteFile(filepath.Join(dir, "item", "items.json"), []byte(items), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quest", "quests.json"), []byte(quests), 0o644); err != nil {
		t.Fatal(err)
	}
	levels := `{"max_level":89,"exp_base":100,"exp_exponent":1.5,"gates":[29,39,49,59,69,79],"points_per_level":4}`
	if err := os.WriteFile(filepath.Join(dir, "level", "levels.json"), []byte(levels), 0o644); err != nil {
		t.Fatal(err)
	}
	// V1.29 两张数值表：打包必填，复用仓库真实文件。
	for _, rel := range []string{"suit/suit.json", "sect_war/sect_war.json"} {
		real, err := filepath.Abs(filepath.Join("..", "..", "configs", "source", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(real)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 战斗域源（真实仓库文件）。
	battleRoot, err := filepath.Abs(filepath.Join("..", "..", "configs", "source", "battle"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "battle"), 0o755); err != nil {
		t.Fatal(err)
	}
	bentries, err := os.ReadDir(battleRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range bentries {
		raw, err := os.ReadFile(filepath.Join(battleRoot, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "battle", e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func validApp() string {
	return `{"heartbeat_seconds":15,"heartbeat_miss_limit":3,"max_characters_per_account":3,
"move_speed":320,"move_tolerance":16,
"spawn":{"map_id":"清河村","x":400},
"scene_capacity":{"town":50,"wild":30,"sect":30,"activity":30},
"factions":["断岳剑宗","破军枪府","惊雷殿","璇玑星宫","缚灵符观","织梦谷","金刚禅院","玄水宫"]}`
}

func validScene() string {
	return `{"map_id":"清河村","type":"town","width":2352,"height":1008,"ground_y":900,
"walk":{"x_min":80,"x_max":2272},
"npcs":[{"id":"hebo","name":"何伯","x":1560,"dialogue":"prologue_hebo"}]}`
}

// validFactions 为八派明细夹具（GDD §5.3）。
func validFactions() string {
	return `[
{"name":"断岳剑宗","category":"物理","color":"朱橙","residence":"高天原·断剑崖","weapon":"剑","feature":"暴击叠「剑意」，层数越高暴伤越高；专收残血","secret":"《断岳剑经》"},
{"name":"破军枪府","category":"物理","color":"朱橙","residence":"葬剑原·陷阵营","weapon":"枪","feature":"连击与破甲，击杀刷新连势，越战越勇","secret":"《破军枪谱》"},
{"name":"惊雷殿","category":"法术","color":"紫","residence":"北天庭·雷泽","weapon":"杖","feature":"雷法群攻，命中叠「感电」，连锁跳跃与概率麻痹","secret":"《惊雷宝典》"},
{"name":"璇玑星宫","category":"法术","color":"紫","residence":"高天原·星宫遗殿","weapon":"杖","feature":"星辉蓄力型法术，蓄力越久伤害越高；与主线星核同源","secret":"《璇玑星经》"},
{"name":"缚灵符观","category":"封印","color":"青","residence":"坠魔谷口·镇魔观","weapon":"爪","feature":"符箓硬控：封行动、封技能；镇魔出身","secret":"《缚灵符箓》"},
{"name":"织梦谷","category":"封印","color":"青","residence":"临渊镇外·迷雾谷","weapon":"爪","feature":"幻术软控：混乱、沉睡、降命中","secret":"《大梦心经》"},
{"name":"金刚禅院","category":"肉盾","color":"绿","residence":"望月城·临海古刹","weapon":"杖","feature":"护盾、反伤与嘲讽聚火，越挨打越硬","secret":"《金刚不坏体》"},
{"name":"玄水宫","category":"肉盾","color":"绿","residence":"蓬莱岛·水府","weapon":"杖","feature":"减伤光环与持续治疗，能扛能奶的续航坦克","secret":"《玄水真经》"}
]`
}

func TestPackValid(t *testing.T) {
	src := writeSource(t, validApp(), validScene())
	out := t.TempDir()
	if err := config.Pack(src, out, "1.0.0"); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	mfPath := filepath.Join(out, "1.0.0", "manifest.json")
	if _, err := os.Stat(mfPath); err != nil {
		t.Fatalf("manifest 未生成: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "1.0.0", "app.json")); err != nil {
		t.Fatalf("app.json 未复制: %v", err)
	}
}

func TestPackRejectsInvalidScene(t *testing.T) {
	bad := `{"map_id":"清河村","width":2352,"height":1008,"ground_y":900,
"walk":{"x_min":3000,"x_max":2272},"npcs":[]}`
	src := writeSource(t, validApp(), bad)
	if err := config.Pack(src, t.TempDir(), "1.0.0"); err == nil {
		t.Fatal("walk.x_min > x_max 的场景应被拒绝打包")
	}
}

func TestPackRejectsInvalidApp(t *testing.T) {
	bad := `{"heartbeat_seconds":15,"heartbeat_miss_limit":3,
"max_characters_per_account":3,"move_speed":320,"move_tolerance":16,
"spawn":{"map_id":"清河村","x":400},"factions":[]}`
	src := writeSource(t, bad, validScene())
	if err := config.Pack(src, t.TempDir(), "1.0.0"); err == nil {
		t.Fatal("空 factions 的 app.json 应被拒绝打包")
	}
}
