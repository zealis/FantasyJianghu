extends Node
## 实机链路探针：真实 HTTP 登录 → WS 鉴权 → 选角 → 进场 → 进战斗 → 截图。
## 用来复现「战斗层只渲染左上角一小块」这类只出现在真实窗口+真实数据下的布局问题。
const USER := "布局探针"
const PWD := "123456"
const OUT := "D:/project/Fantasy-Jianghu/client/tests/live_battle.png"

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
	print("[live] auth code=", auth.get("code"), " chars=", chars.size())
	var char_id := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": "布局探针侠", "faction": "惊雷殿", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
		print("[live] create code=", cr.get("code"), " id=", char_id)
	else:
		char_id = int(chars[0].get("id", 0))
	var sel: Dictionary = await Network.send_and_wait(2, 3, {"char_id": char_id})
	print("[live] select code=", sel.get("code"))
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	print("[live] enter scene code=", snap.get("code"))
	WorldStore.apply_snapshot(snap)
	# 二维坐标系验收：快照 self/bounds/spawn + 一次真实 3/2 移动回包必须带 y。
	print("[live] 2D self=(", WorldStore.self_x, ",", WorldStore.self_y,
		") bounds_y=", WorldStore.bounds_y, " spawn=", WorldStore.spawn)
	var mv: Dictionary = await Network.send_and_wait(3, 2,
		{"x": WorldStore.self_x + 40, "y": WorldStore.self_y - 30})
	print("[live] 2D move resp=", mv)
	WorldStore.apply_snapshot(snap)
	await _frames(8)
	# 主世界实拍：确认主流程切换后登录→进场显示的是 2.5D 俯视整图 + 既有 HUD/任务层。
	await RenderingServer.frame_post_draw
	var world_img := get_viewport().get_texture().get_image()
	world_img.save_png("D:/project/Fantasy-Jianghu/client/tests/live_world.png")
	print("[live] saved live_world.png ", world_img.get_size())

	# 门洞热区交互：主流程下触发应打开门内对话（DialoguePanel 按 dialogue key 渲染）。
	# 只验 UI，不发 3/9（服务端权威传送已由 Go 端到端用例覆盖，避免改动探针角色存档）。
	var dlg := main.get_node("DialogueLayer/DialoguePanel")
	WorldStore.request_hotspot("door_dazhai")
	await _frames(3)
	print("[live] door dialogue visible=", dlg.visible)
	if not dlg.visible:
		printerr("[live] 门洞热区未打开门内对话")
	dlg.visible = false

	var battle: Control = main.get_node("BattleCanvas/BattleLayer")
	print("[live] A vp=", get_viewport().get_visible_rect().size, " battle=", battle.size, " win=", get_window().size)
	main.enter_battle({"monster_id": "muzhuang"})
	await _frames(20)
	print("[live] B battle.visible=", battle.visible, " battle=", battle.size,
		" bg=", (battle._bg.size if battle._bg else "nil"),
		" stage_units=", battle._units.size() if "_units" in battle else "?")
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	img.save_png(OUT)
	print("[live] saved ", OUT, " ", img.get_size())

	# 战斗中主世界必须停用（否则玩家能「看不见地走路」、明雷 4s 冷却会重复开战）。
	var wnode := main.get_node("World")
	print("[live] battle lock=", main.get("_in_battle"), " world_mode=", wnode.process_mode,
		"（应 true / ", Node.PROCESS_MODE_DISABLED, "）")
	if not bool(main.get("_in_battle")) or wnode.process_mode != Node.PROCESS_MODE_DISABLED:
		printerr("[live] 战斗中主世界未停用或重入锁未生效")
	# 重入：再次请求进战斗应被锁挡住，不新开一场（否则表现为残血回满）。
	var first_battle_id := str(battle.get("_battle_id"))
	main.call("enter_battle", {"monster_id": "muzhuang"})
	await _frames(3)
	if str(battle.get("_battle_id")) != first_battle_id:
		printerr("[live] 战斗重入锁失效：重复 4/1 又开了一场战斗")

	# 3/9 传送点：走 world 层的联网分支（_in_main=true / Network.offline=false），
	# 验证客户端协议常量、服务端权威落点与 on_teleported 不被 3/2 纠偏拉回。
	var world_node := main.get_node("World")
	var player_node := world_node.get_node_or_null("YSortLayer/Player")
	WorldStore.request_hotspot("field_gate")
	await _frames(12)
	print("[live] 3/9 teleport player=", player_node.position if player_node else "nil",
		" (应 1500,700)")
	if player_node != null and (player_node as Node2D).position.distance_to(Vector2(1500, 700)) > 2.0:
		printerr("[live] 传送点未落到服务端权威坐标")
	get_tree().quit(0)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login() -> String:
	var reg := await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp := await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[live] 登录失败: ", resp)
		return ""
	return str(resp.get("data", {}).get("token", ""))

func _post(path: String, body: Dictionary) -> Dictionary:
	var req := HTTPRequest.new()
	add_child(req)
	var err := req.request("http://127.0.0.1:7788" + path, ["Content-Type: application/json"], HTTPClient.METHOD_POST, JSON.stringify(body))
	if err != OK:
		return {"code": -1, "msg": "HTTP 请求失败"}
	var res: Array = await req.request_completed
	req.queue_free()
	if int(res[0]) != HTTPRequest.RESULT_SUCCESS:
		return {"code": -1, "msg": "HTTP 无响应"}
	return JSON.parse_string((res[3] as PackedByteArray).get_string_from_utf8())
