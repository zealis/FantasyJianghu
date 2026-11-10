extends Node
## 加点真实链路探针（M2c 切片四）：登录 → 升级拿点 → 2/5 加点 → 11/1 校验 → 2/6 洗点 → 面板实拍。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/points_live.tscn（需服务端 1.7.0）
const USER := "加点探针"
const PWD := "123456"
const OUT := "D:/project/Fantasy-Jianghu/outputs/points_live.png"
const PanelScript := preload("res://scripts/ui/point_panel.gd")

var _fail := 0

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
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": "加点探针侠", "faction": "惊雷殿", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	WorldStore.apply_snapshot(snap)
	await _frames(6)

	# 洗掉历史分配，保证可重复（本轮免费）。
	await Network.send_and_wait(2, 6, {})
	await QuestStore.refresh()
	await _frames(3)

	# 新角色先交付 main_001 拿到升级点（need(1)=100，奖励 120 → Lv2，每级 +4）。
	if QuestStore.unspent_points < 4 and str(QuestStore.by_id("main_001").get("state", "")) == "available":
		print("[points_live] accept=", await QuestStore.accept("main_001"))
		await Network.send_and_wait(10, 1, {"map_id": "清河村", "npc_id": "hebo"})
		await _frames(3)
		print("[points_live] submit=", await QuestStore.submit("main_001"))
		await _frames(4)

	print("[points_live] level=", QuestStore.level, " unspent=", QuestStore.unspent_points,
		" points=", QuestStore.points, " stats=", QuestStore.stats)
	_check(QuestStore.level >= 2, "已在 2 级（升级曲线生效）")
	_check(QuestStore.unspent_points >= 4, "每级 +4 属性点已累加")
	_check(QuestStore.stats.has("patk"), "11/1 下发派生属性 stats")

	var base_patk := int(QuestStore.stats.get("patk", 0))
	var base_speed := int(QuestStore.stats.get("speed", 0))
	var alloc: Dictionary = await Network.send_and_wait(2, 5, {"str": 2, "spi": 0, "agi": 2, "con": 0})
	print("[points_live] allocate code=", allocate_code(alloc), " unspent=", alloc.get("unspent"), " points=", alloc.get("points"))
	_check(int(alloc.get("code", -1)) == 0, "2/5 加点成功")
	_check(int(alloc.get("unspent", -1)) == QuestStore.unspent_points - 4, "扣点正确（-4）")

	await QuestStore.refresh()
	await _frames(3)
	_check(int(QuestStore.points.get("str", 0)) == 2, "力量已分配 2")
	_check(int(QuestStore.points.get("agi", 0)) == 2, "敏捷已分配 2")
	_check(int(QuestStore.stats.get("patk", 0)) == base_patk + 4, "物攻 +4（力量 2×2）")
	_check(int(QuestStore.stats.get("speed", 0)) == base_speed + 2, "速度 +2（敏捷 2×1）")
	print("[points_live] after alloc patk=", QuestStore.stats.get("patk"),
		" speed=", QuestStore.stats.get("speed"), " points=", QuestStore.points)

	var reset: Dictionary = await Network.send_and_wait(2, 6, {})
	_check(int(reset.get("code", -1)) == 0, "2/6 洗点成功")
	await QuestStore.refresh()
	await _frames(3)
	_check(int(QuestStore.points.get("str", 0)) == 0 and int(QuestStore.points.get("agi", 0)) == 0,
		"洗点后四属性归零")
	_check(int(QuestStore.unspent_points) == 4, "洗点返还 4 点")
	print("[points_live] after reset unspent=", QuestStore.unspent_points, " points=", QuestStore.points)

	# 真实数据面板实拍（预加 2 力，展示预览增量）。
	var panel := Control.new()
	panel.set_script(PanelScript)
	main.get_node("UILayer").add_child(panel)
	await _frames(4)
	panel._add("str", 2)
	await _frames(3)
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img != null and not img.is_empty():
		_check(img.save_png(OUT) == OK, "真实链路加点面板实拍已导出")
	else:
		_check(false, "实拍截图为空")

	print("[points_live] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func allocate_code(d: Dictionary) -> int:
	return int(d.get("code", -1))

func _check(cond: bool, label: String) -> void:
	print("[points_live] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp := await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[points_live] 登录失败: ", resp)
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
