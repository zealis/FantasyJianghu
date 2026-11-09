package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"fantasy-jianghu/server/internal/config"
	"fantasy-jianghu/server/internal/protocol"
)

// packFor 打包配置（别名便于测试书写）。
func packFor(src, out, ver string) error { return config.Pack(src, out, ver) }

// packTestConfig 在临时目录打包合法配置，返回配置包目录。
func packTestConfig(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	app := `{"heartbeat_seconds":15,"heartbeat_miss_limit":3,"max_characters_per_account":3,
"move_speed":320,"move_tolerance":16,"spawn":{"map_id":"清河村","x":400},
"scene_capacity":{"town":50,"wild":30,"sect":30,"activity":30},
"factions":["断岳剑宗","破军枪府","惊雷殿","璇玑星宫","缚灵符观","织梦谷","金刚禅院","玄水宫"]}`
	scene := `{"map_id":"清河村","type":"town","width":2352,"height":1008,"ground_y":900,
"walk":{"x_min":80,"x_max":2272},"npcs":[{"id":"hebo","name":"何伯","x":1560,"dialogue":"d1"}]}`
	if err := os.WriteFile(filepath.Join(src, "app.json"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "scenes", "qinghecun.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "factions.json"), []byte(validFactionsJSON()), 0o644); err != nil {
		t.Fatal(err)
	}
	// 任务/物品域（M2b 起为打包必填；引用本夹具场景 NPC 与真实野怪 id）。
	if err := os.MkdirAll(filepath.Join(src, "item"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "quest"), 0o755); err != nil {
		t.Fatal(err)
	}
	items := `{"bag_capacity":10,"items":[
{"id":"pill_xiaohuan","name":"小还丹","category":"丹药","stack":5,"desc":"测试丹药"}]}`
	quests := `{"daily_limits":{"normal":50,"sect":20,"dungeon":10},"quests":[
{"id":"main_001","name":"归村","type":"main","chapter":1,
 "giver":{"map_id":"清河村","npc_id":"hebo"},"min_level":1,"prev":"","desc":"问何伯",
 "objectives":[{"kind":"talk","target":"hebo","count":1}],
 "rewards":{"exp":120,"xiuwei":20,"gold":500,"items":[{"id":"pill_xiaohuan","count":3}]}},
{"id":"main_002","name":"试手","type":"main","chapter":1,
 "giver":{"map_id":"清河村","npc_id":"hebo"},"min_level":1,"prev":"main_001","desc":"打木桩",
 "objectives":[{"kind":"kill","target":"muzhuang","count":1}],
 "rewards":{"exp":300,"xiuwei":50,"gold":1200,"items":[{"id":"pill_xiaohuan","count":2}]}},
{"id":"daily_n_001","name":"巡逻","type":"daily_normal","chapter":0,
 "giver":{"map_id":"清河村","npc_id":"hebo"},"min_level":1,"prev":"","desc":"进村一圈",
 "objectives":[{"kind":"visit","target":"清河村","count":1}],
 "rewards":{"exp":10,"gold":20,"items":[]}}]}`
	if err := os.WriteFile(filepath.Join(src, "item", "items.json"), []byte(items), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "quest", "quests.json"), []byte(quests), 0o644); err != nil {
		t.Fatal(err)
	}
	// 战斗域源（真实仓库文件）。
	battleRoot, err := filepath.Abs(filepath.Join("..", "..", "configs", "source", "battle"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "battle"), 0o755); err != nil {
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
		if err := os.WriteFile(filepath.Join(src, "battle", e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	if err := packFor(src, out, "9.9.9"); err != nil {
		t.Fatalf("打包配置: %v", err)
	}
	return filepath.Join(out, "9.9.9")
}

// startServer 起完整装配（随机端口 + 内存库），返回 HTTP 基址与句柄。
func startServer(t *testing.T) (string, *ServerHandle) {
	t.Helper()
	cfgDir := packTestConfig(t)
	opts := Options{Addr: "127.0.0.1:0", DSN: ":memory:", ConfigDir: cfgDir}
	h, err := buildServer(opts)
	if err != nil {
		t.Fatalf("装配: %v", err)
	}
	ts := httptest.NewServer(h.Handler)
	t.Cleanup(func() { ts.Close(); h.Stop() })
	return ts.URL, h
}

func apiPost(t *testing.T, base, path, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s 响应解码: %v (%s)", path, err, raw)
	}
	return out
}

func dialWS(t *testing.T, base string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(base, "http") + "/ws"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("拨号: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func send(t *testing.T, c *websocket.Conn, main, sub uint16, seq uint32, payload map[string]any) {
	t.Helper()
	b, err := protocol.EncodeFrame(protocol.Frame{Main: main, Sub: sub, Seq: seq, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("发帧: %v", err)
	}
}

func recv(t *testing.T, c *websocket.Conn, timeout time.Duration) protocol.Frame {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("读帧: %v", err)
	}
	f, err := protocol.DecodeFrame(raw)
	if err != nil {
		t.Fatalf("解码: %v", err)
	}
	return f
}

// asID 把 JSON 解码后的数字（恒为 float64）转为 int64。
func asID(v any) int64 { return int64(v.(float64)) }

func expectCode(t *testing.T, f protocol.Frame, seq uint32, wantCode int, stage string) {
	t.Helper()
	if f.Seq != seq {
		t.Fatalf("%s: 回包 seq=%d, 期望 %d（整帧 %+v）", stage, f.Seq, seq, f)
	}
	cf, ok := f.Payload["code"].(float64)
	if !ok {
		t.Fatalf("%s: 回包负载无 code 字段（整帧 %+v）", stage, f)
	}
	if int(cf) != wantCode {
		t.Fatalf("%s: code=%d (%v), 期望 %d", stage, int(cf), f.Payload["msg"], wantCode)
	}
}

func TestFullChain(t *testing.T) {
	base, h := startServer(t)

	// 1. 注册 + 登录（HTTP）。
	reg := apiPost(t, base, "/api/register", `{"username":"联调侠","password":"123456"}`)
	if reg["code"] != float64(0) {
		t.Fatalf("注册失败: %+v", reg)
	}
	login := apiPost(t, base, "/api/login", `{"username":"联调侠","password":"123456"}`)
	if login["code"] != float64(0) {
		t.Fatalf("登录失败: %+v", login)
	}
	token := login["data"].(map[string]any)["token"].(string)

	// 2. WS 鉴权。
	ws := dialWS(t, base)
	send(t, ws, 2, 1, 1, map[string]any{"token": token})
	f := recv(t, ws, 3*time.Second)
	expectCode(t, f, 1, 0, "鉴权")

	// 3. 创建角色 + 选择。
	send(t, ws, 2, 2, 2, map[string]any{"name": "联调之侠", "faction": "惊雷殿", "gender": "女"})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, 2, 0, "创角")
	charID := asID(f.Payload["char"].(map[string]any)["id"])

	send(t, ws, 2, 3, 3, map[string]any{"char_id": float64(charID)})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, 3, 0, "选角")

	// 4. 进场快照。
	send(t, ws, 3, 1, 4, map[string]any{"map_id": "清河村"})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, 4, 0, "进场")
	if f.Payload["ground_y"] != 900.0 {
		t.Fatalf("进场快照 ground_y=%v", f.Payload["ground_y"])
	}

	// 5. 第二连接进场 → 收到 3/4 加入推送；第一连接收到对方加入。
	_ = apiPost(t, base, "/api/register", `{"username":"旁观侠","password":"123456"}`)
	login2 := apiPost(t, base, "/api/login", `{"username":"旁观侠","password":"123456"}`)
	token2 := login2["data"].(map[string]any)["token"].(string)
	ws2 := dialWS(t, base)
	send(t, ws2, 2, 1, 1, map[string]any{"token": token2})
	expectCode(t, recv(t, ws2, 3*time.Second), 1, 0, "鉴权2")
	send(t, ws2, 2, 2, 2, map[string]any{"name": "旁观之侠", "faction": "玄水宫", "gender": "男"})
	f2 := recv(t, ws2, 3*time.Second)
	expectCode(t, f2, 2, 0, "创角2")
	charID2 := asID(f2.Payload["char"].(map[string]any)["id"])
	send(t, ws2, 2, 3, 3, map[string]any{"char_id": float64(charID2)})
	expectCode(t, recv(t, ws2, 3*time.Second), 3, 0, "选角2")
	send(t, ws2, 3, 1, 4, map[string]any{"map_id": "清河村"})
	expectCode(t, recv(t, ws2, 3*time.Second), 4, 0, "进场2")
	// 甲收到乙的 3/4 加入推送。
	join := recv(t, ws, 3*time.Second)
	if join.Main != 3 || join.Sub != 6 {
		t.Fatalf("甲应收到 3/4 加入推送, 收到 %d/%d", join.Main, join.Sub)
	}

	// 6. 甲移动 → 权威回包 + 乙收到 3/3 广播。
	time.Sleep(60 * time.Millisecond) // elapsed 供限速：allowed = 320*0.06+16 ≈ 35
	send(t, ws, 3, 2, 5, map[string]any{"x": 425.0, "y": 900.0})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, 5, 0, "移动")
	ackX := f.Payload["x"].(float64)
	if ackX < 400 || ackX > 435 {
		t.Fatalf("移动回包 x=%v 超出限速范围", ackX)
	}
	mv := recv(t, ws2, 3*time.Second)
	if mv.Main != 3 || mv.Sub != 3 || asID(mv.Payload["id"]) != charID {
		t.Fatalf("乙应收到 3/3 广播: %+v", mv)
	}

	// 7. 甲离场 → 乙收 3/5；位置已落库（x=ackX）。
	send(t, ws, 3, 4, 6, map[string]any{})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, 6, 0, "离场")
	lv := recv(t, ws2, 3*time.Second)
	if lv.Main != 3 || lv.Sub != 5 {
		t.Fatalf("乙应收到 3/5 离场推送: %+v", lv)
	}
	// 直接查库验证落库。
	c, err := h.DB.Characters.GetCharacter(context.Background(), charID)
	if err != nil {
		t.Fatalf("读角色: %v", err)
	}
	if c.Gender != "女" {
		t.Fatalf("创角 gender 未落库: %q", c.Gender)
	}
	if c.PosX != ackX || c.MapID != "清河村" {
		t.Fatalf("离场位置落库不符: x=%v map=%s, 期望 x=%v 清河村", c.PosX, c.MapID, ackX)
	}
}

func TestConfigRejectOnStart(t *testing.T) {
	// 配置非法 → 装配失败（不回退默认值）。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":"0","files":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildServer(Options{Addr: "127.0.0.1:0", DSN: ":memory:", ConfigDir: dir}); err == nil {
		t.Fatal("配置包缺 app.json 应装配失败")
	}
}

// validFactionsJSON 为八派明细夹具（GDD §5.3）。
func validFactionsJSON() string {
	return `[
{"name":"断岳剑宗","category":"物理","color":"朱橙","residence":"高天原·断剑崖","weapon":"剑","feature":"暴击叠「剑意」，层数越高暴伤越高；专收残血","secret":"《断岳剑经》"},
{"name":"破军枪府","category":"物理","color":"朱橙","residence":"葬剑原·陷阵营","weapon":"枪","feature":"连击与破甲，击杀刷新连势，越战越勇","secret":"《破军枪谱》"},
{"name":"惊雷殿","category":"法术","color":"紫","residence":"北天庭·雷泽","weapon":"杖","feature":"雷法群攻，命中叠「感电」，连锁跳跃与概率麻痹","secret":"《惊雷宝典》"},
{"name":"璇玑星宫","category":"法术","color":"紫","residence":"高天原·星宫遗殿","weapon":"杖","feature":"星辉蓄力型法术，蓄力越久伤害越高","secret":"《璇玑星经》"},
{"name":"缚灵符观","category":"封印","color":"青","residence":"坠魔谷口·镇魔观","weapon":"爪","feature":"符箓硬控：封行动、封技能","secret":"《缚灵符箓》"},
{"name":"织梦谷","category":"封印","color":"青","residence":"临渊镇外·迷雾谷","weapon":"爪","feature":"幻术软控：混乱、沉睡、降命中","secret":"《大梦心经》"},
{"name":"金刚禅院","category":"肉盾","color":"绿","residence":"望月城·临海古刹","weapon":"杖","feature":"护盾、反伤与嘲讽聚火，越挨打越硬","secret":"《金刚不坏体》"},
{"name":"玄水宫","category":"肉盾","color":"绿","residence":"蓬莱岛·水府","weapon":"杖","feature":"减伤光环与持续治疗，能扛能奶的续航坦克","secret":"《玄水真经》"}
]`
}

func TestLineIntegration(t *testing.T) {
	base, h := startServer(t)

	// 三连接：甲/乙/丙（创角带性别，进场）。
	mkConn := func(user, charName, gender string) (*websocket.Conn, int64) {
		_ = apiPost(t, base, "/api/register", `{"username":"`+user+`","password":"123456","nickname":"`+user+`昵"}`)
		login := apiPost(t, base, "/api/login", `{"username":"`+user+`","password":"123456"}`)
		token := login["data"].(map[string]any)["token"].(string)
		ws := dialWS(t, base)
		send(t, ws, 2, 1, 1, map[string]any{"token": token})
		expectCode(t, recv(t, ws, 3*time.Second), 1, 0, user+" 鉴权")
		send(t, ws, 2, 2, 2, map[string]any{"name": charName, "faction": "惊雷殿", "gender": gender})
		f := recv(t, ws, 3*time.Second)
		expectCode(t, f, 2, 0, user+" 创角")
		charID := asID(f.Payload["char"].(map[string]any)["id"])
		send(t, ws, 2, 3, 3, map[string]any{"char_id": float64(charID)})
		expectCode(t, recv(t, ws, 3*time.Second), 3, 0, user+" 选角")
		send(t, ws, 3, 1, 4, map[string]any{"map_id": "清河村"})
		f = recv(t, ws, 3*time.Second)
		expectCode(t, f, 4, 0, user+" 进场")
		return ws, charID
	}
	wsA, charA := mkConn("集成甲", "集成甲侠", "男")
	wsB, _ := mkConn("集成乙", "集成乙侠", "女")
	wsC, _ := mkConn("集成丙", "集成丙侠", "男")

	// 三人都在 1 线（容量 50 未满）。
	// 甲收到乙、丙的加入推送；丙收到甲、乙的快照（players 含甲乙）。
	send(t, wsC, 3, 7, 7, map[string]any{"map_id": "清河村"})
	f := recv(t, wsC, 3*time.Second)
	expectCode(t, f, 7, 0, "分线列表")
	lines := f.Payload["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %d 条, 期望 1（容量 50 未满）", len(lines))
	}
	l1 := lines[0].(map[string]any)
	if asID(l1["count"]) != 3 {
		t.Fatalf("线 1 count = %v, 期望 3", l1["count"])
	}

	// 丙切线到 2（3/8 切向未开线 → 服务端拒 code 3，切线只允许既有线）。
	send(t, wsC, 3, 8, 8, map[string]any{"line_no": 2.0})
	f = recv(t, wsC, 3*time.Second)
	if cf := int(f.Payload["code"].(float64)); cf != 3 {
		t.Fatalf("切向不存在线 code=%d, 期望 3", cf)
	}

	// 排空各连接积压的 3/6 加入推送（甲收到乙/丙加入；乙收到丙加入）。
	drainJoin := func(c *websocket.Conn, n int) {
		for i := 0; i < n; i++ {
			f := recv(t, c, 2*time.Second)
			if f.Main != 3 || f.Sub != 6 {
				t.Fatalf("应收到 3/6 加入推送, 收到 %d/%d", f.Main, f.Sub)
			}
		}
	}
	drainJoin(wsA, 2)
	drainJoin(wsB, 1)

	// 同线广播仍通：甲移动 → 丙收到 3/3。
	time.Sleep(60 * time.Millisecond)
	send(t, wsA, 3, 2, 9, map[string]any{"x": 430.0, "y": 900.0})
	expectCode(t, recv(t, wsA, 3*time.Second), 9, 0, "甲移动")
	mv := recv(t, wsC, 3*time.Second)
	if mv.Main != 3 || mv.Sub != 3 || asID(mv.Payload["id"]) != charA {
		t.Fatalf("丙应收到甲的 3/3: %+v", mv)
	}
	// 乙也应收到。
	mvB := recv(t, wsB, 3*time.Second)
	if mvB.Main != 3 || mvB.Sub != 3 || asID(mvB.Payload["id"]) != charA {
		t.Fatalf("乙应收到甲的 3/3: %+v", mvB)
	}
	_ = h
}

// TestBattleChainOverWS 用**真实生产装配**（真实 SetFrameHook / gateway.SendTo）
// 跑完整战斗链路：注册→鉴权→创角→选角→4/1→4/2→4/3 帧→4/4 确认→终局 4/5。
// 旧 frame hook 在 OnCmd 持锁时反查 Room.Participants() 会自死锁，本用例会超时失败。
func TestBattleChainOverWS(t *testing.T) {
	base, _ := startServer(t)

	_ = apiPost(t, base, "/api/register", `{"username":"战斗验收侠","password":"123456"}`)
	login := apiPost(t, base, "/api/login", `{"username":"战斗验收侠","password":"123456"}`)
	token := login["data"].(map[string]any)["token"].(string)
	ws := dialWS(t, base)

	var seq uint32 = 1
	send(t, ws, 2, 1, seq, map[string]any{"token": token})
	expectCode(t, recv(t, ws, 3*time.Second), seq, 0, "战斗验收鉴权")
	seq++
	send(t, ws, 2, 2, seq, map[string]any{"name": "战斗验收之侠", "faction": "惊雷殿", "gender": "男"})
	f := recv(t, ws, 3*time.Second)
	expectCode(t, f, seq, 0, "战斗验收创角")
	charID := asID(f.Payload["char"].(map[string]any)["id"])
	seq++
	send(t, ws, 2, 3, seq, map[string]any{"char_id": float64(charID)})
	expectCode(t, recv(t, ws, 3*time.Second), seq, 0, "战斗验收选角")

	seq++
	send(t, ws, 4, 1, seq, map[string]any{"monster_id": "muzhuang"})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, seq, 0, "进战斗")
	if f.Payload["battle_id"] == nil || f.Payload["self"] == nil || f.Payload["enemy"] == nil {
		t.Fatalf("4/1 快照不完整: %+v", f.Payload)
	}
	if _, ok := f.Payload["skills"].([]any); !ok {
		t.Fatalf("4/1 缺 skills 字段: %+v", f.Payload["skills"])
	}
	// 注：新角色 Lv1，惊雷殿最低主动秘籍开放等级为 10，故此列表预期为空；
	// 「有可用秘籍时正确下发」由 battle 包 Lv20 单测覆盖。

	finished := false
	sawResultFrame := false
	for round := 0; round < 60 && !finished; round++ {
		seq++
		cmdSeq := seq
		send(t, ws, 4, 2, cmdSeq, map[string]any{"cmd": "attack", "skill_id": ""})
		sawFrame, sawCmdReply := false, false
		// 帧顺序：hook 推 4/3（终局时再推 4/5）→ handler 返回后回 4/2 包。
		// 因此只要收齐 4/3 + 4/2 回包，4/5 必然也已到达。
		for i := 0; i < 12 && !(sawFrame && sawCmdReply); i++ {
			rf := recv(t, ws, 3*time.Second)
			switch {
			case rf.Main == 4 && rf.Sub == 3:
				sawFrame = true
				tv := int(rf.Payload["turn_version"].(float64))
				seq++
				send(t, ws, 4, 4, seq, map[string]any{"turn_version": float64(tv)})
				for _, raw := range rf.Payload["events"].([]any) {
					if e, ok := raw.(map[string]any); ok && e["type"] == "end" {
						finished = true
					}
				}
			case rf.Main == 4 && rf.Sub == 5:
				sawResultFrame = true
				finished = true
			case rf.Main == 4 && rf.Sub == 2 && rf.Seq == cmdSeq:
				sawCmdReply = true
			}
		}
		if !sawFrame {
			t.Fatalf("第 %d 回合未收到 4/3 帧（生产 frame hook 死锁或帧流中断）", round+1)
		}
	}
	if !finished {
		t.Fatal("60 回合内未收到终局")
	}
	if !sawResultFrame {
		t.Fatal("终局未收到 4/5 结果帧（协议子码已登记但装配未发送）")
	}
}

// TestDrill4v4ChainOverWS 真实生产装配 + WS 的 4v4 演练全链路：
// 8 单位快照 / 目标选择 / order 行动顺序事件 / 反伤事件 / 终局 4/5。
func TestDrill4v4ChainOverWS(t *testing.T) {
	base, _ := startServer(t)
	_ = apiPost(t, base, "/api/register", `{"username":"演练侠","password":"123456"}`)
	login := apiPost(t, base, "/api/login", `{"username":"演练侠","password":"123456"}`)
	token := login["data"].(map[string]any)["token"].(string)
	ws := dialWS(t, base)

	var seq uint32 = 1
	send(t, ws, 2, 1, seq, map[string]any{"token": token})
	expectCode(t, recv(t, ws, 3*time.Second), seq, 0, "演练鉴权")
	seq++
	send(t, ws, 2, 2, seq, map[string]any{"name": "演练之侠", "faction": "惊雷殿", "gender": "男"})
	f := recv(t, ws, 3*time.Second)
	expectCode(t, f, seq, 0, "演练创角")
	charID := asID(f.Payload["char"].(map[string]any)["id"])
	seq++
	send(t, ws, 2, 3, seq, map[string]any{"char_id": float64(charID)})
	expectCode(t, recv(t, ws, 3*time.Second), seq, 0, "演练选角")

	seq++
	send(t, ws, 4, 1, seq, map[string]any{"mode": "drill"})
	f = recv(t, ws, 3*time.Second)
	expectCode(t, f, seq, 0, "4v4 进战斗")
	if f.Payload["mode"] != "drill" {
		t.Fatalf("回包缺 mode=drill: %+v", f.Payload)
	}
	units, ok := f.Payload["units"].([]any)
	if !ok || len(units) != 8 {
		t.Fatalf("4v4 快照应含 8 个单位, 实得 %d", len(units))
	}
	var thornID float64
	for _, raw := range units {
		u := raw.(map[string]any)
		if _, has := u["slot"]; !has {
			t.Fatalf("单位快照缺 slot: %+v", u)
		}
		if u["name"] == "荆棘武士" {
			thornID = u["id"].(float64)
			if int(u["reflect_pct"].(float64)) != 900 {
				t.Fatalf("荆棘武士反伤应为 900, 实得 %v", u["reflect_pct"])
			}
		}
	}
	if thornID == 0 {
		t.Fatal("演练阵容缺荆棘武士")
	}

	finished, sawOrder, sawReflect := false, false, false
	types := map[string]int{}
	for round := 0; round < 60 && !finished; round++ {
		seq++
		cmdSeq := seq
		send(t, ws, 4, 2, cmdSeq, map[string]any{"cmd": "attack", "skill_id": "", "target_id": thornID})
		sawFrame, sawCmdReply := false, false
		for i := 0; i < 12 && !(sawFrame && sawCmdReply); i++ {
			rf := recv(t, ws, 3*time.Second)
			switch {
			case rf.Main == 4 && rf.Sub == 3:
				sawFrame = true
				tv := int(rf.Payload["turn_version"].(float64))
				seq++
				send(t, ws, 4, 4, seq, map[string]any{"turn_version": float64(tv)})
				for _, raw := range rf.Payload["events"].([]any) {
					e, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					switch e["type"] {
					case "order":
						sawOrder = true
					case "reflect":
						sawReflect = true
					case "end":
						finished = true
					}
					types[e["type"].(string)]++
				}
			case rf.Main == 4 && rf.Sub == 5:
				finished = true
			case rf.Main == 4 && rf.Sub == 2 && rf.Seq == cmdSeq:
				sawCmdReply = true
			}
		}
		if !sawFrame {
			t.Fatalf("第 %d 回合未收到 4/3 帧", round+1)
		}
	}
	if !finished {
		t.Fatal("4v4 演练 60 回合内未终局")
	}
	if !sawOrder {
		t.Fatal("4v4 帧流缺 order 行动顺序事件")
	}
	if !sawReflect {
		t.Fatalf("攻击荆棘武士应触发 reflect 反伤事件；事件类型统计=%v", types)
	}
}

// waitReply 丢弃推送帧，直到收到指定主/子码的回包（任务进度 11/4 会与回包交错）。
func waitReply(t *testing.T, c *websocket.Conn, main, sub uint16, timeout time.Duration) protocol.Frame {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remain := time.Until(deadline)
		if remain <= 0 {
			t.Fatalf("等待 %d/%d 回包超时", main, sub)
		}
		f := recv(t, c, remain)
		if f.Main == main && f.Sub == sub {
			return f
		}
	}
}

// frameKey 为主/子码二元组（收集多帧用）。
type frameKey struct{ main, sub uint16 }

// collectFrames 收集指定主/子码的帧直到齐备（推送与回包顺序不定）。
func collectFrames(t *testing.T, c *websocket.Conn, wants []frameKey, timeout time.Duration) map[frameKey]protocol.Frame {
	t.Helper()
	got := map[frameKey]protocol.Frame{}
	deadline := time.Now().Add(timeout)
	for len(got) < len(wants) {
		remain := time.Until(deadline)
		if remain <= 0 {
			t.Fatalf("收集帧超时，期望 %v，已收 %d 个", wants, len(got))
		}
		f := recv(t, c, remain)
		for _, w := range wants {
			if f.Main == w.main && f.Sub == w.sub {
				got[w] = f
			}
		}
	}
	return got
}

// questByID 从 11/1 列表里取一条任务视图。
func questByID(t *testing.T, list map[string]any, id string) map[string]any {
	t.Helper()
	arr, ok := list["quests"].([]any)
	if !ok {
		t.Fatalf("11/1 缺 quests 数组: %+v", list)
	}
	for _, raw := range arr {
		q, ok := raw.(map[string]any)
		if ok && q["id"] == id {
			return q
		}
	}
	t.Fatalf("任务列表无 %s", id)
	return nil
}

// TestQuestChainOverWS 真实装配 + WS 的任务全链路：
// 接取 → 进场/对话推进 → 交付发奖（钱包/经验/背包）→ 战斗胜利推进击杀 → 日常计数。
func TestQuestChainOverWS(t *testing.T) {
	base, _ := startServer(t)
	_ = apiPost(t, base, "/api/register", `{"username":"任务验收侠","password":"123456"}`)
	login := apiPost(t, base, "/api/login", `{"username":"任务验收侠","password":"123456"}`)
	token := login["data"].(map[string]any)["token"].(string)
	ws := dialWS(t, base)

	var seq uint32 = 1
	send(t, ws, 2, 1, seq, map[string]any{"token": token})
	expectCode(t, waitReply(t, ws, 2, 1, 3*time.Second), seq, 0, "任务鉴权")
	seq++
	send(t, ws, 2, 2, seq, map[string]any{"name": "任务验收之侠", "faction": "惊雷殿", "gender": "男"})
	f := waitReply(t, ws, 2, 2, 3*time.Second)
	expectCode(t, f, seq, 0, "任务创角")
	charID := asID(f.Payload["char"].(map[string]any)["id"])
	seq++
	send(t, ws, 2, 3, seq, map[string]any{"char_id": float64(charID)})
	expectCode(t, waitReply(t, ws, 2, 3, 3*time.Second), seq, 0, "任务选角")

	// 11/1 初始状态：主线可接、日常可接。
	seq++
	send(t, ws, 11, 1, seq, map[string]any{})
	list := waitReply(t, ws, 11, 1, 3*time.Second)
	expectCode(t, list, seq, 0, "任务列表")
	if q := questByID(t, list.Payload, "main_001"); q["state"] != "available" {
		t.Fatalf("main_001 初始状态 = %v, 期望 available", q["state"])
	}
	if q := questByID(t, list.Payload, "main_002"); q["state"] != "locked" {
		t.Fatalf("main_002 应因前置未完成而 locked，实际 %v", q["state"])
	}

	// 接取主线 + 日常。
	seq++
	send(t, ws, 11, 2, seq, map[string]any{"quest_id": "main_001"})
	expectCode(t, waitReply(t, ws, 11, 2, 3*time.Second), seq, 0, "接取 main_001")
	seq++
	send(t, ws, 11, 2, seq, map[string]any{"quest_id": "daily_n_001"})
	expectCode(t, waitReply(t, ws, 11, 2, 3*time.Second), seq, 0, "接取日常")

	// 进场：visit 目标自动推进（3/1 后应有 11/4 推送）。
	seq++
	send(t, ws, 3, 1, seq, map[string]any{"map_id": "清河村"})
	// 推送（11/4）先于回包（3/1）到达，故两者一并收集。
	got := collectFrames(t, ws, []frameKey{{3, 1}, {11, 4}}, 3*time.Second)
	expectCode(t, got[frameKey{3, 1}], seq, 0, "进场")
	push := got[frameKey{11, 4}]
	if push.Payload["quest_id"] != "daily_n_001" || push.Payload["state"] != "completable" {
		t.Fatalf("进场推送不符: %+v", push.Payload)
	}

	// NPC 对话：talk 目标推进。
	seq++
	send(t, ws, 10, 1, seq, map[string]any{"map_id": "清河村", "npc_id": "hebo"})
	got = collectFrames(t, ws, []frameKey{{10, 1}, {11, 4}}, 3*time.Second)
	expectCode(t, got[frameKey{10, 1}], seq, 0, "对话")
	push = got[frameKey{11, 4}]
	if push.Payload["quest_id"] != "main_001" || push.Payload["state"] != "completable" {
		t.Fatalf("对话推送不符: %+v", push.Payload)
	}

	// 交付主线：奖励入账。
	seq++
	send(t, ws, 11, 3, seq, map[string]any{"quest_id": "main_001"})
	sub := waitReply(t, ws, 11, 3, 3*time.Second)
	expectCode(t, sub, seq, 0, "交付 main_001")
	rewards := sub.Payload["rewards"].(map[string]any)
	if asID(rewards["gold"]) != 500 || asID(rewards["exp"]) != 120 {
		t.Fatalf("主线奖励不符: %+v", rewards)
	}

	// 5/1 背包与钱包。
	seq++
	send(t, ws, 5, 1, seq, map[string]any{})
	bag := waitReply(t, ws, 5, 1, 3*time.Second)
	expectCode(t, bag, seq, 0, "背包列表")
	if asID(bag.Payload["used"]) != 1 || asID(bag.Payload["capacity"]) != 10 {
		t.Fatalf("背包占用不符: used=%v capacity=%v", bag.Payload["used"], bag.Payload["capacity"])
	}
	items := bag.Payload["items"].([]any)
	it := items[0].(map[string]any)
	if it["id"] != "pill_xiaohuan" || asID(it["count"]) != 3 {
		t.Fatalf("奖励物品未到账: %+v", it)
	}
	wallet := bag.Payload["wallet"].(map[string]any)
	if asID(wallet["gold"]) != 500 || asID(wallet["xiuwei"]) != 20 {
		t.Fatalf("钱包不符: %+v", wallet)
	}

	// 交付日常：计数 +1。
	seq++
	send(t, ws, 11, 3, seq, map[string]any{"quest_id": "daily_n_001"})
	expectCode(t, waitReply(t, ws, 11, 3, 3*time.Second), seq, 0, "交付日常")
	seq++
	send(t, ws, 11, 5, seq, map[string]any{})
	daily := waitReply(t, ws, 11, 5, 3*time.Second)
	expectCode(t, daily, seq, 0, "日常状态")
	normal := daily.Payload["normal"].(map[string]any)
	if asID(normal["used"]) != 1 || asID(normal["limit"]) != 50 {
		t.Fatalf("普通日常计数不符: %+v", normal)
	}

	// 接取击杀任务 → 打一场木桩 → 终局推进 kill → 交付。
	seq++
	send(t, ws, 11, 2, seq, map[string]any{"quest_id": "main_002"})
	expectCode(t, waitReply(t, ws, 11, 2, 3*time.Second), seq, 0, "接取 main_002")

	seq++
	send(t, ws, 4, 1, seq, map[string]any{"monster_id": "muzhuang"})
	expectCode(t, waitReply(t, ws, 4, 1, 3*time.Second), seq, 0, "进战斗")
	finished := false
	for round := 0; round < 60 && !finished; round++ {
		seq++
		cmdSeq := seq
		send(t, ws, 4, 2, cmdSeq, map[string]any{"cmd": "attack", "skill_id": ""})
		sawFrame, sawReply := false, false
		for i := 0; i < 14 && !(sawFrame && sawReply); i++ {
			rf := recv(t, ws, 3*time.Second)
			switch {
			case rf.Main == 4 && rf.Sub == 3:
				sawFrame = true
				tv := int(rf.Payload["turn_version"].(float64))
				seq++
				send(t, ws, 4, 4, seq, map[string]any{"turn_version": float64(tv)})
				for _, raw := range rf.Payload["events"].([]any) {
					if e, ok := raw.(map[string]any); ok && e["type"] == "end" {
						finished = true
					}
				}
			case rf.Main == 4 && rf.Sub == 5:
				finished = true
			case rf.Main == 4 && rf.Sub == 2 && rf.Seq == cmdSeq:
				sawReply = true
			}
		}
		if !sawFrame {
			t.Fatalf("第 %d 回合未收到 4/3 帧", round+1)
		}
	}
	if !finished {
		t.Fatal("战斗未在 60 回合内结束")
	}
	// 终局后 kill 进度应达可交付。
	seq++
	send(t, ws, 11, 1, seq, map[string]any{})
	list = waitReply(t, ws, 11, 1, 3*time.Second)
	if q := questByID(t, list.Payload, "main_002"); q["state"] != "completable" {
		t.Fatalf("击杀后 main_002 状态 = %v, 期望 completable", q["state"])
	}
	seq++
	send(t, ws, 11, 3, seq, map[string]any{"quest_id": "main_002"})
	sub = waitReply(t, ws, 11, 3, 3*time.Second)
	expectCode(t, sub, seq, 0, "交付 main_002")
	// 背包累计 3+2=5 个丹药（单格上限 5 → 仍 1 格）。
	seq++
	send(t, ws, 5, 1, seq, map[string]any{})
	bag = waitReply(t, ws, 5, 1, 3*time.Second)
	items = bag.Payload["items"].([]any)
	it = items[0].(map[string]any)
	if asID(it["count"]) != 5 {
		t.Fatalf("第二次交付后丹药 = %v, 期望 5", it["count"])
	}
	wallet = bag.Payload["wallet"].(map[string]any)
	if asID(wallet["gold"]) != 1720 { // 500 主线 + 20 日常 + 1200 主线
		t.Fatalf("金币 = %v, 期望 1720", wallet["gold"])
	}
}
