extends Node
## 实机链路面板探针：真实登录 → 选角 → 进场 → 真实接取/对话/交付 → J/B 面板截图。
## 验证定稿面板渲染的是服务端真实回包（11/1 / 11/2 / 10/1 / 11/3 / 5/1），不是离线假数据。
## 运行：Godot_v4.7.2-stable_win64.exe --path client res://tests/live_quest_capture.tscn
const USER := "面板探针"
const PWD := "123456"
const OUT_QUEST := "D:/project/Fantasy-Jianghu/client/tests/live_quest_panel.png"
const OUT_BAG := "D:/project/Fantasy-Jianghu/client/tests/live_quest_bag.png"

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
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": "面板探针侠", "faction": "断岳剑宗", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
		print("[live-quest] create code=", cr.get("code"), " id=", char_id)
	else:
		char_id = int(chars[0].get("id", 0))
	var sel: Dictionary = await Network.send_and_wait(2, 3, {"char_id": char_id})
	# 探针绕过登录页，按 login.gd 选角口径回填 SessionStore（门派/性别决定站台皮肤与属性列）。
	var info: Dictionary = {}
	for c in chars:
		if int(c.get("id", 0)) == char_id:
			info = c
	SessionStore.player_name = str(info.get("name", SessionStore.player_name))
	SessionStore.faction = str(info.get("faction", "断岳剑宗"))
	SessionStore.gender = str(info.get("gender", "男"))
	SessionStore.notify_changed()
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	print("[live-quest] select=", sel.get("code"), " enter=", snap.get("code"))
	WorldStore.apply_snapshot(snap)
	await _frames(6)
	print("[live-quest] 11/1 loaded=", QuestStore.loaded, " quests=", QuestStore.quests.size(),
		" bag used=", QuestStore.bag.get("used"), "/", QuestStore.bag.get("capacity"))

	# 真实走一环主线：接取 → 对话推进 → 交付（奖励入包），再接下一环形成 active 行。
	if str(QuestStore.by_id("main_001").get("state", "")) == "available":
		print("[live-quest] accept=", await QuestStore.accept("main_001"))
		var talk: Dictionary = await Network.send_and_wait(10, 1, {"map_id": "清河村", "npc_id": "hebo"})
		print("[live-quest] talk code=", talk.get("code"))
		await _frames(4)
		print("[live-quest] submit=", await QuestStore.submit("main_001"))
	if str(QuestStore.by_id("main_002").get("state", "")) == "available":
		print("[live-quest] accept2=", await QuestStore.accept("main_002"))
	await _frames(4)

	var ui: Control = main.get_node("UILayer/QuestUI")
	ui.call("open_quest")
	await _frames(6)
	if not await _capture(OUT_QUEST):
		return
	ui.call("open_bag")
	ui._selected_item = "pill_xiaohuan"
	ui._refresh_bag()
	await _frames(6)
	if not await _capture(OUT_BAG):
		return
	# M2c 切片一：日常链产出经验丹 → 5/2 使用 → exp+100（需服务端 1.4.0）。
	var daily_id := "daily_n_001"
	if str(QuestStore.by_id(daily_id).get("state", "")) == "available":
		print("[live-quest] accept daily=", await QuestStore.accept(daily_id))
		var re: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
		WorldStore.apply_snapshot(re)
		await _frames(4)
		print("[live-quest] submit daily=", await QuestStore.submit(daily_id))
	await _frames(3)
	var has_dan := false
	for it in QuestStore.bag.get("items", []):
		if str(it.get("id", "")) == "exp_dan":
			has_dan = true
	if has_dan:
		var exp_before := QuestStore.exp
		var level_before := QuestStore.level
		print("[live-quest] use exp_dan=", await QuestStore.use_item("exp_dan"))
		await _frames(3)
		print("[live-quest] exp before=", exp_before, " after=", QuestStore.exp, " level=", QuestStore.level)
		# 1.6.0 起经验入账会结算升级：要么同级 exp+100，要么跨级（溢出被消耗）。
		var same_level_gain: bool = QuestStore.level == level_before and QuestStore.exp == exp_before + 100
		if not same_level_gain and QuestStore.level <= level_before:
			printerr("[live-quest] FAIL 使用经验丹未加经验/未升级")
			get_tree().quit(1)
			return
	else:
		print("[live-quest] WARN 背包无经验丹（日常已交付过），跳过使用断言")
	# M2c 切片二：对话教头 → 交付 main_002 得粗木剑 → 穿戴 → 物攻+18 → 卸下。
	if str(QuestStore.by_id("main_002").get("state", "")) == "active":
		await Network.send_and_wait(10, 1, {"map_id": "清河村", "npc_id": "tieshan"})
		await _frames(3)
		print("[live-quest] submit main_002=", await QuestStore.submit("main_002"))
	await _frames(3)
	await EquipStore.refresh()
	if not EquipStore.loaded:
		printerr("[live-quest] FAIL 装备视图未加载（服务端未加载 1.5.0？）")
		get_tree().quit(1)
		return
	var has_sword := false
	for it in QuestStore.bag.get("items", []):
		if str(it.get("id", "")) == "sword_wood":
			has_sword = true
	if not has_sword:
		printerr("[live-quest] FAIL 背包无粗木剑")
		get_tree().quit(1)
		return
	var patk_before := int(EquipStore.attrs.get("patk", 0))
	print("[live-quest] wear=", await EquipStore.wear("sword_wood"))
	var patk_after := int(EquipStore.attrs.get("patk", 0))
	print("[live-quest] patk before=", patk_before, " after=", patk_after)
	if patk_after != patk_before + 18:
		printerr("[live-quest] FAIL 穿戴未加物攻 18")
		get_tree().quit(1)
		return
	print("[live-quest] remove=", await EquipStore.remove("weapon"))
	# 设置底栏退出登录：真断线 + 回登录层 + 清令牌。
	var sp: Control = main.get_node("UILayer/SettingsPanel")
	sp.call("open")
	await _frames(4)
	main.call("return_to_login", true)
	await _frames(4)
	var login_visible: bool = main.get_node("LoginLayer").visible
	print("[live-quest] logout login_visible=", login_visible, " offline=", Network.offline,
		" token_empty=", SessionStore.token == "")
	if not login_visible or not Network.offline or SessionStore.token != "":
		printerr("[live-quest] FAIL 退出登录未回登录层/未断线/未清令牌")
		get_tree().quit(1)
		return
	print("[live-quest] DONE wallet=", QuestStore.wallet, " bag=", QuestStore.bag.get("items"))
	get_tree().quit(0)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _capture(path: String) -> bool:
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null:
		printerr("[live-quest] 取帧失败: ", path)
		get_tree().quit(1)
		return false
	var err := img.save_png(path)
	if err != OK:
		printerr("[live-quest] 保存失败: ", path, " err=", err)
		get_tree().quit(1)
		return false
	print("[live-quest] saved ", path, " ", img.get_width(), "x", img.get_height())
	return true

func _login() -> String:
	await _post("/api/register", {"username": USER, "password": PWD, "nickname": USER})
	var resp: Dictionary = await _post("/api/login", {"username": USER, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[live-quest] 登录失败: ", resp)
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