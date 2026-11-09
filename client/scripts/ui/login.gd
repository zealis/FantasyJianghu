extends Control
## UI 层 · 登录/注册页签卡（UI 规范 §9 定稿：登录界面.png / 注册界面.png）。
## 首页国风长卷底图（资产素材/美术素材/首页背景.webp，已裁为 16:9）+ 居中宣纸卡：
## 登录（账号/密码/服务器行/协议勾选/CTA）；注册（账号/密码/确认密码/昵称/协议/注册并登录）。
## 底部健康忠告与版本号。

const HTTP_BASE := "http://127.0.0.1:7788"

# 宣纸色板（UI 规范 §2 近似取色）。
const COLOR_PAPER := Color("f6efdf")
const COLOR_INK := Color("3b2e1e")
const COLOR_ACCENT := Color("c46a2b")
const COLOR_BORDER := Color("a88b62")

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _status: Label
var _tab_login: Button
var _tab_register: Button
var _login_box: VBoxContainer
var _register_box: VBoxContainer
var _account_edit: LineEdit
var _password_edit: LineEdit
var _reg_account: LineEdit
var _reg_password: LineEdit
var _reg_confirm: LineEdit
var _reg_nickname: LineEdit
var _agree_login: CheckBox
var _agree_register: CheckBox
var _char_panel: VBoxContainer  # 角色列表/创角区（T9/T10 接管流程）

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_build_bg()
	_build_card()
	Network.authenticated_ok.connect(_on_authenticated)
	Network.authenticated_failed.connect(_on_auth_failed)
	Network.disconnected.connect(_on_disconnected)

func _build_bg() -> void:
	var bg := TextureRect.new()
	bg.texture = load("res://assets/ui/home_bg.jpg")
	bg.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	bg.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(bg)
	# 长卷较亮，压一层极淡墨褐，保证居中宣纸卡与文字仍有足够对比（V1.28 §3）。
	var shade := ColorRect.new()
	shade.color = Color(0.16, 0.11, 0.06, 0.16)
	shade.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	shade.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(shade)

func _build_card() -> void:
	var center := CenterContainer.new()
	center.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(center)

	var panel := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = COLOR_PAPER
	sb.corner_radius_top_left = 10
	sb.corner_radius_top_right = 10
	sb.corner_radius_bottom_left = 10
	sb.corner_radius_bottom_right = 10
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = COLOR_BORDER
	sb.content_margin_left = 28
	sb.content_margin_right = 28
	sb.content_margin_top = 20
	sb.content_margin_bottom = 16
	panel.add_theme_stylebox_override("panel", sb)
	panel.custom_minimum_size = Vector2(520, 0)
	center.add_child(panel)

	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 10)
	panel.add_child(root)

	var title := Label.new()
	title.text = "幻想江湖"
	title.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	title.add_theme_font_size_override("font_size", 40)
	title.add_theme_color_override("font_color", COLOR_INK)
	root.add_child(title)

	# 页签行：登录 / 注册。
	var tabs := HBoxContainer.new()
	tabs.alignment = BoxContainer.ALIGNMENT_CENTER
	tabs.add_theme_constant_override("separation", 40)
	root.add_child(tabs)
	_tab_login = _make_tab("登 录")
	_tab_register = _make_tab("注 册")
	_tab_login.pressed.connect(func() -> void: _switch_tab(true))
	_tab_register.pressed.connect(func() -> void: _switch_tab(false))
	tabs.add_child(_tab_login)
	tabs.add_child(_tab_register)

	_login_box = _build_login_box()
	root.add_child(_login_box)
	_register_box = _build_register_box()
	root.add_child(_register_box)

	_char_panel = VBoxContainer.new()
	_char_panel.add_theme_constant_override("separation", 6)
	_char_panel.visible = false
	root.add_child(_char_panel)

	_status = Label.new()
	_status.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_status.add_theme_color_override("font_color", COLOR_ACCENT)
	root.add_child(_status)

	# 底部：健康忠告 + 版本号。
	var footer := HBoxContainer.new()
	root.add_child(footer)
	var notice := Label.new()
	notice.text = "健康游戏忠告：抵制不良游戏，拒绝盗版游戏。注意自我保护，谨防受骗上当。"
	notice.add_theme_font_size_override("font_size", 11)
	notice.add_theme_color_override("font_color", Color("7a6a52"))
	notice.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	footer.add_child(notice)
	var ver := Label.new()
	ver.text = "版本 V1.0.0"
	ver.add_theme_font_size_override("font_size", 11)
	ver.add_theme_color_override("font_color", Color("7a6a52"))
	footer.add_child(ver)

	_switch_tab(true)

func _make_tab(text: String) -> Button:
	var b := Button.new()
	b.text = text
	b.flat = true
	b.add_theme_font_size_override("font_size", 20)
	b.add_theme_color_override("font_color", COLOR_INK)
	return b

func _switch_tab(login_active: bool) -> void:
	_login_box.visible = login_active
	_register_box.visible = not login_active
	_tab_login.add_theme_color_override("font_color", COLOR_ACCENT if login_active else COLOR_INK)
	_tab_register.add_theme_color_override("font_color", COLOR_INK if login_active else COLOR_ACCENT)
	_status.text = ""

func _field(placeholder: String, secret := false) -> LineEdit:
	var e := LineEdit.new()
	e.placeholder_text = placeholder
	e.secret = secret
	e.custom_minimum_size = Vector2(0, 34)
	return e

func _build_login_box() -> VBoxContainer:
	var box := VBoxContainer.new()
	box.add_theme_constant_override("separation", 8)
	_account_edit = _field("账 号")
	box.add_child(_account_edit)
	_password_edit = _field("密 码", true)
	box.add_child(_password_edit)

	# 服务器行（静态：清河一服 · 畅通）。
	var server_row := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color("fdf8ec")
	sb.border_width_left = 1
	sb.border_width_right = 1
	sb.border_width_top = 1
	sb.border_width_bottom = 1
	sb.border_color = COLOR_BORDER
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	server_row.add_theme_stylebox_override("panel", sb)
	var row := HBoxContainer.new()
	server_row.add_child(row)
	var lab := Label.new()
	lab.text = "服务器"
	lab.custom_minimum_size = Vector2(64, 0)
	lab.add_theme_color_override("font_color", COLOR_INK)
	row.add_child(lab)
	var val := Label.new()
	val.text = "清河一服"
	val.add_theme_color_override("font_color", COLOR_INK)
	val.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	row.add_child(val)
	var ok := Label.new()
	ok.text = "● 畅通"
	ok.add_theme_color_override("font_color", Color("3f9d5a"))
	row.add_child(ok)
	box.add_child(server_row)

	_agree_login = CheckBox.new()
	_agree_login.text = "已阅读并同意《用户协议》与《隐私政策》"
	_agree_login.button_pressed = true
	box.add_child(_agree_login)

	var cta := _cta_button("登 录 游 戏")
	cta.pressed.connect(_on_login_pressed)
	box.add_child(cta)

	var links := HBoxContainer.new()
	links.alignment = BoxContainer.ALIGNMENT_CENTER
	links.add_theme_constant_override("separation", 60)
	box.add_child(links)
	var quick := _link_button("快速注册")
	quick.pressed.connect(func() -> void: _switch_tab(false))
	links.add_child(quick)
	var forgot := _link_button("忘记密码")
	forgot.pressed.connect(func() -> void: _set_status("请联系客服找回密码（M1 未开放）"))
	links.add_child(forgot)
	return box

func _build_register_box() -> VBoxContainer:
	var box := VBoxContainer.new()
	box.add_theme_constant_override("separation", 8)
	_reg_account = _field("账 号")
	box.add_child(_reg_account)
	_reg_password = _field("密 码", true)
	box.add_child(_reg_password)
	_reg_confirm = _field("确认密码", true)
	box.add_child(_reg_confirm)
	_reg_nickname = _field("昵 称")
	box.add_child(_reg_nickname)
	_agree_register = CheckBox.new()
	_agree_register.text = "已阅读并同意《用户协议》与《隐私政策》"
	_agree_register.button_pressed = true
	box.add_child(_agree_register)
	var cta := _cta_button("注 册 并 登 录")
	cta.pressed.connect(_on_register_and_login)
	box.add_child(cta)
	return box

func _cta_button(text: String) -> Button:
	var b := Button.new()
	b.text = text
	b.custom_minimum_size = Vector2(0, 44)
	var sb := StyleBoxFlat.new()
	sb.bg_color = COLOR_ACCENT
	sb.corner_radius_top_left = 6
	sb.corner_radius_top_right = 6
	sb.corner_radius_bottom_left = 6
	sb.corner_radius_bottom_right = 6
	b.add_theme_stylebox_override("normal", sb)
	b.add_theme_color_override("font_color", Color("fdf6ea"))
	b.add_theme_font_size_override("font_size", 18)
	return b

func _link_button(text: String) -> Button:
	var b := Button.new()
	b.text = text
	b.flat = true
	b.add_theme_color_override("font_color", COLOR_ACCENT)
	return b

func _set_status(text: String, is_err := false) -> void:
	_status.text = text
	_status.add_theme_color_override("font_color", Color("b3432a") if is_err else COLOR_ACCENT)

## ---- HTTP 注册/登录（§5.3 登录注册走 HTTP）----

func _on_register_and_login() -> void:
	var user := _reg_account.text.strip_edges()
	var pwd := _reg_password.text
	if pwd != _reg_confirm.text:
		_set_status("两次输入的密码不一致", true)
		return
	if not _agree_register.button_pressed:
		_set_status("请先阅读并同意协议", true)
		_set_status("请先阅读并同意协议")
		return
	_set_status("注册中…")
	var resp := await _http_post("/api/register", {"username": user, "password": pwd, "nickname": _reg_nickname.text.strip_edges()})
	if int(resp.get("code", -1)) != 0:
		_set_status("注册失败：%s" % str(resp.get("msg", "未知错误")), true)
		return
	# 注册并登录：直接登录。
	await _do_login(user, pwd)

func _on_login_pressed() -> void:
	if not _agree_login.button_pressed:
		_set_status("请先阅读并同意协议", true)
		return
	await _do_login(_account_edit.text.strip_edges(), _password_edit.text)

func _do_login(user: String, pwd: String) -> void:
	_set_status("登录中…")
	var resp := await _http_post("/api/login", {"username": user, "password": pwd})
	if int(resp.get("code", -1)) != 0:
		_set_status("登录失败：%s" % str(resp.get("msg", "未知错误")), true)
		return
	var data: Dictionary = resp.get("data", {})
	SessionStore.token = str(data.get("token", ""))
	_set_status("连接服务器…")
	var auth: Dictionary = await Network.connect_and_auth()
	SessionStore.account_id = int(auth.get("account_id", SessionStore.account_id))  # M9 回填
	SessionStore.factions = auth.get("factions", [])  # 八派明细（宗门卡片来源）

func _credentials() -> Dictionary:
	return {"username": _account_edit.text.strip_edges(), "password": _password_edit.text}

func _http_post(path: String, body: Dictionary) -> Dictionary:
	var req := HTTPRequest.new()
	add_child(req)
	req.request(HTTP_BASE + path, ["Content-Type: application/json"], HTTPClient.METHOD_POST, JSON.stringify(body))
	var result: Array = await req.request_completed
	req.queue_free()
	if int(result[0]) != HTTPRequest.RESULT_SUCCESS:
		return {"code": -1, "msg": "网络错误（服务端未启动？）"}
	return JSON.parse_string(result[3].get_string_from_utf8()) if result[3].size() > 0 else {}

## ---- 鉴权完成 → 角色列表（T9/T10 将接管创角流程）----

func _on_authenticated(chars: Array) -> void:
	_set_status("")
	_login_box.visible = false
	_register_box.visible = false
	_tab_login.visible = false
	_tab_register.visible = false
	_refresh_chars(chars)

func _on_auth_failed(msg: String) -> void:
	_set_status(msg, true)
	_login_box.visible = true
	_tab_login.visible = true
	_tab_register.visible = true
	_switch_tab(true)

func _on_disconnected(msg: String) -> void:
	_set_status(msg + "，请重新登录", true)
	_char_panel.visible = false
	_login_box.visible = true
	_tab_login.visible = true
	_tab_register.visible = true
	_switch_tab(true)

func _refresh_chars(chars: Array) -> void:
	SessionStore.chars = chars
	for c in _char_panel.get_children():
		c.queue_free()
	_char_panel.visible = true
	if chars.is_empty():
		# 创角两步流（§9）：命名 → 选择宗门 → 创建。
		_open_naming()
		return
	for entry in chars:
		var info: Dictionary = entry
		var btn := Button.new()
		btn.text = "%s · %s · Lv%d" % [info.get("name", "?"), info.get("faction", "?"), int(info.get("level", 1))]
		btn.custom_minimum_size = Vector2(0, 40)
		btn.pressed.connect(_on_char_selected.bind(int(info.get("id", 0)), info))
		_char_panel.add_child(btn)

func _open_naming() -> void:
	_set_status("")
	var naming := preload("res://scenes/ui/naming.tscn").instantiate()
	add_child(naming)
	naming.naming_done.connect(func(n: String, g: String) -> void:
		naming.queue_free()
		_open_sect_select(n, g))

func _open_sect_select(char_name: String, gender: String) -> void:
	var sect := preload("res://scenes/ui/sect_select.tscn").instantiate()
	sect._factions = SessionStore.factions
	add_child(sect)
	sect.creation_confirmed.connect(func(faction: String) -> void:
		sect.queue_free()
		_create_character(char_name, faction, gender))

func _create_character(char_name: String, faction: String, gender: String) -> void:
	_set_status("创建角色中…")
	var resp := await Network.send_and_wait(2, 2, {"name": char_name, "faction": faction, "gender": gender})
	if int(resp.get("code", -1)) != 0:
		_set_status("创建失败：%s" % str(resp.get("msg", "未知错误")), true)
		return
	# 创建成功 → 选角 → 进场（复用选角流转）。
	var created: Dictionary = resp.get("char", {})
	SessionStore.player_name = str(created.get("name", char_name))
	SessionStore.faction = faction
	SessionStore.gender = gender
	SessionStore.notify_changed()
	_set_status("进入江湖…")
	var sel := await Network.send_and_wait(2, 3, {"char_id": created.get("id", 0)})
	if int(sel.get("code", -1)) != 0:
		_set_status("选角失败：%s" % str(sel.get("msg", "未知错误")), true)
		return
	var snap := await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	if int(snap.get("code", -1)) != 0:
		_set_status("进场失败：%s" % str(snap.get("msg", "未知错误")), true)
		return
	WorldStore.apply_snapshot(snap)

func _on_char_selected(char_id: int, info: Dictionary) -> void:
	SessionStore.player_name = str(info.get("name", SessionStore.player_name))
	SessionStore.faction = str(info.get("faction", SessionStore.faction))
	SessionStore.gender = str(info.get("gender", SessionStore.gender))
	SessionStore.notify_changed()
	_set_status("进入江湖…")
	var resp := await Network.send_and_wait(2, 3, {"char_id": char_id})
	if int(resp.get("code", -1)) != 0:
		_set_status("选角失败：%s" % str(resp.get("msg", "未知错误")), true)
		return
	# 进场 3/1 为请求-回包：快照随回包到达（seq 配对），应用后转主世界。
	var snap := await Network.send_and_wait(3, 1, {"map_id": "清河村"})
	if int(snap.get("code", -1)) != 0:
		_set_status("进场失败：%s" % str(snap.get("msg", "未知错误")), true)
		return
	WorldStore.apply_snapshot(snap)
