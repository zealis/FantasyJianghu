extends Node
## 心法=门派身份 真实链路探针：为八派各建 1 级角色（裸装、无共鸣），
## 读 11/1 派生属性，核对心法被动已经生效（不依赖 5 件完美装备）。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/heart_live.tscn
const PWD := "123456"

## 门派 → 期望的心法收益（万分制，只列出该派心法涉及的项）。
const CASES := [
	{"faction": "断岳剑宗", "name": "剑一", "want": {"crit_dmg": 1000}},
	{"faction": "破军枪府", "name": "枪二", "want": {"pcombo": 500}},
	{"faction": "惊雷殿", "name": "雷三", "want": {}},
	{"faction": "璇玑星宫", "name": "星四", "want": {}},
	{"faction": "缚灵符观", "name": "符五", "want": {"sealhit": 800}},
	{"faction": "织梦谷", "name": "梦六", "want": {"sealhit": 500}},
	{"faction": "金刚禅院", "name": "禅七", "want": {"reflect_per_stack": 300, "reflect_max_stacks": 3}},
	{"faction": "玄水宫", "name": "水八", "want": {"heal_bonus": 800}},
]

var _fail := 0
var _main: Node

func _ready() -> void:
	_main = preload("res://scenes/main.tscn").instantiate()
	add_child(_main)
	await _frames(3)

	for c in CASES:
		await _probe(str(c["faction"]), str(c["name"]), c["want"])

	print("[heart_live] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _probe(faction: String, char_name: String, want: Dictionary) -> void:
	var sfx := str(int(Time.get_unix_time_from_system()) % 10000)
	# 角色名全局唯一：同一探针反复跑必须换名字，否则建角失败、后续 11/1 全空。
	char_name = char_name + _cn4(int(sfx))
	var user := "心法_" + faction + "_" + sfx
	var token := await _login(user)
	if token == "":
		_check(false, "%s 登录" % faction)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	var chars: Array = auth.get("chars", [])
	var char_id := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2,
			{"name": char_name, "faction": faction, "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	await QuestStore.refresh()
	await _frames(4)

	var stats: Dictionary = QuestStore.stats
	_check(not stats.is_empty(), "%s 11/1 下发派生属性" % faction)
	if int(QuestStore.level) != 1:
		_check(false, "%s 新角色应为 1 级，实际 %d" % [faction, int(QuestStore.level)])
	# 心法必须有可观测收益：至少一项与期望一致，且期望项全部命中。
	var hit := true
	for k in want:
		if int(stats.get(k, -1)) != int(want[k]):
			hit = false
			_check(false, "%s %s 期望 %d，实际 %s" % [faction, k, int(want[k]), str(stats.get(k))])
	if hit:
		var detail := []
		for k in want:
			detail.append("%s=%s" % [k, str(stats.get(k))])
		_check(true, "%s 心法生效（%s）" % [faction, ", ".join(detail) if detail.size() > 0 else "见属性"])
	# 该派心法的「主收益」项也不能为 0（防止只配对空期望）。
	var primary_map := {
		"断岳剑宗": "patk", "破军枪府": "patk", "惊雷殿": "matk", "璇玑星宫": "matk",
		"缚灵符观": "sealhit", "织梦谷": "sealhit", "金刚禅院": "hp", "玄水宫": "hp",
	}
	var primary: String = str(primary_map.get(faction, "hp"))
	_check(int(stats.get(primary, 0)) > 0, "%s 主属性 %s 有值" % [faction, primary])

func _check(cond: bool, label: String) -> void:
	print("[heart_live] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

## 数字 → 汉字（角色名规则：2–6 个汉字）。
func _cn4(n: int) -> String:
	var d := "零一二三四五六七八九"
	var s := ""
	for i in range(4):
		s += d.substr((n / int(pow(10, 3 - i))) % 10, 1)
	return s

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login(user: String) -> String:
	await _post("/api/register", {"username": user, "password": PWD, "nickname": user})
	var resp := await _post("/api/login", {"username": user, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[heart_live] 登录失败: ", resp)
		return ""
	return str(resp.get("data", {}).get("token", ""))

func _post(path: String, body: Dictionary) -> Dictionary:
	var req := HTTPRequest.new()
	add_child(req)
	var err := req.request("http://127.0.0.1:7788" + path, ["Content-Type: application/json"],
		HTTPClient.METHOD_POST, JSON.stringify(body))
	if err != OK:
		return {"code": -1}
	var res: Array = await req.request_completed
	req.queue_free()
	if int(res[0]) != HTTPRequest.RESULT_SUCCESS:
		return {"code": -1}
	return JSON.parse_string((res[3] as PackedByteArray).get_string_from_utf8())
