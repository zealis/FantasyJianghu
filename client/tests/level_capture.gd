extends Node
## 升级曲线真实链路探针：新账号 → main_001 交付 120 exp → 应升 2 级（need(1)=100）溢出 20 →
## main_005（需 2 级）解锁。验证 GDD §4.2 自动升级 + 溢出不丢 + 等级门禁。
## 运行：Godot_v4.7.2-stable_win64.exe --path client res://tests/level_capture.tscn（需服务端 1.6.0）
const USER := "等级探针"
const PWD := "123456"

func _ready() -> void:
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
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": "等级探针侠", "faction": "惊雷殿", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	WorldStore.apply_snapshot(snap)
	await _frames(6)
	print("[level] initial level=", QuestStore.level, " exp=", QuestStore.exp)
	if str(QuestStore.by_id("main_001").get("state", "")) == "available":
		print("[level] accept=", await QuestStore.accept("main_001"))
		await Network.send_and_wait(10, 1, {"map_id": "清河村", "npc_id": "hebo"})
		await _frames(4)
		print("[level] submit=", await QuestStore.submit("main_001"))
	await _frames(4)
	print("[level] after level=", QuestStore.level, " exp=", QuestStore.exp, " exp_next=", QuestStore.exp_next)
	if QuestStore.level != 2 or QuestStore.exp != 20:
		printerr("[level] FAIL 应升 2 级溢出 20: level=", QuestStore.level, " exp=", QuestStore.exp)
		get_tree().quit(1)
		return
	var m5: Dictionary = QuestStore.by_id("main_005")
	var m5_reason := str(m5.get("reason", ""))
	print("[level] main_005 state=", str(m5.get("state", "")), " reason=", m5_reason)
	# main_005 另有前置 main_004；此处只验证等级门禁已解除（reason 不再是"需 2 级"）。
	if m5_reason.contains("需 2 级"):
		printerr("[level] FAIL 2 级后 main_005 等级门禁未解除")
		get_tree().quit(1)
		return
	print("[level] PASS 自动升级/溢出保留/等级门禁解锁")
	get_tree().quit(0)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp: Dictionary = await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[level] 登录失败: ", resp)
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