extends Control
## UI 层 · 加点面板（M2c 切片四 / B3），按 ui-mockups/加点界面.png 三栏定稿：
## 左：人物卡 + 可分配点数 + 洗点；中：四属性步进 + 推荐方案 + 一键加点；
## 右：属性预览（当前 → 预加后，增量绿字，服务端 11/1 stats 权威）。
## 确认走 2/5（预加增量），洗点走 2/6（本轮免费），数据全部来自 QuestStore。

signal closed

const AvatarFrames := preload("res://scripts/ui/avatar_frames.gd")

const C_PAPER := Color("F5ECD7")
const C_PAPER_2 := Color("EFE4CB")
const C_EDGE := Color("8A6D3B")
const C_INK := Color("3A2E1C")
const C_MUTED := Color("6B573A")
const C_ACC := Color("DE6423")
const C_ACC_DEEP := Color("B34A14")
const C_GOOD := Color("3F7A3F")
const C_HEAD := Color("4A3826")

const ATTRS := [
	{"key": "str", "name": "力量", "color": "C0602A", "gain": "物攻 +2 · 命中 +1"},
	{"key": "spi", "name": "灵力", "color": "7B4FA0", "gain": "法攻 +2 · 真气 +5"},
	{"key": "agi", "name": "敏捷", "color": "2F7F86", "gain": "速度 +1 · 闪避 +10"},
	{"key": "con", "name": "体质", "color": "4E8A4F", "gain": "生命 +10 · 物防 +1"},
]
const PRESETS := [
	{"name": "3力1敏", "plan": {"str": 3, "agi": 1}},
	{"name": "4力全攻", "plan": {"str": 4}},
	{"name": "2力1敏1体", "plan": {"str": 2, "agi": 1, "con": 1}},
]
const PREVIEW_ROWS := [
	{"key": "hp", "name": "生命", "pct": false},
	{"key": "mp", "name": "真气", "pct": false},
	{"key": "patk", "name": "物理攻击", "pct": false},
	{"key": "matk", "name": "法术攻击", "pct": false},
	{"key": "pdef", "name": "物理防御", "pct": false},
	{"key": "mdef", "name": "法术防御", "pct": false},
	{"key": "speed", "name": "战斗速度", "pct": false},
	{"key": "pcrit", "name": "物理暴击", "pct": true},
]

var _pending := {"str": 0, "spi": 0, "agi": 0, "con": 0}
var _unspent := 0
var _preset_idx := 0

var _avail_label: Label
var _name_label: Label
var _sub_label: Label
var _title_label: Label
var _portrait: TextureRect
var _value_labels: Dictionary = {}
var _pending_labels: Dictionary = {}
var _plus_buttons: Dictionary = {}
var _minus_buttons: Dictionary = {}
var _preview_now: Dictionary = {}
var _preview_after: Dictionary = {}
var _preview_delta: Dictionary = {}
var _foot: Label
var _confirm: Button
var _preset_buttons: Array = []

func _ready() -> void:
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_STOP
	_build()
	QuestStore.changed.connect(_sync)
	_sync()

# ---- 构建 ----

func _panel_style(bg: Color, border: Color, radius: int = 8) -> StyleBoxFlat:
	var sb := StyleBoxFlat.new()
	sb.bg_color = bg
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = border
	sb.corner_radius_top_left = radius
	sb.corner_radius_top_right = radius
	sb.corner_radius_bottom_left = radius
	sb.corner_radius_bottom_right = radius
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 10
	sb.content_margin_bottom = 10
	return sb

func _paper(bg: Color = C_PAPER_2) -> Panel:
	var p := Panel.new()
	p.add_theme_stylebox_override("panel", _panel_style(bg, C_EDGE))
	return p

func _label(text: String, size: int, color: Color = C_INK) -> Label:
	var l := Label.new()
	l.text = text
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	return l

func _button(text: String, primary: bool = false) -> Button:
	var b := Button.new()
	b.text = text
	b.add_theme_font_size_override("font_size", 15)
	if primary:
		for st in ["normal", "hover", "pressed", "disabled"]:
			var sb := _panel_style(C_ACC if st != "pressed" else C_ACC_DEEP, C_ACC_DEEP, 6)
			sb.content_margin_left = 14
			sb.content_margin_right = 14
			b.add_theme_stylebox_override(st, sb)
		b.add_theme_color_override("font_color", Color("FFF7EA"))
		b.add_theme_color_override("font_disabled_color", Color(1, 1, 1, 0.55))
	else:
		for st in ["normal", "hover", "pressed", "disabled"]:
			var sb := _panel_style(C_PAPER, C_EDGE, 6)
			sb.content_margin_left = 14
			sb.content_margin_right = 14
			b.add_theme_stylebox_override(st, sb)
		b.add_theme_color_override("font_color", C_INK)
	return b

func _build() -> void:
	var dim := ColorRect.new()
	dim.color = Color(0.06, 0.05, 0.03, 0.58)
	dim.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	dim.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(dim)

	var panel := Panel.new()
	panel.add_theme_stylebox_override("panel", _panel_style(C_PAPER, C_EDGE, 10))
	# 固定 1280×720 视口下的绝对布局：Control 挂在 Node2D 下时 anchors 会按零尺寸父级算，
	# 导致面板跑到负坐标（实拍只看到右下角）。统一用 position/size。
	panel.position = Vector2(40, 24)
	panel.size = Vector2(1200, 672)
	panel.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(panel)

	# 标题条
	var head := ColorRect.new()
	head.color = C_HEAD
	head.position = Vector2(2, 2)
	head.size = Vector2(1196, 44)
	panel.add_child(head)
	var title := _label("加点", 20, Color("F7EEDC"))
	title.position = Vector2(0, 0)
	title.size = Vector2(1196, 44)
	title.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	title.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	head.add_child(title)
	var back := _button("‹", false)
	back.position = Vector2(8, 7)
	back.size = Vector2(40, 32)
	back.pressed.connect(_close)
	head.add_child(back)

	var body := HBoxContainer.new()
	body.position = Vector2(12, 56)
	body.size = Vector2(1176, 552)
	body.add_theme_constant_override("separation", 12)
	panel.add_child(body)

	body.add_child(_build_left())
	body.add_child(_build_middle())
	body.add_child(_build_right())

	# 底部操作条
	var footer := HBoxContainer.new()
	footer.position = Vector2(16, 618)
	footer.size = Vector2(1168, 42)
	footer.add_theme_constant_override("separation", 12)
	panel.add_child(footer)
	_foot = _label("", 13, C_MUTED)
	_foot.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	footer.add_child(_foot)
	var sp := Control.new()
	sp.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	footer.add_child(sp)
	var res := _button("重置", false)
	res.custom_minimum_size = Vector2(110, 36)
	res.pressed.connect(_reset_pending)
	footer.add_child(res)
	_confirm = _button("确认加点", true)
	_confirm.custom_minimum_size = Vector2(140, 36)
	_confirm.pressed.connect(_confirm_points)
	footer.add_child(_confirm)

func _build_left() -> Control:
	var col := _paper()
	col.custom_minimum_size = Vector2(300, 0)
	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	for side in ["left", "right", "top", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 12)
	col.add_child(margin)
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 8)
	margin.add_child(v)

	var head := HBoxContainer.new()
	head.add_theme_constant_override("separation", 10)
	v.add_child(head)
	var avatar := TextureRect.new()
	avatar.custom_minimum_size = Vector2(64, 64)
	avatar.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	avatar.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	avatar.texture = _avatar_texture()
	head.add_child(avatar)
	var info := VBoxContainer.new()
	info.add_theme_constant_override("separation", 2)
	head.add_child(info)
	_name_label = _label(SessionStore.player_name, 20, C_INK)
	info.add_child(_name_label)
	_sub_label = _label("%d级 · %s" % [QuestStore.level, SessionStore.faction], 13, C_MUTED)
	info.add_child(_sub_label)
	_title_label = _label("", 12, C_ACC_DEEP)
	info.add_child(_title_label)

	_portrait = TextureRect.new()
	_portrait.custom_minimum_size = Vector2(0, 360)
	_portrait.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_portrait.size_flags_vertical = Control.SIZE_EXPAND_FILL
	_portrait.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	# 正面立绘（heroes/single，512×512 方图）：按高度铺满卡片，人物居中，左右留白被裁、本体不裁。
	_portrait.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	_portrait.texture = _portrait_texture()
	v.add_child(_portrait)

	var sep := HSeparator.new()
	v.add_child(sep)
	var avail := HBoxContainer.new()
	avail.add_theme_constant_override("separation", 8)
	v.add_child(avail)
	var al := _label("可分配点数", 15, C_INK)
	al.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	avail.add_child(al)
	var asp := Control.new()
	asp.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	avail.add_child(asp)
	_avail_label = _label("0", 30, C_ACC)
	avail.add_child(_avail_label)
	var wash := _button("洗点", false)
	wash.custom_minimum_size = Vector2(72, 30)
	wash.pressed.connect(_wash)
	avail.add_child(wash)
	var note := _label("本轮洗点免费（洗髓丹消耗待物品轮）", 11, C_MUTED)
	v.add_child(note)
	return col

func _build_middle() -> Control:
	var col := _paper()
	col.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	for side in ["left", "right", "top", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 12)
	col.add_child(margin)
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 8)
	margin.add_child(v)

	var head := HBoxContainer.new()
	v.add_child(head)
	head.add_child(_label("一级属性", 17, C_INK))
	var sp := Control.new()
	sp.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	head.add_child(sp)
	head.add_child(_label("每升一级 +4 点", 12, C_MUTED))

	for def in ATTRS:
		v.add_child(_attr_row(def))

	var sp2 := Control.new()
	sp2.size_flags_vertical = Control.SIZE_EXPAND_FILL
	v.add_child(sp2)

	var rec := HBoxContainer.new()
	rec.add_theme_constant_override("separation", 8)
	v.add_child(rec)
	rec.add_child(_label("推荐加点", 13, C_MUTED))
	for i in range(PRESETS.size()):
		var b := _button(str(PRESETS[i]["name"]), false)
		b.add_theme_font_size_override("font_size", 13)
		b.custom_minimum_size = Vector2(0, 30)
		b.pressed.connect(_pick_preset.bind(i))
		_preset_buttons.append(b)
		rec.add_child(b)
	var one := _button("一键加点", true)
	one.custom_minimum_size = Vector2(100, 30)
	one.pressed.connect(_fill_all)
	rec.add_child(one)
	return col

func _attr_row(def: Dictionary) -> Control:
	var key := str(def["key"])
	var row := Panel.new()
	row.add_theme_stylebox_override("panel", _panel_style(C_PAPER, Color("D8CBA8"), 6))
	row.custom_minimum_size = Vector2(0, 68)
	var h := HBoxContainer.new()
	h.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	h.offset_left = 10
	h.offset_right = -10
	h.offset_top = 6
	h.offset_bottom = -6
	h.add_theme_constant_override("separation", 8)
	row.add_child(h)

	var dot := Panel.new()
	dot.custom_minimum_size = Vector2(12, 12)
	dot.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	dot.add_theme_stylebox_override("panel", _panel_style(Color(str(def["color"])), Color(str(def["color"])), 6))
	h.add_child(dot)

	var left := VBoxContainer.new()
	left.add_theme_constant_override("separation", 0)
	left.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	h.add_child(left)
	left.add_child(_label(str(def["name"]), 16, C_INK))
	left.add_child(_label("每点：" + str(def["gain"]), 11, C_MUTED))

	var val := _label("0", 30, C_INK)
	val.custom_minimum_size = Vector2(52, 0)
	val.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
	val.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	h.add_child(val)
	_value_labels[key] = val

	# 预加增量（定稿橙字）：显示在数值右侧，确认后清零。
	var pend := _label("", 13, C_ACC)
	pend.custom_minimum_size = Vector2(30, 0)
	pend.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
	pend.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	h.add_child(pend)
	_pending_labels[key] = pend

	var minus := _button("－", false)
	minus.custom_minimum_size = Vector2(32, 32)
	minus.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	minus.pressed.connect(_add.bind(key, -1))
	h.add_child(minus)
	_minus_buttons[key] = minus

	var plus := _button("＋", true)
	plus.custom_minimum_size = Vector2(32, 32)
	plus.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	plus.pressed.connect(_add.bind(key, 1))
	h.add_child(plus)
	_plus_buttons[key] = plus
	return row

func _build_right() -> Control:
	var col := _paper()
	col.custom_minimum_size = Vector2(304, 0)
	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	for side in ["left", "right", "top", "bottom"]:
		margin.add_theme_constant_override("margin_" + side, 12)
	col.add_child(margin)
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 6)
	margin.add_child(v)
	v.add_child(_label("属性预览", 17, C_INK))

	for row in PREVIEW_ROWS:
		var key := str(row["key"])
		var h := HBoxContainer.new()
		h.add_theme_constant_override("separation", 6)
		v.add_child(h)
		var nl := _label(str(row["name"]), 13, C_MUTED)
		nl.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		h.add_child(nl)
		var now := _label("--", 14, C_INK)
		now.custom_minimum_size = Vector2(64, 0)
		now.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
		h.add_child(now)
		_preview_now[key] = now
		var arrow := _label("", 13, C_MUTED)
		arrow.custom_minimum_size = Vector2(18, 0)
		arrow.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
		h.add_child(arrow)
		var after := _label("", 14, C_GOOD)
		after.custom_minimum_size = Vector2(64, 0)
		after.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
		h.add_child(after)
		_preview_after[key] = after
		var dl := _label("", 12, C_GOOD)
		dl.custom_minimum_size = Vector2(44, 0)
		h.add_child(dl)
		_preview_delta[key] = dl

	var gap := Control.new()
	gap.size_flags_vertical = Control.SIZE_EXPAND_FILL
	v.add_child(gap)
	for line in ["· 预览为本轮预加点后的数值", "· 确认后生效，战斗中不可更改", "· 洗点返还全部已分配点数"]:
		v.add_child(_label(line, 11, C_MUTED))
	return col

# ---- 数据 ----

func _avatar_texture() -> Texture2D:
	var path := "res://assets/avatars/%s_%s.png" % [SessionStore.faction, SessionStore.gender]
	if ResourceLoader.exists(path):
		return AvatarFrames.trim(load(path))
	return load("res://assets/sprites/player/idle_0.png")

func _portrait_texture() -> Texture2D:
	# 定稿要求正面立绘：优先 assets/heroes/single/<门派>_<性别>.png（512×512），
	# 缺图才回落行走并腿帧，保证任何角色都能打开面板。
	var front := "res://assets/heroes/single/%s_%s.png" % [SessionStore.faction, SessionStore.gender]
	if ResourceLoader.exists(front):
		return load(front)
	var key := SessionStore.faction + SessionStore.gender
	var walk := "res://assets/sprites/walk/%s/walk_1.png" % key
	if ResourceLoader.exists(walk):
		return load(walk)
	return load("res://assets/sprites/player/idle_1.png")

func _pending_sum() -> int:
	var s := 0
	for k in _pending:
		s += int(_pending[k])
	return s

func _add(key: String, delta: int) -> void:
	if delta > 0 and _pending_sum() >= _unspent:
		return
	var v := int(_pending[key]) + delta
	if v < 0:
		return
	_pending[key] = v
	_sync()

func _pick_preset(idx: int) -> void:
	_preset_idx = idx
	_sync()

func _fill_all() -> void:
	var plan: Dictionary = PRESETS[_preset_idx]["plan"]
	while _pending_sum() < _unspent:
		var added := false
		for k in plan:
			var remain := _unspent - _pending_sum()
			if remain <= 0:
				break
			var add: int = mini(int(plan[k]), remain)
			_pending[k] = int(_pending[k]) + add
			added = add > 0
		if not added:
			break
	_sync()

func _reset_pending() -> void:
	_pending = {"str": 0, "spi": 0, "agi": 0, "con": 0}
	_sync()

func _confirm_points() -> void:
	if _pending_sum() <= 0:
		return
	var delta := _pending.duplicate()
	var err := await QuestStore.allocate(delta)
	if err == "":
		_pending = {"str": 0, "spi": 0, "agi": 0, "con": 0}
	_sync()

func _wash() -> void:
	await QuestStore.reset_points()
	_pending = {"str": 0, "spi": 0, "agi": 0, "con": 0}
	_sync()

func _close() -> void:
	closed.emit()
	queue_free()

func _delta_for(key: String) -> int:
	var p_str := int(_pending["str"])
	var p_spi := int(_pending["spi"])
	var p_agi := int(_pending["agi"])
	var p_con := int(_pending["con"])
	match key:
		"hp":
			return p_con * 10
		"mp":
			return p_spi * 5
		"patk":
			return p_str * 2
		"matk":
			return p_spi * 2
		"pdef":
			return p_con * 1
		"speed":
			return p_agi * 1
		"pdodge":
			return p_agi * 10
		"pcrit":
			return 0
	return 0

func _fmt(key: String, value: int) -> String:
	if key == "pcrit":
		return "%.1f%%" % (float(value) / 100.0)
	return _thousands(value)

func _thousands(v: int) -> String:
	var s := str(v)
	var out := ""
	var count := 0
	for i in range(s.length() - 1, -1, -1):
		out = s[i] + out
		count += 1
		if count % 3 == 0 and i > 0:
			out = "," + out
	return out

func _sync() -> void:
	_unspent = QuestStore.unspent_points
	_name_label.text = SessionStore.player_name
	_sub_label.text = "%d级 · %s" % [QuestStore.level, SessionStore.faction]
	_title_label.text = ("【%s】" % SessionStore.title) if SessionStore.title != "" else "【初入江湖】"
	if _portrait.texture == null:
		_portrait.texture = _portrait_texture()

	var used := _pending_sum()
	_avail_label.text = str(maxi(0, _unspent - used))

	var total_allocated := 0
	for def in ATTRS:
		var key := str(def["key"])
		var cur := int(QuestStore.points.get(key, 0))
		total_allocated += cur
		var pend := int(_pending[key])
		var val: Label = _value_labels[key]
		val.text = str(cur + pend)
		val.add_theme_color_override("font_color", C_ACC if pend > 0 else C_INK)
		var plus: Button = _plus_buttons[key]
		plus.disabled = used >= _unspent
		var minus: Button = _minus_buttons[key]
		minus.disabled = pend <= 0
		var pl: Label = _pending_labels[key]
		pl.text = ("+%d" % pend) if pend > 0 else ""

	for row in PREVIEW_ROWS:
		var key := str(row["key"])
		var base := int(QuestStore.stats.get(key, 0))
		var delta := _delta_for(key)
		var now: Label = _preview_now[key]
		var after: Label = _preview_after[key]
		var dl: Label = _preview_delta[key]
		if QuestStore.stats.is_empty():
			now.text = "--"
			after.text = ""
			dl.text = ""
			continue
		now.text = _fmt(key, base)
		if delta > 0:
			after.text = "→ " + _fmt(key, base + delta)
			dl.text = "(+%d)" % delta if key != "pcrit" else "(+%.1f%%)" % (float(delta) / 100.0)
		else:
			after.text = ""
			dl.text = ""

	for i in range(_preset_buttons.size()):
		var b: Button = _preset_buttons[i]
		var on: bool = i == _preset_idx
		# 选中态用朱橙描边 + 浅橙底，不用 disabled（disabled 会把文字灰掉看不清）。
		b.add_theme_color_override("font_color", C_ACC_DEEP if on else C_INK)
		var sb2 := _panel_style(Color("F3D9BE") if on else C_PAPER, C_ACC if on else C_EDGE, 6)
		sb2.content_margin_left = 10
		sb2.content_margin_right = 10
		b.add_theme_stylebox_override("normal", sb2)
	_confirm.disabled = used <= 0
	_foot.text = "已累计分配 %d 点 · 当前预加 %d 点，确认后剩余 %d 点" % [
		total_allocated, used, maxi(0, _unspent - used)]
