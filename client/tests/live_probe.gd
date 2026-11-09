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
	await _frames(4)

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
