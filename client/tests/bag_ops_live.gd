extends Node
## 背包整理/出售/锁定真实链路探针（M2c 切片五）：5/3 整理 → 5/5 锁定并拒绝出售 → 解锁 →
## 数量选择器按 N 个出售（金币 = 单价 × N）→ 面板实拍（锁定态 + 选择器态两张）。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/bag_ops_live.tscn（需服务端 1.8.0）
## 每次运行用独立账号：出售会真的扣物品，固定账号重跑必然「背包已空」而误判失败。
const USER_BASE := "背包探针"
const PWD := "123456"
const OUT := "D:/project/Fantasy-Jianghu/outputs/bag_ops.png"
const OUT_LOCK := "D:/project/Fantasy-Jianghu/outputs/bag_ops_lock.png"

var _fail := 0
var _user := ""
var _char_name := ""

const DIGITS_CN := ["零", "一", "二", "三", "四", "五", "六", "七", "八", "九"]

func _ready() -> void:
	var sfx := str(int(Time.get_unix_time_from_system()) % 1000000)
	_user = USER_BASE + "_" + sfx
	# 角色名规则：2–6 个汉字。取时间后 4 位转汉字，拼成「探针三四五六」（6 字，唯一）。
	var digits := "%04d" % (int(sfx) % 10000)
	_char_name = "探针"
	for i in range(digits.length()):
		_char_name += DIGITS_CN[int(digits[i])]
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
		var cr: Dictionary = await Network.send_and_wait(2, 2, {"name": _char_name, "faction": "惊雷殿", "gender": "男"})
		char_id = int(cr.get("char", {}).get("id", 0))
	else:
		char_id = int(chars[0].get("id", 0))
	await Network.send_and_wait(2, 3, {"char_id": char_id})
	print("[bag_ops] user=", _user, " char_id=", char_id)
	var snap: Dictionary = await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	WorldStore.apply_snapshot(snap)
	await _frames(5)

	QuestStore.refresh()
	await _frames(4)
	if _bag_items().is_empty():
		# 保底：走主线拿奖励物品。
		if str(QuestStore.by_id("main_001").get("state", "")) == "available":
			await QuestStore.accept("main_001")
			await Network.send_and_wait(10, 1, {"map_id": "清河村", "npc_id": "hebo"})
			await _frames(3)
			await QuestStore.submit("main_001")
			await _frames(4)
	print("[bag_ops] items=", _bag_items())

	# 幂等保底：清掉上次运行可能残留的锁定态，否则重跑会因「全是锁定物品」误判。
	for it in _bag_items():
		if bool(it.get("locked", false)):
			await QuestStore.toggle_lock(str(it["id"]), false)
			await _frames(1)

	var r1 := await QuestStore.sort_bag()
	_check(r1 == "", "5/3 整理成功")
	var ranks := {"完美": 5, "极品": 4, "精品": 3, "良品": 2, "凡品": 1}
	var prev := 99
	var sorted_ok := true
	for it in _bag_items():
		var q := int(ranks.get(str(it.get("quality", "凡品")), 1))
		if q > prev:
			sorted_ok = false
		prev = q
	_check(sorted_ok and not _bag_items().is_empty(), "整理后按品质降序")

	# 出售目标优先选「数量 ≥ 2」的：卖 1 个后该格仍在，后面锁定才有对象；
	# 只有一个的物品卖完会整行删除（存储语义），会让锁定步骤失去目标。
	var target: Dictionary = {}
	for it in _bag_items():
		if int(it.get("sell", 0)) > 0 and not bool(it.get("locked", false)) \
				and int(it.get("count", 0)) >= 2:
			target = it
			break
	if target.is_empty():
		for it in _bag_items():
			if int(it.get("sell", 0)) > 0 and not bool(it.get("locked", false)):
				target = it
				break
	_check(not target.is_empty(), "背包中存在可出售物品")
	if target.is_empty():
		print("[bag_ops] DONE fail=", _fail)
		get_tree().quit(1)
		return
	var id := str(target["id"])
	var unit := int(target.get("sell", 0))
	var c0 := int(target.get("count", 0))
	var ui: Control = main.get_node("UILayer/QuestUI")
	ui._bag_panel.visible = true
	ui._selected_item = id
	ui._refresh_bag()
	await _frames(2)

	# ① 先验证锁定并截图：锁定后禁止出售，解锁才能卖，顺序反了就拍不到锁角标。
	var r3 := await QuestStore.toggle_lock(id, true)
	_check(r3 == "", "5/5 锁定成功")
	_check(bool(_find(id).get("locked", false)), "物品显示已锁定")
	ui._refresh_bag()
	await _frames(2)
	# 清掉上一步「已锁定」的 toast：它 2.6s 才消失，会压住属性列，影响截图判读。
	ui._toast.visible = false
	ui._toast_timer = 0.0
	await _frames(2)
	var ok_lock: bool = await _shot(OUT_LOCK)
	_check(ok_lock, "锁定态实拍已导出")
	var denied: Dictionary = await Network.send_and_wait(5, Protocol.SUB_BAG_SELL,
		{"item_id": id, "count": 1})
	_check(int(denied.get("code", -1)) != 0, "锁定后出售被服务端拒绝")

	# ② 解锁后按数量选择器出售，验证 gained = 单价 × 数量、售完复位。
	var r4 := await QuestStore.toggle_lock(id, false)
	_check(r4 == "" and not bool(_find(id).get("locked", true)), "5/5 解锁成功")
	ui._refresh_bag()
	await _frames(2)
	var want: int = mini(c0, 2)
	ui._set_sell_qty(want)
	_check(ui._sell_qty == want, "出售数量选择器可设为 %d" % want)
	_check(ui._sell_gain_label.text == "可得 %d 金币" % (unit * want),
		"选择器显示可得 %d 金币" % (unit * want))
	await _frames(2)
	ui._toast.visible = false
	ui._toast_timer = 0.0
	await _frames(2)
	var ok_sel: bool = await _shot(OUT)
	_check(ok_sel, "背包面板实拍已导出（选择器态）")
	var g0 := int(QuestStore.wallet.get("gold", 0))
	await ui._on_bag_sell()
	await _frames(3)
	_check(int(QuestStore.wallet.get("gold", 0)) == g0 + unit * want,
		"按 %d 个出售，金币 +%d" % [want, unit * want])
	_check(ui._sell_qty == 1, "出售后数量选择器复位为 1")
	var after := _find(id)
	_check(after.is_empty() or int(after.get("count", 0)) == c0 - want,
		"出售后数量 -%d" % want)

	# 收尾：兜底清掉任何残留锁定态。
	for it in _bag_items():
		if bool(it.get("locked", false)):
			await QuestStore.toggle_lock(str(it["id"]), false)

	print("[bag_ops] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _bag_items() -> Array:
	return QuestStore.bag.get("items", [])

func _find(id: String) -> Dictionary:
	for it in _bag_items():
		if str(it.get("id", "")) == id:
			return it
	return {}

func _check(cond: bool, label: String) -> void:
	print("[bag_ops] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

## 等一帧真正画完再抓图，否则会拿到上一帧（或空图）。
func _shot(path: String) -> bool:
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		return false
	return img.save_png(path) == OK

func _login() -> String:
	await _post("/api/register", {"username": _user, "password": PWD, "nickname": _user})
	var resp := await _post("/api/login", {"username": _user, "password": PWD})
	if int(resp.get("code", -1)) != 0:
		printerr("[bag_ops] 登录失败: ", resp)
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
