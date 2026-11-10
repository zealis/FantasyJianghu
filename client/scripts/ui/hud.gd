extends Control
## UI 层 · 主界面 HUD（对齐 ui-mockups/主界面_清河村_最终版本版.png）：
## 左上角色卡（门派×性别头像 + 等级徽 + 名字【称号】通则 + 血/蓝/经验条）；
## 顶部地点横幅；横幅下分线入口（V1.27）；右侧纯图标列；左下快捷行。
## M1 无战斗属性来源：条以比例显示、无数值标签（计划登记偏差，M2 属性下发后启用）。

const COLOR_INK := Color("3b2e1e")
const COLOR_PAPER := Color("f6efdf")

var _theme: Theme = preload("res://resources/theme_paper.tres")
const AvatarFrames := preload("res://scripts/ui/avatar_frames.gd")
const PointPanelScript := preload("res://scripts/ui/point_panel.gd")
var _points_panel: Control

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	_build_character_card()
	_build_location_banner()
	_build_line_entry()
	_build_right_icons()
	_build_quick_row()
	SessionStore.session_changed.connect(_refresh)
	WorldStore.snapshot_applied.connect(_refresh_line_entry)
	_refresh()

func _panel() -> PanelContainer:
	var p := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(1, 1, 1, 0.88)
	sb.corner_radius_top_left = 8
	sb.corner_radius_top_right = 8
	sb.corner_radius_bottom_left = 8
	sb.corner_radius_bottom_right = 8
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = Color("a88b62")
	sb.content_margin_left = 10
	sb.content_margin_right = 10
	sb.content_margin_top = 8
	sb.content_margin_bottom = 8
	p.add_theme_stylebox_override("panel", sb)
	return p

## 左上角色卡：头像（等级徽）+ 名字 + 三色条。
func _build_character_card() -> void:
	var card := _panel()
	card.position = Vector2(12, 12)
	card.custom_minimum_size = Vector2(280, 0)
	card.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(card)
	var hbox := HBoxContainer.new()
	hbox.add_theme_constant_override("separation", 10)
	card.add_child(hbox)

	# 头像 + 等级徽（叠放）。
	var avatar_wrap := Control.new()
	avatar_wrap.custom_minimum_size = Vector2(64, 64)
	hbox.add_child(avatar_wrap)
	var avatar := TextureRect.new()
	avatar.name = "Avatar"
	avatar.texture = _avatar_texture()
	avatar.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	avatar.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	avatar.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	# 用户定案：加点入口就是左上头像（不再放设置里）。
	avatar.mouse_filter = Control.MOUSE_FILTER_STOP
	avatar.mouse_default_cursor_shape = Control.CURSOR_POINTING_HAND
	avatar.tooltip_text = "点击查看 / 分配属性点"
	avatar.gui_input.connect(_on_avatar_input)
	avatar_wrap.add_child(avatar)

	# 定稿头像带细画框；素材自带的框随 11% 裁剪一起去掉后，这里补一圈同色描边。
	var frame := Panel.new()
	frame.name = "AvatarFrame"
	frame.mouse_filter = Control.MOUSE_FILTER_IGNORE
	var fsb := StyleBoxFlat.new()
	fsb.bg_color = Color(0, 0, 0, 0)
	fsb.border_width_left = 2
	fsb.border_width_right = 2
	fsb.border_width_top = 2
	fsb.border_width_bottom = 2
	fsb.border_color = Color("8a6a3b")
	frame.add_theme_stylebox_override("panel", fsb)
	frame.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	avatar_wrap.add_child(frame)
	var badge := Label.new()
	badge.name = "LevelBadge"
	badge.text = str(SessionStore.level)
	badge.add_theme_font_size_override("font_size", 11)
	badge.add_theme_color_override("font_color", COLOR_INK)
	var bsb := StyleBoxFlat.new()
	bsb.bg_color = COLOR_PAPER
	bsb.border_width_left = 2
	bsb.border_width_right = 2
	bsb.border_width_top = 2
	bsb.border_width_bottom = 2
	bsb.border_color = Color("8a6a3b")
	bsb.corner_radius_top_left = 11
	bsb.corner_radius_top_right = 11
	bsb.corner_radius_bottom_left = 11
	bsb.corner_radius_bottom_right = 11
	bsb.content_margin_left = 0
	bsb.content_margin_right = 0
	badge.add_theme_stylebox_override("normal", bsb)
	# 定稿：圆形等级徽压在头像右下角。锚到右下角后必须用 offset 定位；
	# 之前 anchors 后再设 position=(40,44) 会被当成「父节点右下 + 偏移」，直接甩出卡片。
	badge.set_anchors_preset(Control.PRESET_BOTTOM_RIGHT)
	# 22×22（头像 64px 的 34%）：骑在头像右下角上（向内 18px、向外 4px），
	# 遮的是肩部/法器而不是脸；30px 版本会压到下巴，24px 贴内沿仍蹭到脸颊。
	badge.offset_left = -15
	badge.offset_top = -15
	badge.offset_right = 7
	badge.offset_bottom = 7
	badge.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	badge.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	avatar_wrap.add_child(badge)

	var vbox := VBoxContainer.new()
	vbox.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	vbox.add_theme_constant_override("separation", 4)
	hbox.add_child(vbox)
	var name_lab := Label.new()
	name_lab.name = "NameLabel"
	name_lab.text = SessionStore.display_name()  # 【称号】+名字通则（无称号仅名字）
	name_lab.add_theme_font_size_override("font_size", 16)
	name_lab.add_theme_color_override("font_color", COLOR_INK)
	vbox.add_child(name_lab)
	for bar in [["HpBar", Color("c0392b"), SessionStore.hp_ratio], ["MpBar", Color("2f7fa8"), SessionStore.mp_ratio], ["ExpBar", Color("4e8a4f"), SessionStore.exp_ratio]]:
		var pb := ProgressBar.new()
		pb.name = bar[0]
		pb.custom_minimum_size = Vector2(170, 12)
		pb.max_value = 1.0
		pb.value = bar[2]
		pb.show_percentage = false
		var track := StyleBoxFlat.new()
		track.bg_color = Color("e8e0cf")
		track.corner_radius_top_left = 6
		track.corner_radius_top_right = 6
		track.corner_radius_bottom_left = 6
		track.corner_radius_bottom_right = 6
		var fill := StyleBoxFlat.new()
		fill.bg_color = bar[1]
		fill.corner_radius_top_left = 6
		fill.corner_radius_top_right = 6
		fill.corner_radius_bottom_left = 6
		fill.corner_radius_bottom_right = 6
		pb.add_theme_stylebox_override("background", track)
		pb.add_theme_stylebox_override("fill", fill)
		vbox.add_child(pb)

func _on_avatar_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_open_points()

## 加点面板挂在 UILayer（HUD 的父级），盖在世界与 HUD 之上。
func _open_points() -> void:
	if _points_panel != null and is_instance_valid(_points_panel):
		_points_panel.queue_free()
	var p := Control.new()
	p.set_script(PointPanelScript)
	var host: Node = get_parent()
	if host == null:
		host = self
	host.add_child(p)
	_points_panel = p

func _avatar_texture() -> Texture2D:
	var path := "res://assets/avatars/%s_%s.png" % [SessionStore.faction, SessionStore.gender]
	if ResourceLoader.exists(path):
		# 素材自带画框、部分门派顶部有横杆：裁掉四周 11% 再缩到 64px，
		# 否则角色头顶会出现「一根横线」。
		return AvatarFrames.trim(load(path))
	return load("res://assets/sprites/player/idle_0.png")

## 顶部地点横幅。
func _build_location_banner() -> void:
	var banner := _panel()
	banner.set_anchors_and_offsets_preset(Control.PRESET_TOP_WIDE)
	banner.offset_left = 500
	banner.offset_right = -500
	banner.offset_top = 12
	banner.offset_bottom = 48
	banner.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(banner)
	var label := Label.new()
	label.name = "LocationLabel"
	label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	label.text = WorldStore.location_banner()
	label.add_theme_font_size_override("font_size", 16)
	label.add_theme_color_override("font_color", COLOR_INK)
	banner.add_child(label)

## 分线切换入口（V1.27）：横幅正下方小纸钮「N线 ▾」，点开列各线人数。
func _build_line_entry() -> void:
	var btn := Button.new()
	btn.name = "LineEntry"
	btn.text = "1线 ▾"
	btn.mouse_filter = Control.MOUSE_FILTER_STOP
	btn.position = Vector2(560, 52)
	btn.custom_minimum_size = Vector2(160, 24)
	btn.pressed.connect(_on_line_entry_pressed)
	add_child(btn)

func _refresh_line_entry() -> void:
	var btn := find_child("LineEntry", true, false) as Button
	if btn:
		btn.text = "%d线 ▾" % WorldStore.line_no

func _on_line_entry_pressed() -> void:
	var existing := find_child("LinePopup", true, false)
	if existing:
		existing.queue_free()
		return
	var resp: Dictionary = await WorldStore.fetch_lines()
	_show_line_popup(resp)

func _show_line_popup(resp: Dictionary) -> void:
	if int(resp.get("code", -1)) != 0:
		return
	var popup := PanelContainer.new()
	popup.name = "LinePopup"
	popup.position = Vector2(560, 78)
	popup.custom_minimum_size = Vector2(160, 0)
	var vbox := VBoxContainer.new()
	popup.add_child(vbox)
	for l in resp.get("lines", []):
		var info: Dictionary = l
		var no := int(info.get("no", 0))
		var count := int(info.get("count", 0))
		var capacity := int(info.get("capacity", 0))
		var line_btn := Button.new()
		line_btn.text = "%d线  %d/%d%s" % [no, count, capacity, "（当前）" if no == WorldStore.line_no else ""]
		line_btn.disabled = no == WorldStore.line_no or count >= capacity
		line_btn.pressed.connect(_on_line_switch.bind(no))
		vbox.add_child(line_btn)
	if (resp.get("lines", []) as Array).is_empty():
		var empty := Label.new()
		empty.text = "暂无分线数据"
		vbox.add_child(empty)
	add_child(popup)

func _on_line_switch(no: int) -> void:
	var err: String = await WorldStore.switch_line(no)
	var popup := find_child("LinePopup", true, false)
	if popup:
		popup.queue_free()
	if err != "":
		_toast(err)

## 右侧纯图标列（定稿：设置/背包/秘籍/任务/仓库/坐骑蛋/金元宝）。
func _build_right_icons() -> void:
	var col := VBoxContainer.new()
	col.set_anchors_and_offsets_preset(Control.PRESET_RIGHT_WIDE)
	col.offset_left = -64
	col.offset_right = -16
	col.offset_top = 16
	col.offset_bottom = -160
	col.alignment = BoxContainer.ALIGNMENT_BEGIN
	col.add_theme_constant_override("separation", 14)
	add_child(col)
	for icon in ["设置", "背包", "秘籍", "任务", "仓库", "坐骑蛋", "金元宝"]:
		col.add_child(_icon_button(icon))

## 左下快捷行（聊天/邮件/队伍/宗派）。
func _build_quick_row() -> void:
	var row := HBoxContainer.new()
	row.position = Vector2(16, 660)
	row.add_theme_constant_override("separation", 18)
	add_child(row)
	for icon in ["聊天", "邮件", "队伍", "宗派"]:
		row.add_child(_icon_button(icon))

func _icon_button(icon_name: String) -> Button:
	var b := Button.new()
	b.name = "Icon_%s" % icon_name
	b.custom_minimum_size = Vector2(44, 44)
	var path := "res://assets/icons/%s.png" % icon_name
	if ResourceLoader.exists(path):
		b.icon = load(path)
		b.expand_icon = true
	b.tooltip_text = icon_name
	b.pressed.connect(func() -> void:
		if icon_name == "背包" or icon_name == "任务":
			_open_quest_ui(icon_name)
		elif icon_name == "设置":
			var sp := get_node_or_null("../SettingsPanel")
			if sp:
				sp.call("open")
			else:
				_toast("设置功能即将开放")
		else:
			_toast("%s 功能即将开放" % icon_name))
	return b

## 背包/任务图标直达定稿面板（QuestUI 与 HUD 同挂 UILayer）。
func _open_quest_ui(icon_name: String) -> void:
	var ui := get_node_or_null("../QuestUI")
	if ui == null:
		_toast("%s 功能即将开放" % icon_name)
		return
	ui.call("open_bag" if icon_name == "背包" else "open_quest")

## 轻提示（自动消失）。消息提醒关闭时不落 toast（设置·系统）。
func _toast(msg: String) -> void:
	if not Settings.notify:
		return
	var existing := find_child("Toast", true, false)
	if existing:
		existing.queue_free()
	var t := Label.new()
	t.name = "Toast"
	t.text = msg
	t.add_theme_font_size_override("font_size", 14)
	t.add_theme_color_override("font_color", Color("fdf6ea"))
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.2, 0.15, 0.1, 0.9)
	sb.corner_radius_top_left = 6
	sb.corner_radius_top_right = 6
	sb.corner_radius_bottom_left = 6
	sb.corner_radius_bottom_right = 6
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	var p := PanelContainer.new()
	p.name = "Toast"
	p.add_theme_stylebox_override("panel", sb)
	p.set_anchors_and_offsets_preset(Control.PRESET_CENTER_TOP)
	p.offset_top = 90
	p.add_child(t)
	p.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(p)
	var timer := get_tree().create_timer(2.0)
	timer.timeout.connect(func() -> void:
		if is_instance_valid(p):
			p.queue_free())

func _refresh() -> void:
	var n := find_child("NameLabel", true, false) as Label
	if n:
		n.text = SessionStore.display_name()
	var badge := find_child("LevelBadge", true, false) as Label
	if badge:
		badge.text = str(SessionStore.level)
	var avatar := find_child("Avatar", true, false) as TextureRect
	if avatar:
		avatar.texture = _avatar_texture()
	var loc := find_child("LocationLabel", true, false) as Label
	if loc:
		loc.text = WorldStore.location_banner()
