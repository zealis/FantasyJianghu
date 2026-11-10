package config

import (
	"os"
	"path/filepath"
	"testing"
)

// packValidSource 打包一份合法源，返回配置包目录。
func packValidSource(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	app := `{"heartbeat_seconds":15,"heartbeat_miss_limit":3,"max_characters_per_account":3,
"move_speed":320,"move_tolerance":16,
"spawn":{"map_id":"清河村","x":400},
"scene_capacity":{"town":50,"wild":30,"sect":30,"activity":30},
"factions":["断岳剑宗","破军枪府","惊雷殿","璇玑星宫","缚灵符观","织梦谷","金刚禅院","玄水宫"]}`
	scene := `{"map_id":"清河村","type":"town","width":2352,"height":1008,"ground_y":900,
"walk":{"x_min":80,"x_max":2272},
"npcs":[{"id":"hebo","name":"何伯","x":1560,"dialogue":"prologue_hebo"}]}`
	if err := os.WriteFile(filepath.Join(src, "app.json"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "scenes", "qinghecun.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "factions.json"), []byte(minimalValidData().marshalFactions()), 0o644); err != nil {
		t.Fatal(err)
	}
	// M2b：任务域与物品字典为必填项（§7.2 配置齐全才允许打包）。
	if err := os.MkdirAll(filepath.Join(src, "item"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "quest"), 0o755); err != nil {
		t.Fatal(err)
	}
	items := `{"bag_capacity":20,"items":[{"id":"pill_xiaohuan","name":"小还丹","category":"丹药","stack":99,"desc":"测试丹药"}]}`
	quests := `{"daily_limits":{"normal":50,"sect":20,"dungeon":10},"quests":[
{"id":"main_001","name":"归村","type":"main","chapter":1,
 "giver":{"map_id":"清河村","npc_id":"hebo"},"min_level":1,"prev":"","desc":"测试主线",
 "objectives":[{"kind":"talk","target":"hebo","count":1}],
 "rewards":{"exp":10,"xiuwei":1,"gold":100,"items":[{"id":"pill_xiaohuan","count":1}]}}]}`
	if err := os.WriteFile(filepath.Join(src, "item", "items.json"), []byte(items), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "quest", "quests.json"), []byte(quests), 0o644); err != nil {
		t.Fatal(err)
	}
	// M2c 切片三：等级域为必填项。
	if err := os.MkdirAll(filepath.Join(src, "level"), 0o755); err != nil {
		t.Fatal(err)
	}
	levels := `{"max_level":89,"exp_base":100,"exp_exponent":1.5,"gates":[29,39,49,59,69,79],"points_per_level":4}`
	if err := os.WriteFile(filepath.Join(src, "level", "levels.json"), []byte(levels), 0o644); err != nil {
		t.Fatal(err)
	}
	copyBattleSrc(t, src)
	out := t.TempDir()
	if err := Pack(src, out, "1.0.0"); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return filepath.Join(out, "1.0.0")
}

// copyBattleSrc 把仓库内战斗配置源拷入临时源目录。
func copyBattleSrc(t *testing.T, src string) {
	t.Helper()
	battleRoot, err := filepath.Abs(filepath.Join("..", "..", "configs", "source", "battle"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(battleRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "battle"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(battleRoot, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "battle", e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadValid(t *testing.T) {
	dir := packValidSource(t)
	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Version != "1.0.0" {
		t.Fatalf("Version = %q", data.Version)
	}
	if data.App.MoveSpeed != 320 || len(data.App.Factions) != 8 {
		t.Fatalf("App 解析不一致: %+v", data.App)
	}
	s, ok := data.Scenes["清河村"]
	if !ok || s.GroundY != 900 || s.Walk.XMin != 80 || s.Walk.XMax != 2272 {
		t.Fatalf("场景解析不一致: %+v", s)
	}
	if len(s.NPCs) != 1 || s.NPCs[0].ID != "hebo" {
		t.Fatalf("NPC 解析不一致: %+v", s.NPCs)
	}
}

func TestLoadRejectsTamperedFile(t *testing.T) {
	dir := packValidSource(t)
	p := filepath.Join(dir, "app.json")
	b, _ := os.ReadFile(p)
	b[len(b)/2] = 'X' // 中段字节改动：保证哈希必然失配
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("哈希失配的配置包应拒绝加载")
	}
}

func TestLoadRejectsBadStructure(t *testing.T) {
	dir := packValidSource(t)
	// 重写 manifest 哈希使其匹配，但 app.json 结构非法（空 factions）。
	p := filepath.Join(dir, "app.json")
	if err := os.WriteFile(p, []byte(`{"heartbeat_seconds":15,"factions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshManifest(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("结构非法（空 factions）应拒绝加载，不得回退默认值")
	}
}

// refreshManifest 重算配置包内 manifest 哈希（模拟"结构非法但哈希自洽"）。
func refreshManifest(dir string) error {
	mfPath := filepath.Join(dir, "manifest.json")
	var mf Manifest
	b, err := os.ReadFile(mfPath)
	if err != nil {
		return err
	}
	if err := jsonUnmarshal(b, &mf); err != nil {
		return err
	}
	for i := range mf.Files {
		data, err := os.ReadFile(filepath.Join(dir, mf.Files[i].Name))
		if err != nil {
			return err
		}
		mf.Files[i].SHA256 = sha256Hex(data)
		mf.Files[i].Size = len(data)
	}
	out, err2 := jsonMarshal(mf)
	if err2 != nil {
		return err2
	}
	return os.WriteFile(mfPath, out, 0o644)
}

func TestSceneTypeAndCapacity(t *testing.T) {
	dir := packValidSource(t)
	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Scenes["清河村"].Type != "town" {
		t.Fatalf("scene type = %q, 期望 town", data.Scenes["清河村"].Type)
	}
	if data.LineCapacity("town") != 50 || data.LineCapacity("wild") != 30 {
		t.Fatalf("LineCapacity town/wild = %d/%d, 期望 50/30", data.LineCapacity("town"), data.LineCapacity("wild"))
	}
	if data.LineCapacity("unknown") != 0 {
		t.Fatal("未知类型容量应为 0")
	}
}

func TestValidateRejectsBadSceneTypeAndCapacity(t *testing.T) {
	// 非法场景类型
	d := minimalValidData()
	d.Scenes["清河村"] = SceneLogic{MapID: "清河村", Type: "castle", Width: 100, Height: 100, Walk: WalkRange{XMin: 0, XMax: 100}}
	if err := Validate(d); err == nil {
		t.Fatal("非法场景类型应被拒绝")
	}
	// 缺 capacity 键
	d2 := minimalValidData()
	delete(d2.App.SceneCapacity, "sect")
	if err := Validate(d2); err == nil {
		t.Fatal("缺 capacity 键应被拒绝")
	}
	// 容量非正
	d3 := minimalValidData()
	d3.App.SceneCapacity["town"] = 0
	if err := Validate(d3); err == nil {
		t.Fatal("容量 0 应被拒绝")
	}
}

func TestFactionsDetail(t *testing.T) {
	dir := packValidSource(t)
	data, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(data.Factions) != 8 {
		t.Fatalf("factions = %d 项, 期望 8", len(data.Factions))
	}
	f0 := data.Factions[0]
	if f0.Name != "断岳剑宗" || f0.Category != "物理" || f0.Color != "朱橙" || f0.Weapon != "剑" {
		t.Fatalf("门派明细不一致: %+v", f0)
	}
	if f0.Secret == "" || f0.Feature == "" || f0.Residence == "" {
		t.Fatalf("门派明细缺字段: %+v", f0)
	}
}

func TestValidateRejectsBadFactionDetail(t *testing.T) {
	d := minimalValidData()
	d.Factions[3].Category = "治疗" // 非法类别
	if err := Validate(d); err == nil {
		t.Fatal("非法系别应被拒绝")
	}
	d2 := minimalValidData()
	d2.Factions = d2.Factions[:7]
	if err := Validate(d2); err == nil {
		t.Fatal("八派缺一应被拒绝")
	}
}

// minimalValidData 构造一份可直接过 Validate 的最小内存配置。
func minimalValidData() Data {
	return Data{
		App: AppSettings{
			HeartbeatSeconds: 15, HeartbeatMissLimit: 3, MaxCharactersPerAccount: 3,
			MoveSpeed: 320, MoveTolerance: 16,
			Spawn:         Spawn{MapID: "清河村", X: 400},
			Factions:      []string{"断岳剑宗", "破军枪府", "惊雷殿", "璇玑星宫", "缚灵符观", "织梦谷", "金刚禅院", "玄水宫"},
			SceneCapacity: map[string]int{"town": 50, "wild": 30, "sect": 30, "activity": 30},
		},
		Scenes: map[string]SceneLogic{
			"清河村": {MapID: "清河村", Type: "town", Width: 2352, Height: 1008, GroundY: 900, Walk: WalkRange{XMin: 80, XMax: 2272}},
		},
		Levels: LevelData{MaxLevel: 89, ExpBase: 100, ExpExponent: 1.5,
			Gates: []int{29, 39, 49, 59, 69, 79}, PointsPerLevel: 4},
		Factions: []FactionInfo{
			{Name: "断岳剑宗", Category: "物理", Color: "朱橙", Residence: "高天原·断剑崖", Weapon: "剑", Feature: "暴击叠「剑意」，层数越高暴伤越高；专收残血", Secret: "《断岳剑经》"},
			{Name: "破军枪府", Category: "物理", Color: "朱橙", Residence: "葬剑原·陷阵营", Weapon: "枪", Feature: "连击与破甲，击杀刷新连势，越战越勇", Secret: "《破军枪谱》"},
			{Name: "惊雷殿", Category: "法术", Color: "紫", Residence: "北天庭·雷泽", Weapon: "杖", Feature: "雷法群攻，命中叠「感电」，连锁跳跃与概率麻痹", Secret: "《惊雷宝典》"},
			{Name: "璇玑星宫", Category: "法术", Color: "紫", Residence: "高天原·星宫遗殿", Weapon: "杖", Feature: "星辉蓄力型法术，蓄力越久伤害越高", Secret: "《璇玑星经》"},
			{Name: "缚灵符观", Category: "封印", Color: "青", Residence: "坠魔谷口·镇魔观", Weapon: "爪", Feature: "符箓硬控：封行动、封技能", Secret: "《缚灵符箓》"},
			{Name: "织梦谷", Category: "封印", Color: "青", Residence: "临渊镇外·迷雾谷", Weapon: "爪", Feature: "幻术软控：混乱、沉睡、降命中", Secret: "《大梦心经》"},
			{Name: "金刚禅院", Category: "肉盾", Color: "绿", Residence: "望月城·临海古刹", Weapon: "杖", Feature: "护盾、反伤与嘲讽聚火，越挨打越硬", Secret: "《金刚不坏体》"},
			{Name: "玄水宫", Category: "肉盾", Color: "绿", Residence: "蓬莱岛·水府", Weapon: "杖", Feature: "减伤光环与持续治疗，能扛能奶的续航坦克", Secret: "《玄水真经》"},
		},
	}
}

// marshalFactions 序列化门派明细（测试夹具用）。
func (d Data) marshalFactions() []byte {
	b, _ := jsonMarshal(d.Factions)
	return b
}
