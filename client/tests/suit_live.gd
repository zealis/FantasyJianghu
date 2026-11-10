extends Node
## 品质共鸣真实链路探针：登录带 5 件装备的角色 → 6/3 → 打印服务端 suit 原文并渲染共鸣行实拍。
## 用来区分「服务端没给」还是「客户端解析/渲染错」。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/suit_live.tscn

const USER := "背包探针_299892"
const PWD := "123456"
const OUT := "D:/project/Fantasy-Jianghu/outputs/suit_live.png"

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
	if chars.is_empty():
		printerr("[suit_live] 账号无角色")
		get_tree().quit(1)
		return
	var cid := int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": cid})
	WorldStore.apply_snapshot(await Network.send_and_wait(3, 1, {"map_id": "清河村"}))
	await _frames(5)

	# 直接看 6/3 的原始返回——这是服务端真实下发的内容。
	var raw: Dictionary = await Network.send_and_wait(6, Protocol.SUB_EQUIP_VIEW, {})
	print("[suit_live] 6/3 原始 suit = ", raw.get("suit"))
	print("[suit_live] 6/3 code=", raw.get("code"), " slots=", (raw.get("slots", {}) as Dictionary).size(),
		" attrs.hp=", (raw.get("attrs", {}) as Dictionary).get("hp"))

	await QuestStore.refresh_bag()
	await EquipStore.refresh()
	await _frames(6)
	print("[suit_live] EquipStore.suit = ", EquipStore.suit)

	var ui: Control = main.get_node("UILayer/QuestUI")
	ui.open_bag()
	await _frames(8)
	print("[suit_live] 共鸣行文本 = ", ui._resonance_label.text)

	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img != null and not img.is_empty():
		img.save_png(OUT)
		print("[suit_live] 截图已导出")
	print("[suit_live] DONE")
	get_tree().quit(0)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp := await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[suit_live] 登录失败: ", resp)
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
