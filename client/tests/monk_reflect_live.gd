extends Node
## 禅院反伤真实链路探针：新建金刚禅院角色（1 级、裸装）→ 4/1 进战斗 → 连续普攻 →
## 校验 4/1 下发金刚印参数、战报里出现 reflect 事件且 rate≥3%、层数递增封顶。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/monk_reflect_live.tscn
const USER_BASE := "反伤探针"
const PWD := "123456"
const OUT := "D:/project/Fantasy-Jianghu/outputs/monk_reflect_live.png"

var _fail := 0
var _user := ""
var _char_name := ""
var _events: Array = []
var _result := -2
var _turn_version := 0
var _snapshot: Dictionary = {}
var _captured := false

func _ready() -> void:
	# 每次跑用独立账号：角色不能重名，且反伤验证要从 1 级裸装开始。
	var sfx := int(Time.get_unix_time_from_system()) % 10000
	_user = USER_BASE + "_" + str(sfx)
	_char_name = "护法" + _cn4(sfx)
	var main := preload("res://scenes/main.tscn").instantiate()
	add_child(main)
	await _frames(3)
	var token := await _login()
	if token == "":
		get_tree().quit(1)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	var chars: Array = auth.get("chars", [])
	var char_id := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2,
			{"name": _char_name, "faction": "金刚禅院", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	WorldStore.apply_snapshot(snap)
	await _frames(4)

	# 监听战斗帧/终局（4/3、4/5）。
	Network.packet_received.connect(_on_packet)

	var enter: Dictionary = await Network.send_and_wait(4, Protocol.SUB_BATTLE_ENTER,
		{"monster_id": "muzhuang"})
	if int(enter.get("code", -1)) != 0:
		printerr("[monk_reflect] 进入战斗失败: ", enter)
		get_tree().quit(1)
		return
	_snapshot = enter
	# 直接调 4/1 不会经过场景触发器，手动让战斗层显示这一帧状态（仅为抓图）。
	var battle_layer := main.get_node_or_null("BattleCanvas/BattleLayer")
	if battle_layer != null and battle_layer.has_method("enter_battle"):
		battle_layer.enter_battle(enter)
	_check(int(enter.get("code", -1)) == 0, "4/1 进入战斗成功")

	var units: Array = enter.get("units", [])
	var self_unit: Dictionary = {}
	for u in units:
		if int(u.get("side", -1)) == 0:
			self_unit = u
			break
	_check(not self_unit.is_empty(), "4/1 下发我方单位")
	_check(int(self_unit.get("reflect_max_stacks", 0)) == 3, "4/1 下发金刚印上限 3 层")
	_check(int(self_unit.get("reflect_per_stack", -1)) == 300 if self_unit.has("reflect_per_stack") else true,
		"4/1 下发每层反伤（若字段存在应为 3%）")

	var monster_id := 0
	for u in units:
		if int(u.get("side", -1)) == 1:
			monster_id = int(u.get("id", 0))
			break
	_check(monster_id != 0, "4/1 下发敌方单位")

	# 连打若干回合：每 0.4s 提交一次普攻，直到出终局或次数用尽。
	var max_rounds := 30
	for i in range(max_rounds):
		if _result != -2:
			break
		await Network.send_and_wait(4, Protocol.SUB_BATTLE_CMD,
			{"cmd": "attack", "target_id": monster_id})
		for f in range(6):
			await get_tree().create_timer(0.12).timeout
		# 一出现反伤就抓图（战斗结束后会回世界，抓不到战斗画面）。
		if not _captured and _has_reflect():
			await RenderingServer.frame_post_draw
			var shot := get_viewport().get_texture().get_image()
			if shot != null and not shot.is_empty():
				_captured = shot.save_png(OUT) == OK

	var reflects: Array = []
	for e in _events:
		if str(e.get("type", "")) == "reflect":
			reflects.append(e)
	_check(not reflects.is_empty(), "战报出现反伤事件（1 级裸装也有）")
	if not reflects.is_empty():
		var first: Dictionary = reflects[0]
		var rate := int((first.get("flags", {}) as Dictionary).get("rate", 0))
		var stacks := int((first.get("flags", {}) as Dictionary).get("stacks", 0))
		var value := int(first.get("value", 0))
		print("[monk_reflect] 首次反伤 rate=", rate, " stacks=", stacks, " value=", value)
		_check(rate >= 300, "首次受击反伤率 ≥3%（金刚印 1 层）")
		_check(stacks == 1, "首次反伤时金刚印为 1 层")
		_check(value > 0, "反伤伤害大于 0")
		var max_stacks := 0
		for e in reflects:
			max_stacks = maxi(max_stacks, int((e.get("flags", {}) as Dictionary).get("stacks", 0)))
		_check(max_stacks <= 3, "金刚印层数不超过 3")
		print("[monk_reflect] 反伤事件 ", reflects.size(), " 次，最大层数 ", max_stacks)

	_check(_captured, "战斗实拍已导出（反伤发生时抓图）")

	print("[monk_reflect] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _has_reflect() -> bool:
	for e in _events:
		if str(e.get("type", "")) == "reflect":
			return true
	return false

func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	if main_code != 4:
		return
	match sub_code:
		Protocol.SUB_BATTLE_FRAME:
			_turn_version = maxi(_turn_version, int(payload.get("turn_version", 0)))
			_events.append_array(payload.get("events", []))
			Network.send(4, Protocol.SUB_BATTLE_ACK, {"turn_version": _turn_version})
		Protocol.SUB_BATTLE_RESULT:
			_result = int(payload.get("winner", -2))

func _check(cond: bool, label: String) -> void:
	print("[monk_reflect] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

## 时间后缀转 4 个汉字（角色名规则：2–6 个汉字）。
func _cn4(n: int) -> String:
	var d := "零一二三四五六七八九"
	var s := ""
	for i in range(4):
		s += d.substr((n / int(pow(10, 3 - i))) % 10, 1)
	return s

func _login() -> String:
	await _post("/api/register", {"username": _user, "password": PWD, "nickname": _user})
	var resp := await _post("/api/login", {"username": _user, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[monk_reflect] 登录失败: ", resp)
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
