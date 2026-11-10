extends Node
## 同屏实机验证：真实 HTTP 登录 + WS 鉴权 + 进清河村，等机器人走到附近后截图。
## 依赖 remote_bot 同时在跑。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/remote_live_capture.tscn
const USER := "remote_viewer"
const PWD := "123456"
const CHAR_NAME := "同屏观察者"
const FACTION := "惊雷殿"
const GENDER := "女"
const OUT := "D:/project/Fantasy-Jianghu/outputs/remote_live.png"

func _ready() -> void:
	var main := preload("res://scenes/main.tscn").instantiate()
	add_child(main)
	await get_tree().process_frame

	var token := await _login()
	if token == "":
		get_tree().quit(1)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	var chars: Array = auth.get("chars", [])
	var cid := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2,
			{"name": CHAR_NAME, "faction": FACTION, "gender": GENDER})
		cid = int(cr.get("char", {}).get("id", 0))
		print("[live] create=", cr.get("code"), " id=", cid)
	else:
		cid = int(chars[0].get("id", 0))
	var sel: Dictionary = await Network.send_and_wait(2, 3, {"char_id": cid})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	print("[live] select=", sel.get("code"), " enter=", snap.get("code"), " cid=", cid)
	WorldStore.apply_snapshot(snap)

	# 等机器人移动几个来回，让 3/6/3/3 都走到。
	await get_tree().create_timer(3.0).timeout
	var count := WorldStore.remote_players.size()
	print("[live] remote players=", count, " ids=", WorldStore.remote_players.keys())
	for id in WorldStore.remote_players:
		var info: Dictionary = WorldStore.remote_players[id]
		print("[live]   ", id, " name=", info.get("name"), " x=", info.get("x"),
			" faction=", info.get("faction"), " gender=", info.get("gender"))
	# 主流程已切到 2.5D 俯视：远端玩家挂在 YSortLayer 下（Remote_<id>），参与前后遮挡。
	var world := main.get_node("World")
	var nodes: Array = []
	var ysort := world.get_node_or_null("YSortLayer")
	if ysort != null:
		for n in ysort.get_children():
			if str(n.name).begins_with("Remote_"):
				nodes.append(n)
	print("[live] remote nodes=", nodes.size())
	for n in nodes:
		print("[live]   node ", n.name, " skin=", n.skin_key, " anim=",
			(n._sprite.animation if n._sprite else "nil"),
			" plate='", n._plate.text, "' vis=", n._plate.visible,
			" gpos=", n._plate.global_position, " size=", n._plate.size)

	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[live] empty viewport")
		get_tree().quit(1)
		return
	var err := img.save_png(OUT)
	print("[live] saved=", OUT, " size=", img.get_size(), " err=", err)
	get_tree().quit(0 if err == OK and count > 0 else 1)

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp := await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[live] 登录失败: ", resp)
		return ""
	return str(resp.get("data", {}).get("token", ""))

func _post(path: String, body: Dictionary) -> Dictionary:
	var req := HTTPRequest.new()
	add_child(req)
	var err := req.request("http://127.0.0.1:7788" + path, ["Content-Type: application/json"],
		HTTPClient.METHOD_POST, JSON.stringify(body))
	if err != OK:
		return {"code": -1, "msg": "HTTP 请求失败"}
	var res: Array = await req.request_completed
	req.queue_free()
	if int(res[0]) != HTTPRequest.RESULT_SUCCESS:
		return {"code": -1, "msg": "HTTP 无响应"}
	return JSON.parse_string((res[3] as PackedByteArray).get_string_from_utf8())
