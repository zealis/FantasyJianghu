extends Node
## 感电施加口径真实链路探针（秘籍表 V1.2 §4 注）：
## 惊雷殿（持《惊雷宝典》）法术命中叠感电；其他门派（这里取璇玑星宫）法术命中不得叠层。
## 用 4v4 演练场（等级 20）以便双方都有可释放的法术秘籍。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/shock_scope_live.tscn
const PWD := "123456"

var _fail := 0
var _events: Array = []
var _result := -2
var _turn_version := 0

func _ready() -> void:
	var main := preload("res://scenes/main.tscn").instantiate()
	add_child(main)
	await _frames(3)
	Network.packet_received.connect(_on_packet)

	for faction in ["惊雷殿", "璇玑星宫"]:
		await _probe(faction)

	print("[shock_scope] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _probe(faction: String) -> void:
	_events.clear()
	_result = -2
	_turn_version = 0
	var sfx := int(Time.get_unix_time_from_system()) % 10000
	var cn := _cn4(sfx)
	var user := "感电_" + faction + "_" + str(sfx)
	var token := await _login(user)
	if token == "":
		_check(false, "%s 登录" % faction)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	var chars: Array = auth.get("chars", [])
	var char_id := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2,
			{"name": cn, "faction": faction, "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	await _frames(4)

	var enter: Dictionary = await Network.send_and_wait(4, Protocol.SUB_BATTLE_ENTER, {"mode": "drill"})
	if int(enter.get("code", -1)) != 0:
		_check(false, "%s 进入演练场: %s" % [faction, str(enter.get("msg", ""))])
		return
	# 选一本可用的攻击秘籍（单体/群体、攻击类型为法术）。
	var skill_id := ""
	for sk in enter.get("skills", []):
		var cat := str(sk.get("category", ""))
		if cat == "单体" or cat == "群体":
			skill_id = str(sk.get("id", ""))
			break
	_check(skill_id != "", "%s 演练场下发可用攻击秘籍" % faction)

	var enemy_id := 0
	for u in enter.get("units", []):
		if int(u.get("side", -1)) == 1:
			enemy_id = int(u.get("id", 0))
			break
	_check(enemy_id != 0, "%s 下发敌方单位" % faction)

	for i in range(8):
		if _result != -2:
			break
		if skill_id != "":
			await Network.send_and_wait(4, Protocol.SUB_BATTLE_CMD,
				{"cmd": "skill", "skill_id": skill_id, "target_id": enemy_id})
		else:
			await Network.send_and_wait(4, Protocol.SUB_BATTLE_CMD,
				{"cmd": "attack", "target_id": enemy_id})
		for f in range(6):
			await get_tree().create_timer(0.12).timeout

	var shocks := 0
	var max_stacks := 0
	for e in _events:
		if str(e.get("type", "")) == "shock":
			shocks += 1
			max_stacks = maxi(max_stacks, int((e.get("flags", {}) as Dictionary).get("stacks", 0)))
	var want := faction == "惊雷殿"
	print("[shock_scope] ", faction, " shock 事件=", shocks, " 最大层数=", max_stacks)
	if want:
		_check(shocks > 0, "惊雷殿（持惊雷宝典）法术命中叠感电")
		_check(max_stacks >= 1 and max_stacks <= 3, "感电层数在 1..3")
	else:
		_check(shocks == 0, "非雷殿（%s）法术命中不施加感电" % faction)

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
	print("[shock_scope] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _cn4(n: int) -> String:
	var d := "零一二三四五六七八九"
	var s := ""
	for i in range(4):
		s += d.substr((n / int(pow(10, 3 - i))) % 10, 1)
	return s

func _login(user: String) -> String:
	await _post("/api/register", {"username": user, "password": PWD, "nickname": user})
	var resp := await _post("/api/login", {"username": user, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[shock_scope] 登录失败: ", resp)
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
