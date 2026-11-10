extends Node
## 移动×帧率对照探针：离线/在线 × 60/30 帧，点按自动走 600px，采样位置与目标取消次数。
## 复现「开 30 帧后点击地面只走一小段」。运行：Godot_v4.7.2-stable_win64.exe --path client res://tests/move_fps_probe.tscn
const USER := "移动探针"
const PWD := "123456"

var _player: Node2D
var _cancels := 0
var _had_target := false
var _tracing := false
var _trace: Array = []

func _process(_delta: float) -> void:
	if _tracing and _player != null and _trace.size() < 240:
		var t: float = _player._target_x
		_trace.append([roundi(_player.position.x), -1 if is_nan(t) else roundi(t)])

func _ready() -> void:
	Network.packet_received.connect(_on_packet)
	_player = preload("res://scripts/game/player.gd").new()
	add_child(_player)
	await _frames(3)
	for fps in [60, 30]:
		if not await _case("offline", fps, false):
			get_tree().quit(1)
			return
	_player.queue_free()
	await _frames(2)
	var main := preload("res://scenes/main.tscn").instantiate()
	add_child(main)
	await _frames(3)
	var token := await _login()
	if token == "":
		get_tree().quit(1)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	var char_id := 0
	var chars: Array = auth.get("chars", [])
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": "移动探针侠", "faction": "惊雷殿", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	if char_id == 0:
		get_tree().quit(1)
		return
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	WorldStore.apply_snapshot(snap)
	await _frames(6)
	_player = get_tree().get_first_node_in_group("player") as Node2D
	for fps in [60, 30]:
		if not await _case("online", fps, true):
			get_tree().quit(1)
			return
	print("[move-probe] DONE")
	get_tree().quit(0)

func _case(mode: String, fps: int, online: bool, attempt: int = 0) -> bool:
	Settings.set_value("power_save", false)
	Settings.set_value("fps_cap", fps)
	await _frames(4)
	_cancels = 0
	_had_target = false
	_trace.clear()
	_tracing = online
	var start_x: float = _player.position.x
	var goal: float = start_x + 600.0
	if goal > WorldStore.walk_bounds.y - 20.0:
		goal = start_x - 600.0
	goal = clampf(goal, WorldStore.walk_bounds.x, WorldStore.walk_bounds.y)
	if not online:
		_player.position.x = 300.0
		start_x = 300.0
		goal = 900.0
	_player.walk_to(goal)
	var series: Array = []
	for i in range(20):
		await get_tree().create_timer(0.1).timeout
		# 物理键状态是系统级的：别的窗口（用户实机）按键会污染探针，检测到即重试。
		if Input.is_physical_key_pressed(KEY_A) or Input.is_physical_key_pressed(KEY_D) \
				or Input.is_physical_key_pressed(KEY_LEFT) or Input.is_physical_key_pressed(KEY_RIGHT):
			_tracing = false
			if attempt < 2:
				print("[move-probe] interference: 外部物理按键，重试 ", mode, " fps=", fps)
				return await _case(mode, fps, online, attempt + 1)
			printerr("[move-probe] FAIL 外部按键持续干扰，无法判定")
			return false
		series.append(roundi(_player.position.x))
	var reached: bool = absf(_player.position.x - goal) < 8.0
	print("[move-probe] ", mode, " fps=", fps, " start=", roundi(start_x), " goal=", roundi(goal),
		" final=", roundi(_player.position.x), " reached=", reached, " cancels=", _cancels,
		" physics_fps=", Engine.get_physics_frames(), " series=", series)
	if not reached:
		printerr("[move-probe] FAIL ", mode, " fps=", fps, " 未走满（点按/自动走被中断）")
		print("[move-probe] trace=", _trace)
	_tracing = false
	return reached

func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	if main_code != 3 or sub_code != 2 or _player == null:
		return
	if _tracing:
		print("[move-probe] 3/2 srv=", payload.get("x"), " cli=", roundi(_player.position.x),
			" last_reported=", _player._last_reported_x)
	var has := not is_nan(_player._target_x)
	if _had_target and not has:
		_cancels += 1
		print("[move-probe] target cancelled at x=", roundi(_player.position.x),
			" srv_x=", payload.get("x"))
	_had_target = has

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp: Dictionary = await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[move-probe] 登录失败: ", resp)
		return ""
	return str(resp.get("data", {}).get("token", ""))

func _post(path: String, body: Dictionary) -> Dictionary:
	var req := HTTPRequest.new()
	add_child(req)
	var err := req.request("http://127.0.0.1:7788" + path, ["Content-Type: application/json"], HTTPClient.METHOD_POST, JSON.stringify(body))
	if err != OK:
		return {"code": -1}
	var res: Array = await req.request_completed
	req.queue_free()
	if int(res[0]) != HTTPRequest.RESULT_SUCCESS:
		return {"code": -1}
	return JSON.parse_string((res[3] as PackedByteArray).get_string_from_utf8())