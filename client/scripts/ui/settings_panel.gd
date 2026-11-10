extends Control
## UI 层 · 设置（按 ui-mockups/设置界面.png 定稿：音频/画面/战斗/系统四组卡 + 底栏）。
## 全部控件接 Settings 真实生效并持久化；震屏演出未接入，「震动提示」留位置灰不伪造。

const C_FRAME := Color("2B3654")
const C_TITLE := Color("B08D57")
const C_TITLE_TEXT := Color("FFFAEB")
const C_PAPER := Color("F5ECD7")
const C_TAB := Color("EEE4CB")
const C_EDGE := Color("8A6D3B")
const C_EDGE_LIGHT := Color("C9B586")
const C_INK := Color("4A3823")
const C_MUTED := Color("78603C")
const C_HEAD := Color("8A3410")
const C_ACC := Color("C66E34")
const C_TRACK := Color("D8CBA8")
const C_OFF := Color("C9BFA8")

var _panel: Control
var _percent_labels: Dictionary = {}
var _seg_buttons: Dictionary = {}
var _break_btn: Button
var _break_label: Label

func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	_build()
	Settings.changed.connect(_sync)
	QuestStore.changed.connect(_sync)
	_sync()

func open() -> void:
	_panel.visible = true
	_sync()

func close() -> void:
	_panel.visible = false

func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventKey and event.pressed and not event.echo and event.keycode == KEY_ESCAPE:
		if _panel.visible:
			close()
			get_viewport().set_input_as_handled()

# ---- 构件 ----

func _box(bg: Color, border: Color = Color(0, 0, 0, 0), width: int = 0, radius: int = 4,
		margin: int = 0) -> StyleBoxFlat:
	var sb := StyleBoxFlat.new()
	sb.bg_color = bg
	sb.set_corner_radius_all(radius)
	sb.set_border_width_all(width)
	sb.border_color = border
	sb.set_content_margin_all(margin)
	return sb

func _label(text: String, size: int, color: Color = C_INK, bold: bool = false) -> Label:
	var l := Label.new()
	l.text = text
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	if bold:
		var f := FontVariation.new()
		f.base_font = ThemeDB.fallback_font
		f.variation_embolden = 0.6
		l.add_theme_font_override("font", f)
	return l

func _build() -> void:
	_panel = Control.new()
	_panel.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_panel.mouse_filter = Control.MOUSE_FILTER_STOP
	_panel.visible = false
	add_child(_panel)
	var frame := ColorRect.new()
	frame.color = C_FRAME
	frame.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	frame.mouse_filter = Control.MOUSE_FILTER_IGNORE
	_panel.add_child(frame)
	var paper := PanelContainer.new()
	paper.add_theme_stylebox_override("panel", _box(C_PAPER, C_EDGE, 2, 8, 0))
	paper.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	paper.offset_left = 10
	paper.offset_top = 10
	paper.offset_right = -10
	paper.offset_bottom = -10
	_panel.add_child(paper)
	var vb := VBoxContainer.new()
	vb.add_theme_constant_override("separation", 0)
	paper.add_child(vb)
	vb.add_child(_title_bar())
	var body := VBoxContainer.new()
	body.size_flags_vertical = Control.SIZE_EXPAND_FILL
	body.add_theme_constant_override("separation", 16)
	var wrap := PanelContainer.new()
	wrap.add_theme_stylebox_override("panel", _box(Color(0, 0, 0, 0), Color(0, 0, 0, 0), 0, 0, 16))
	wrap.size_flags_vertical = Control.SIZE_EXPAND_FILL
	wrap.add_child(body)
	vb.add_child(wrap)
	var grid := GridContainer.new()
	grid.columns = 2
	grid.add_theme_constant_override("h_separation", 16)
	grid.add_theme_constant_override("v_separation", 16)
	body.add_child(grid)
	grid.add_child(_card("音 频", [
		_slider_row("背景音乐", "bgm"),
		_slider_row("游戏音效", "sfx"),
		_toggle_row("一键静音", "mute"),
	]))
	grid.add_child(_card("画 面", [
		_seg_row("画质", "quality", ["低", "中", "高"], [0, 1, 2]),
		_toggle_row("技能特效", "vfx"),
		_toggle_row("省电模式", "power_save"),
	]))
	grid.add_child(_card("战 斗", [
		_toggle_row("自动战斗", "auto_battle"),
		_seg_row("战斗加速", "speed_index", ["×1", "×2", "×3"], [0, 1, 2]),
		_toggle_row("伤害飘字", "float_text"),
	]))
	grid.add_child(_card("系 统", [
		_toggle_row("消息提醒", "notify"),
		_toggle_row("震动提示", "", true, "震屏演出未接入，开放后启用"),
		_seg_row("帧率上限", "fps_cap", ["30", "60"], [30, 60]),
		_break_row(),
	]))
	var gap := Control.new()
	gap.size_flags_vertical = Control.SIZE_EXPAND_FILL
	body.add_child(gap)
	var bottom := HBoxContainer.new()
	bottom.add_theme_constant_override("separation", 12)
	body.add_child(bottom)
	bottom.add_child(_label("版本 %s" % str(ProjectSettings.get_setting("application/config/version")), 12, C_MUTED))
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	bottom.add_child(spacer)
	var switch := _button("切换账号", false)
	switch.custom_minimum_size = Vector2(120, 36)
	switch.pressed.connect(func() -> void: _logout(false))
	bottom.add_child(switch)
	var quit := _button("退出登录", true)
	quit.custom_minimum_size = Vector2(140, 36)
	quit.pressed.connect(func() -> void: _logout(true))
	bottom.add_child(quit)

func _title_bar() -> PanelContainer:
	var bar := PanelContainer.new()
	bar.add_theme_stylebox_override("panel", _box(C_TITLE, Color(0, 0, 0, 0), 0, 0, 0))
	bar.custom_minimum_size = Vector2(0, 44)
	var hb := HBoxContainer.new()
	bar.add_child(hb)
	var back := Button.new()
	back.text = "‹"
	back.flat = true
	back.custom_minimum_size = Vector2(44, 44)
	back.add_theme_font_size_override("font_size", 22)
	back.add_theme_color_override("font_color", C_TITLE_TEXT)
	back.pressed.connect(func() -> void: close())
	hb.add_child(back)
	var t := _label("设 置", 18, C_TITLE_TEXT, true)
	t.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	t.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	hb.add_child(t)
	var pad := Control.new()
	pad.custom_minimum_size = Vector2(44, 0)
	hb.add_child(pad)
	return bar

func _card(title: String, rows: Array) -> PanelContainer:
	var card := PanelContainer.new()
	card.add_theme_stylebox_override("panel", _box(C_PAPER, C_EDGE, 2, 8, 14))
	card.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	card.size_flags_vertical = Control.SIZE_EXPAND_FILL
	var vb := VBoxContainer.new()
	vb.add_theme_constant_override("separation", 18)
	card.add_child(vb)
	vb.add_child(_label(title, 16, C_HEAD, true))
	var sep := HSeparator.new()
	sep.add_theme_stylebox_override("separator", _box(C_EDGE_LIGHT, Color(0, 0, 0, 0), 0, 0, 0))
	sep.custom_minimum_size = Vector2(0, 1)
	vb.add_child(sep)
	for row in rows:
		vb.add_child(row)
	return card

func _row_head(text: String) -> HBoxContainer:
	var hb := HBoxContainer.new()
	hb.add_theme_constant_override("separation", 12)
	var name_lab := _label(text, 14, C_INK)
	name_lab.custom_minimum_size = Vector2(140, 0)
	name_lab.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	hb.add_child(name_lab)
	return hb

func _slider_row(text: String, key: String) -> Control:
	var hb := _row_head(text)
	var slider := HSlider.new()
	slider.name = "Slider_%s" % key
	slider.min_value = 0
	slider.max_value = 100
	slider.step = 1
	slider.value = int(Settings.get(key))
	slider.custom_minimum_size = Vector2(240, 18)
	var track := _box(C_TRACK, Color(0, 0, 0, 0), 0, 3, 0)
	track.content_margin_top = 3
	track.content_margin_bottom = 3
	slider.add_theme_stylebox_override("slider", track)
	var fill := _box(C_ACC, Color(0, 0, 0, 0), 0, 3, 0)
	fill.content_margin_top = 3
	fill.content_margin_bottom = 3
	slider.add_theme_stylebox_override("grabber_area", fill)
	slider.add_theme_stylebox_override("grabber_area_highlight", fill)
	slider.add_theme_icon_override("grabber", _knob_texture())
	slider.add_theme_icon_override("grabber_highlight", _knob_texture())
	hb.add_child(slider)
	var pct := _label("%d%%" % int(Settings.get(key)), 14, C_INK, true)
	pct.custom_minimum_size = Vector2(48, 0)
	pct.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	_percent_labels[key] = pct
	hb.add_child(pct)
	var tail := Control.new()
	tail.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	hb.add_child(tail)
	slider.value_changed.connect(func(v: float) -> void:
		Settings.set_value(key, int(v))
		pct.text = "%d%%" % int(v))
	return hb

func _break_row() -> Control:
	# 文档口径（GDD §4.2）：普通等级经验满**自动升级**，只有 29/39/49/59/69/79 是突破关口，
	# 需要手动「突破升级」才能跨越。所以本行只在关口等级出现按钮，平时只显示等级与经验。
	var hb := _row_head("等级与突破")
	_break_label = _label("", 12, C_MUTED)
	_break_label.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_break_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	hb.add_child(_break_label)
	_break_btn = _button("突破", true)
	_break_btn.custom_minimum_size = Vector2(72, 28)
	_break_btn.pressed.connect(func() -> void: await QuestStore.breakthrough())
	hb.add_child(_break_btn)
	return hb

func _toggle_row(text: String, key: String, disabled: bool = false, tooltip: String = "") -> Control:
	var hb := _row_head(text)
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	hb.add_child(spacer)
	var cb := CheckButton.new()
	cb.name = "Toggle_%s" % (key if key != "" else "shake")
	cb.button_pressed = bool(Settings.get(key)) if key != "" else false
	cb.disabled = disabled
	cb.tooltip_text = tooltip
	cb.add_theme_icon_override("checked", _toggle_texture(true))
	cb.add_theme_icon_override("unchecked", _toggle_texture(false))
	for style in ["normal", "hover", "pressed", "focus", "disabled"]:
		cb.add_theme_stylebox_override(style, StyleBoxEmpty.new())
	hb.add_child(cb)
	if key != "":
		cb.toggled.connect(func(on: bool) -> void: Settings.set_value(key, on))
	return hb

func _seg_row(text: String, key: String, options: Array, values: Array) -> Control:
	var hb := _row_head(text)
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	hb.add_child(spacer)
	var seg := HBoxContainer.new()
	seg.add_theme_constant_override("separation", 0)
	hb.add_child(seg)
	var buttons: Array = []
	for i in range(options.size()):
		var b := Button.new()
		b.text = str(options[i])
		b.custom_minimum_size = Vector2(56, 28)
		b.add_theme_font_size_override("font_size", 14)
		var sb := _box(C_PAPER, C_EDGE, 1, 0, 4)
		sb.border_width_left = 1 if i == 0 else 0
		sb.border_width_right = 1 if i == options.size() - 1 else 0
		sb.corner_radius_top_left = 4 if i == 0 else 0
		sb.corner_radius_bottom_left = 4 if i == 0 else 0
		sb.corner_radius_top_right = 4 if i == options.size() - 1 else 0
		sb.corner_radius_bottom_right = 4 if i == options.size() - 1 else 0
		b.add_theme_stylebox_override("normal", sb)
		b.add_theme_stylebox_override("hover", sb)
		b.add_theme_stylebox_override("pressed", sb)
		b.add_theme_stylebox_override("focus", StyleBoxEmpty.new())
		b.pressed.connect(func() -> void: Settings.set_value(key, values[i]))
		seg.add_child(b)
		buttons.append(b)
	_seg_buttons[key] = {"buttons": buttons, "values": values}
	return hb

func _button(text: String, primary: bool) -> Button:
	var b := Button.new()
	b.text = text
	b.add_theme_font_size_override("font_size", 15)
	if primary:
		b.add_theme_stylebox_override("normal", _box(C_ACC, C_EDGE, 1, 4, 8))
		b.add_theme_stylebox_override("hover", _box(C_ACC.lightened(0.08), C_EDGE, 1, 4, 8))
		b.add_theme_stylebox_override("pressed", _box(Color("B34A14"), C_EDGE, 2, 4, 8))
		b.add_theme_color_override("font_color", C_TITLE_TEXT)
	else:
		b.add_theme_stylebox_override("normal", _box(C_TAB, C_EDGE, 2, 4, 8))
		b.add_theme_stylebox_override("hover", _box(C_TAB.lightened(0.05), C_EDGE, 2, 4, 8))
		b.add_theme_stylebox_override("pressed", _box(C_TAB.darkened(0.08), C_EDGE, 2, 4, 8))
		b.add_theme_color_override("font_color", C_INK)
	return b

## 设置回写后同步控件（含跨面板修改与持久化加载）。
func _sync() -> void:
	for key in _percent_labels:
		(_percent_labels[key] as Label).text = "%d%%" % int(Settings.get(key))
		var slider := find_child("Slider_%s" % key, true, false) as HSlider
		if slider and int(slider.value) != int(Settings.get(key)):
			slider.value = int(Settings.get(key))
	for key in _seg_buttons:
		var info: Dictionary = _seg_buttons[key]
		var buttons: Array = info["buttons"]
		var values: Array = info["values"]
		for i in range(buttons.size()):
			var b: Button = buttons[i]
			var sel: bool = int(Settings.get(key)) == int(values[i])
			var sb := b.get_theme_stylebox("normal").duplicate() as StyleBoxFlat
			sb.bg_color = C_TITLE if sel else C_PAPER
			b.add_theme_stylebox_override("normal", sb)
			b.add_theme_stylebox_override("hover", sb)
			b.add_theme_color_override("font_color", C_TITLE_TEXT if sel else C_INK)
	for key in ["mute", "vfx", "power_save", "auto_battle", "float_text", "notify"]:
		var cb := find_child("Toggle_%s" % key, true, false) as CheckButton
		if cb and cb.button_pressed != bool(Settings.get(key)):
			cb.button_pressed = bool(Settings.get(key))
	if _break_btn != null:
		var lv := QuestStore.level
		var gate := QuestStore.next_gate
		var at_gate := gate > 0 and lv == gate
		var head := "等级 %d · 经验 %d/%d · 未分配点 %d" % [
			lv, QuestStore.exp, QuestStore.exp_next, QuestStore.unspent_points]
		if at_gate:
			_break_btn.visible = true
			_break_btn.disabled = not QuestStore.gate_ready
			_break_btn.tooltip_text = "跨越当前关口" if QuestStore.gate_ready else "经验满后自动停在此关口，需手动突破"
			_break_label.text = head + ("· 经验已满，可突破" if QuestStore.gate_ready else "· 经验满后可突破")
		else:
			# 非关口等级：隐藏突破按钮，明确「自动升级」，消除「要不要手动点」的误解。
			_break_btn.visible = false
			if gate > 0:
				_break_label.text = head + "· 经验满自动升级（%d 级为突破关口）" % gate
			else:
				_break_label.text = head + "· 经验满自动升级"

func _logout(clear_account: bool) -> void:
	close()
	var main := get_tree().current_scene
	if main and main.has_method("return_to_login"):
		main.call("return_to_login", clear_account)

# ---- 运行时贴图（纸体系滑杆钮/开关，不引外部素材） ----

func _knob_texture() -> ImageTexture:
	var img := Image.create(18, 18, false, Image.FORMAT_RGBA8)
	img.fill(Color(0, 0, 0, 0))
	for y in range(18):
		for x in range(18):
			var d := Vector2(x - 8.5, y - 8.5).length()
			if d <= 7.0:
				img.set_pixel(x, y, Color("FFFAEB"))
			elif d <= 8.5:
				img.set_pixel(x, y, C_EDGE)
	return ImageTexture.create_from_image(img)

func _toggle_texture(on: bool) -> ImageTexture:
	var img := Image.create(44, 22, false, Image.FORMAT_RGBA8)
	img.fill(Color(0, 0, 0, 0))
	var bg := C_ACC if on else C_OFF
	for y in range(22):
		for x in range(44):
			var in_pill := x >= 11 and x < 33
			in_pill = in_pill or Vector2(x - 11, y - 11).length() <= 11.0
			in_pill = in_pill or Vector2(x - 33, y - 11).length() <= 11.0
			if in_pill:
				img.set_pixel(x, y, bg)
	var kx := 33.0 if on else 11.0
	for y in range(22):
		for x in range(44):
			if Vector2(x - kx, y - 11).length() <= 8.0:
				img.set_pixel(x, y, Color("FFFAEB"))
	return ImageTexture.create_from_image(img)