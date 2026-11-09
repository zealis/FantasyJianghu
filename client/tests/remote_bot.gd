extends Node
## 同屏验证用机器人（headless）：登录 → 选角 → 进清河村 → 持续左右移动 40 秒。
## 运行：Godot_v4.7.2-stable_win64_console.exe --headless --path client res://tests/remote_bot.tscn
const USER := "remote_bot"
const PWD := "123456"
const CHAR_NAME := "同屏机器人"
const FACTION := "断岳剑宗"
const GENDER := "男"

func _ready() -> void:
	var token := await _login()
	if token == "":
		get_tree().quit(1)
		return
	var auth: Dictionary = await Network.connect_and_auth(Network.SERVER_URL, token)
	print("[bot] auth=", auth.get("code"))
	var chars: Array = auth.get("chars", [])
	var cid := 0
	if chars.is_empty():
		var cr: Dictionary = await Network.send_and_wait(2, 2,
			{"name": CHAR_NAME, "faction": FACTION, "gender": GENDER})
		cid = int(cr.get("char", {}).get("id", 0))
		print("[bot] create=", cr.get("code"), " id=", cid, " msg=", cr.get("msg", ""))
	else:
		cid = int(chars[0].get("id", 0))
	if cid == 0:
		get_tree().quit(1)
		return
	var sel: Dictionary = await Network.send_and_wait(2, 3, {"char_id": cid})
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	print("[bot] select=", sel.get("code"), " enter=", snap.get("code"), " cid=", cid)
	var y := float(snap.get("ground_y", 900.0))
	var x := 700.0
	var dir := 1
	for i in range(160):
		await get_tree().create_timer(0.25).timeout
		x += dir * 100.0
		if x > 1000.0 or x < 600.0:
			dir = -dir
		Network.send(3, 2, {"x": x, "y": y})
	print("[bot] done")
	get_tree().quit(0)

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp := await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[bot] 登录失败: ", resp)
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
