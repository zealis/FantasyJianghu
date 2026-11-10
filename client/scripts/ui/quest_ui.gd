extends Control
## UI 层 · 任务与背包（GDD §9 / 数据全部来自 QuestStore，服务端权威）。
## 按 ui-mockups 定稿重做：任务界面.png / 背包界面_套装显示版背包样式最终版_v3.png。
## 取色优先于规范令牌（承战斗轮定案），色值见 项目进行记录/任务背包定稿对照_2026-11-09.md。
## 组成：右上任务追踪条、J 任务面板、B 背包面板、奖励到账提示。

const C_FRAME := Color("2B3654")
const C_TITLE := Color("B08D57")
const C_TITLE_TEXT := Color("FFFAEB")
const C_PAPER := Color("F5ECD7")
const C_ROW := Color("FAF6E9")
const C_TAB := Color("EEE4CB")
const C_EDGE := Color("8A6D3B")
const C_EDGE_LIGHT := Color("C9B586")
const C_INK := Color("4A3823")
const C_MUTED := Color("78603C")
const C_DONE_BADGE := Color("B2A894")
const C_ACC := Color("C66E34")
const C_OK := Color("57965A")
const C_REWARD := Color("926814")
const C_DIM := Color("968A70")
const C_BAR := Color("7A5A30")
const C_BAR_TEXT := Color("FFE296")
const C_DETAIL := Color("F0E7D0")

const QUEST_TABS := [["main", "主线"], ["side", "支线"], ["daily", "日常"], ["sect", "宗派"]]
const CN_NUM := ["零", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"]
## UI 规范 §2.2 品质色（名称与物品格边框同色）。
const QUALITY_COLORS := {
	"凡品": Color("8E8B83"), "良品": Color("5E9C5F"), "精品": Color("4E86B8"),
	"极品": Color("9E63B8"), "完美": Color("D9A441"),
}
const STAT_NAME := {
	"hp": "气血", "mp": "法力", "patk": "物攻", "matk": "法攻",
	"pdef": "物防", "mdef": "法防", "speed": "速度",
}
## 品质共鸣用属性名（含概率项，数值规范 §1.2 万分制）。
const SUIT_ATTR_NAME := {
	"hp": "气血", "mp": "法力", "patk": "物攻", "matk": "法攻",
	"pdef": "物防", "mdef": "法防", "speed": "速度",
	"pcrit": "暴击", "mcrit": "法暴", "pcombo": "连击",
	"sealhit": "封印命中", "pdodge": "闪避",
}

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _bold: FontVariation

var _tracker: PanelContainer
var _tracker_title: Label
var _tracker_body: Label

var _panel: Control
var _panel_body: VBoxContainer
var _daily_label: Label
var _quest_tab := "main"
var _tab_buttons: Dictionary = {}

var _bag_panel: Control
var _bag_grid: GridContainer
var _bag_detail: VBoxContainer
var _bag_figure: TextureRect
var _bag_cap_label: Label
var _bag_gold_label: Label
var _bag_yuan_label: Label
var _bag_stats: Dictionary = {}
var _slot_panels: Dictionary = {}
var _attrs_note: Label
var _resonance_label: Label
var _selected_item := ""
var _bag_ops: Dictionary = {}
var _item_icons: Dictionary = {}

# 出售数量选择器（用户定案：定稿没有数量选择，但单卖 1 个不够用）。
var _sell_qty: int = 1
var _sell_qty_for: String = ""
var _sell_qty_label: Label
var _sell_gain_label: Label
var _sell_minus: Button
var _sell_plus: Button
var _sell_all: Button

var _toast: Label
var _toast_timer: float = 0.0

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	_bold = FontVariation.new()
	_bold.base_font = ThemeDB.fallback_font
	_bold.variation_embolden = 0.6
	_item_icons = _load_json("res://data/item_icons.json").get("icons", {})
	_build_tracker()
	_build_panel()
	_build_bag_panel()
	_build_toast()
	QuestStore.changed.connect(_refresh)
	QuestStore.reward_granted.connect(_on_reward)
	QuestStore.notified.connect(_show_toast)
	EquipStore.changed.connect(_on_equip_changed)
	SessionStore.session_changed.connect(_refresh_paperdoll)
	_refresh()

func _load_json(path: String) -> Dictionary:
	var text := FileAccess.get_file_as_string(path)
	var data: Variant = JSON.parse_string(text)
	return data if data is Dictionary else {}

func _process(delta: float) -> void:
	if _toast_timer > 0.0:
		_toast_timer -= delta
		if _toast_timer <= 0.0:
			_toast.visible = false

func _unhandled_input(event: InputEvent) -> void:
	if not (event is InputEventKey) or not event.pressed or event.echo:
		return
	match event.keycode:
		KEY_J:
			open_quest()
			get_viewport().set_input_as_handled()
		KEY_B:
			open_bag()
			get_viewport().set_input_as_handled()
		KEY_ESCAPE:
			if _panel.visible or _bag_panel.visible:
				close_panels()
				get_viewport().set_input_as_handled()

func open_quest() -> void:
	var show := not _panel.visible
	close_panels()
	_panel.visible = show
	if show:
		QuestStore.refresh()

func open_bag() -> void:
	var show := not _bag_panel.visible
	close_panels()
	_bag_panel.visible = show
	if show:
		QuestStore.refresh_bag()

func close_panels() -> void:
	_panel.visible = false
	_bag_panel.visible = false

# ---- 通用构件 ----

func _box(bg: Color, border: Color = Color(0, 0, 0, 0), width: int = 0, radius: int = 4,
		margin: int = 0) -> StyleBoxFlat:
	var sb := StyleBoxFlat.new()
	sb.bg_color = bg
	sb.corner_radius_top_left = radius
	sb.corner_radius_top_right = radius
	sb.corner_radius_bottom_left = radius
	sb.corner_radius_bottom_right = radius
	sb.border_width_left = width
	sb.border_width_right = width
	sb.border_width_top = width
	sb.border_width_bottom = width
	sb.border_color = border
	sb.content_margin_left = margin
	sb.content_margin_right = margin
	sb.content_margin_top = margin
	sb.content_margin_bottom = margin
	return sb

func _panel_of(sb: StyleBoxFlat) -> PanelContainer:
	var p := PanelContainer.new()
	p.add_theme_stylebox_override("panel", sb)
	return p

func _label(text: String, size: int, color: Color = C_INK, bold: bool = false) -> Label:
	var l := Label.new()
	l.text = text
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	if bold:
		l.add_theme_font_override("font", _bold)
	return l

func _badge(text: String, bg: Color, size: int = 12, min_w: int = 0) -> Label:
	var l := _label(text, size, C_TITLE_TEXT)
	l.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	l.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	l.add_theme_stylebox_override("normal", _box(bg, Color(0, 0, 0, 0), 0, 4, 4))
	if min_w > 0:
		l.custom_minimum_size = Vector2(min_w, 24)
	return l

func _button(text: String, primary: bool = false, disabled: bool = false) -> Button:
	var b := Button.new()
	b.text = text
	b.add_theme_font_size_override("font_size", 14)
	if primary:
		b.add_theme_stylebox_override("normal", _box(C_ACC, C_EDGE, 1, 4, 6))
		b.add_theme_stylebox_override("hover", _box(C_ACC.lightened(0.08), C_EDGE, 1, 4, 6))
		b.add_theme_stylebox_override("pressed", _box(Color("B34A14"), C_EDGE, 2, 4, 6))
		b.add_theme_color_override("font_color", C_TITLE_TEXT)
	else:
		b.add_theme_stylebox_override("normal", _box(C_TAB, C_EDGE, 2, 4, 6))
		b.add_theme_stylebox_override("hover", _box(C_TAB.lightened(0.05), C_EDGE, 2, 4, 6))
		b.add_theme_stylebox_override("pressed", _box(C_TAB.darkened(0.08), C_EDGE, 2, 4, 6))
		b.add_theme_color_override("font_color", C_INK)
	b.add_theme_stylebox_override("disabled", _box(C_TAB.darkened(0.06), C_EDGE_LIGHT, 1, 4, 6))
	b.add_theme_color_override("font_disabled_color", C_DIM)
	b.disabled = disabled
	return b

func _spacer() -> Control:
	var s := Control.new()
	s.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	return s

func _title_bar(title: String, on_back: Callable) -> PanelContainer:
	var bar := _panel_of(_box(C_TITLE, Color(0, 0, 0, 0), 0, 0, 0))
	bar.custom_minimum_size = Vector2(0, 44)
	var hb := HBoxContainer.new()
	bar.add_child(hb)
	var back := Button.new()
	back.text = "‹"
	back.flat = true
	back.custom_minimum_size = Vector2(44, 44)
	back.add_theme_font_size_override("font_size", 22)
	back.add_theme_color_override("font_color", C_TITLE_TEXT)
	back.pressed.connect(on_back)
	hb.add_child(back)
	var t := _label(title, 18, C_TITLE_TEXT, true)
	t.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	t.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	hb.add_child(t)
	var pad := Control.new()
	pad.custom_minimum_size = Vector2(44, 0)
	hb.add_child(pad)
	return bar

func _fullscreen_shell(title: String, on_back: Callable) -> Array:
	var root := Control.new()
	root.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	root.mouse_filter = Control.MOUSE_FILTER_STOP
	var frame := ColorRect.new()
	frame.color = C_FRAME
	frame.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	frame.mouse_filter = Control.MOUSE_FILTER_IGNORE
	root.add_child(frame)
	var paper := _panel_of(_box(C_PAPER, C_EDGE, 2, 8, 0))
	paper.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	paper.offset_left = 10
	paper.offset_top = 10
	paper.offset_right = -10
	paper.offset_bottom = -10
	root.add_child(paper)
	var vb := VBoxContainer.new()
	vb.add_theme_constant_override("separation", 0)
	paper.add_child(vb)
	vb.add_child(_title_bar(title, on_back))
	return [root, vb]

# ---- 追踪条 ----

func _build_tracker() -> void:
	_tracker = _panel_of(_box(Color(0.96, 0.93, 0.84, 0.92), C_EDGE, 2, 8, 10))
	_tracker.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(_tracker)
	_tracker.set_anchors_preset(Control.PRESET_TOP_RIGHT)
	_tracker.custom_minimum_size = Vector2(260, 0)
	_tracker.offset_left = -276
	_tracker.offset_top = 96
	_tracker.offset_right = -16
	var box := VBoxContainer.new()
	box.add_theme_constant_override("separation", 2)
	_tracker.add_child(box)
	_tracker_title = _label("任务", 15, C_ACC, true)
	box.add_child(_tracker_title)
	_tracker_body = _label("", 13, C_MUTED)
	_tracker_body.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	box.add_child(_tracker_body)
	_tracker.gui_input.connect(func(ev: InputEvent) -> void:
		if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
			close_panels()
			_panel.visible = true
			QuestStore.refresh())

# ---- 任务面板 ----

func _build_panel() -> void:
	var shell := _fullscreen_shell("任务", func() -> void: close_panels())
	_panel = shell[0]
	_panel.visible = false
	add_child(_panel)
	var vb: VBoxContainer = shell[1]
	var tab_row := HBoxContainer.new()
	tab_row.add_theme_constant_override("separation", 8)
	var wrap := _panel_of(_box(Color(0, 0, 0, 0), Color(0, 0, 0, 0), 0, 0, 12))
	wrap.add_child(tab_row)
	vb.add_child(wrap)
	for tab in QUEST_TABS:
		var b := _button(str(tab[1]))
		b.custom_minimum_size = Vector2(64, 30)
		b.pressed.connect(_on_tab.bind(str(tab[0])))
		_tab_buttons[str(tab[0])] = b
		tab_row.add_child(b)
	tab_row.add_child(_spacer())
	_daily_label = _label("", 12, C_MUTED)
	_daily_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	tab_row.add_child(_daily_label)
	var scroll := ScrollContainer.new()
	scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	vb.add_child(scroll)
	var margin := _panel_of(_box(Color(0, 0, 0, 0), Color(0, 0, 0, 0), 0, 0, 12))
	margin.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	scroll.add_child(margin)
	_panel_body = VBoxContainer.new()
	_panel_body.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_panel_body.add_theme_constant_override("separation", 2)
	margin.add_child(_panel_body)
	_style_tabs()

func _on_tab(tab: String) -> void:
	_quest_tab = tab
	_style_tabs()
	_refresh_panel()

func _style_tabs() -> void:
	for key in _tab_buttons:
		var b: Button = _tab_buttons[key]
		var sel: bool = key == _quest_tab
		b.add_theme_stylebox_override("normal", _box(C_TITLE if sel else C_TAB, C_EDGE, 2, 4, 6))
		b.add_theme_stylebox_override("hover", _box((C_TITLE if sel else C_TAB).lightened(0.06), C_EDGE, 2, 4, 6))
		b.add_theme_color_override("font_color", C_TITLE_TEXT if sel else C_INK)

func _tab_quests() -> Array:
	var out: Array = []
	for q in QuestStore.quests:
		var t := str(q.get("type", ""))
		var hit := false
		match _quest_tab:
			"main": hit = t == "main"
			"side": hit = t == "side"
			"daily": hit = t == "daily_normal" or t == "daily_dungeon"
			"sect": hit = t == "daily_sect"
		if hit:
			out.append(q)
	return out

func _refresh_panel() -> void:
	_daily_label.text = _daily_text()
	for c in _panel_body.get_children():
		c.queue_free()
	var list := _tab_quests()
	if list.is_empty():
		var empty := _label("暂无%s任务" % _tab_name(_quest_tab), 14, C_MUTED)
		empty.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
		_panel_body.add_child(empty)
		return
	var i := 0
	for q in list:
		_panel_body.add_child(_quest_row(q, i % 2 == 0))
		i += 1

func _tab_name(tab: String) -> String:
	for t in QUEST_TABS:
		if str(t[0]) == tab:
			return str(t[1])
	return tab

func _daily_text() -> String:
	var parts: Array = []
	for cat in ["normal", "sect", "dungeon"]:
		var d: Dictionary = QuestStore.daily.get(cat, {})
		parts.append("%s %d/%d" % [str(QuestStore.CATEGORY_NAME.get(cat, cat)),
			int(d.get("used", 0)), int(d.get("limit", 0))])
	return "   ".join(parts)

func _chapter_badge(q: Dictionary) -> String:
	var t := str(q.get("type", ""))
	match t:
		"main":
			var ch := int(q.get("chapter", 0))
			if ch <= 0:
				return "序章"
			return "第%s章" % str(CN_NUM[ch] if ch < CN_NUM.size() else str(ch))
		"side":
			return "支线"
		"daily_sect":
			return "宗派"
		"daily_dungeon":
			return "副本"
	return "日常"

func _quest_row(q: Dictionary, light: bool) -> Control:
	var st := str(q.get("state", ""))
	var done := st == "done"
	var row := _panel_of(_box(C_ROW if light else C_PAPER, Color(0, 0, 0, 0), 0, 4, 10))
	row.mouse_filter = Control.MOUSE_FILTER_STOP
	var hb := HBoxContainer.new()
	hb.add_theme_constant_override("separation", 12)
	row.add_child(hb)
	hb.add_child(_badge(_chapter_badge(q), C_DONE_BADGE if done else C_MUTED, 12, 64))
	var left := VBoxContainer.new()
	left.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	left.size_flags_stretch_ratio = 3.0
	left.add_theme_constant_override("separation", 2)
	hb.add_child(left)
	var name_row := HBoxContainer.new()
	name_row.add_theme_constant_override("separation", 8)
	left.add_child(name_row)
	name_row.add_child(_label(str(q.get("name", "")), 15, C_DIM if done else C_INK, true))
	if str(QuestStore.tracked().get("id", "")) == str(q.get("id", "")) and not done:
		name_row.add_child(_badge("追踪中", C_ACC, 11))
	var desc := _label(str(q.get("desc", "")), 12, C_DIM if done else C_MUTED)
	desc.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	desc.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	left.add_child(desc)
	if st == "locked":
		left.add_child(_label(str(q.get("reason", "未满足条件")), 12, C_MUTED))
	var mid := VBoxContainer.new()
	mid.custom_minimum_size = Vector2(300, 0)
	mid.add_theme_constant_override("separation", 2)
	hb.add_child(mid)
	mid.add_child(_label(_progress_text(q), 14, C_DIM if done else C_INK, true))
	mid.add_child(_label(_reward_text(q), 12, C_DIM if done else C_REWARD))
	hb.add_child(_spacer())
	var right := VBoxContainer.new()
	right.custom_minimum_size = Vector2(96, 0)
	right.alignment = BoxContainer.ALIGNMENT_CENTER
	hb.add_child(right)
	match st:
		"done":
			right.add_child(_label("✓ 已完成", 14, C_OK, true))
		"available":
			var b := _button("接取", true)
			b.pressed.connect(func() -> void: QuestStore.accept(str(q.get("id", ""))))
			right.add_child(b)
		"active":
			var b := _button("前往", true)
			b.pressed.connect(_on_go.bind(q))
			right.add_child(b)
		"completable":
			var b := _button("交付", true)
			b.pressed.connect(func() -> void: QuestStore.submit(str(q.get("id", ""))))
			right.add_child(b)
		_:
			var b := _button("未开放", false, true)
			b.tooltip_text = str(q.get("reason", "未满足条件"))
			right.add_child(b)
	row.gui_input.connect(func(ev: InputEvent) -> void:
		if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
			if st == "active" or st == "completable" or st == "available":
				QuestStore.tracked_override = str(q.get("id", ""))
				_refresh())
	return row

func _progress_text(q: Dictionary) -> String:
	var src := q
	# 服务端 11/1 对 done 行 objectives 归零；done 状态本身即「目标全部达成」的证明，按 need 显示。
	if str(q.get("state", "")) == "done":
		src = q.duplicate(true)
		for o in src.get("objectives", []):
			o["cur"] = o.get("need", 0)
	var objs: Array = src.get("objectives", [])
	if objs.size() == 1:
		var o: Dictionary = objs[0]
		return "进度 %d / %d" % [int(o.get("cur", 0)), int(o.get("need", 0))]
	return "进度 " + QuestStore.progress_text(src)

func _fmt(n: int) -> String:
	var s := str(n)
	var out := ""
	while s.length() > 3:
		out = "," + s.right(3) + out
		s = s.left(s.length() - 3)
	return s + out

func _reward_text(q: Dictionary) -> String:
	var r: Dictionary = q.get("rewards", {})
	var parts: Array = []
	if int(r.get("exp", 0)) > 0:
		parts.append("经验 +%s" % _fmt(int(r.get("exp", 0))))
	if int(r.get("xiuwei", 0)) > 0:
		parts.append("修为 +%s" % _fmt(int(r.get("xiuwei", 0))))
	if int(r.get("gold", 0)) > 0:
		parts.append("金币 +%s" % _fmt(int(r.get("gold", 0))))
	for it in r.get("items", []):
		var info: Dictionary = it
		parts.append("%s ×%d" % [str(info.get("name", "")), int(info.get("count", 0))])
	return " · ".join(parts) if not parts.is_empty() else "无奖励"

## 前往：追踪该任务并自动走到 giver NPC 交互距离内（复用点按沿路径段移动）。
func _on_go(q: Dictionary) -> void:
	QuestStore.tracked_override = str(q.get("id", ""))
	close_panels()
	_refresh()
	var giver: Dictionary = q.get("giver", {})
	var map_id := str(giver.get("map_id", ""))
	if map_id != "" and map_id != WorldStore.current_map:
		_show_toast("目标在%s，请先前往该地图" % map_id)
		return
	var npc_id := str(giver.get("npc_id", ""))
	for n in get_tree().get_nodes_in_group("npc"):
		if str(n.get("npc_id")) != npc_id:
			continue
		var player := get_tree().get_first_node_in_group("player") as Node2D
		if player == null:
			return
		var side := -1.0 if player.position.x < n.position.x else 1.0
		player.call("walk_to", n.position.x + side * 70.0)
		return
	_show_toast("目标 NPC 不在当前场景")

# ---- 背包面板 ----

func _build_bag_panel() -> void:
	var shell := _fullscreen_shell("物品", func() -> void: close_panels())
	_bag_panel = shell[0]
	_bag_panel.visible = false
	add_child(_bag_panel)
	var vb: VBoxContainer = shell[1]
	var content := HBoxContainer.new()
	content.size_flags_vertical = Control.SIZE_EXPAND_FILL
	content.add_theme_constant_override("separation", 16)
	var wrap := _panel_of(_box(Color(0, 0, 0, 0), Color(0, 0, 0, 0), 0, 0, 12))
	wrap.size_flags_vertical = Control.SIZE_EXPAND_FILL
	wrap.add_child(content)
	vb.add_child(wrap)
	content.add_child(_build_bag_left())
	content.add_child(_build_bag_right())
	var bar := _panel_of(_box(C_BAR, Color(0, 0, 0, 0), 0, 0, 16))
	bar.custom_minimum_size = Vector2(0, 36)
	vb.add_child(bar)
	var hb := HBoxContainer.new()
	bar.add_child(hb)
	_bag_gold_label = _label("金币：0", 16, C_BAR_TEXT, true)
	hb.add_child(_bag_gold_label)
	hb.add_child(_spacer())
	_bag_yuan_label = _label("元宝：0", 16, C_BAR_TEXT, true)
	hb.add_child(_bag_yuan_label)

func _build_bag_left() -> Control:
	var left := VBoxContainer.new()
	left.custom_minimum_size = Vector2(484, 0)
	left.add_theme_constant_override("separation", 8)
	left.add_child(_build_paperdoll())
	var tabs := HBoxContainer.new()
	tabs.add_theme_constant_override("separation", 8)
	left.add_child(tabs)
	var bag_tab := _button("背包")
	bag_tab.custom_minimum_size = Vector2(64, 30)
	bag_tab.add_theme_stylebox_override("normal", _box(C_TITLE, C_EDGE, 2, 4, 6))
	bag_tab.add_theme_color_override("font_color", C_TITLE_TEXT)
	tabs.add_child(bag_tab)
	for i in range(1, 5):
		var b := _button("仓库%d" % i, false, true)
		b.custom_minimum_size = Vector2(72, 30)
		b.tooltip_text = "仓库需消耗金元宝解锁（后续版本开放）"
		tabs.add_child(b)
	var scroll := ScrollContainer.new()
	scroll.custom_minimum_size = Vector2(0, 92)
	scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	left.add_child(scroll)
	_bag_grid = GridContainer.new()
	_bag_grid.columns = 10
	_bag_grid.add_theme_constant_override("h_separation", 4)
	_bag_grid.add_theme_constant_override("v_separation", 4)
	scroll.add_child(_bag_grid)
	var ops := HBoxContainer.new()
	ops.add_theme_constant_override("separation", 8)
	left.add_child(ops)
	var sort_btn := _button("整理")
	sort_btn.custom_minimum_size = Vector2(72, 30)
	sort_btn.tooltip_text = "按品质 / 类别整理背包"
	sort_btn.pressed.connect(_on_bag_sort)
	ops.add_child(sort_btn)
	_bag_ops["sort"] = sort_btn
	var sell_btn := _button("出售")
	sell_btn.custom_minimum_size = Vector2(72, 30)
	sell_btn.tooltip_text = "出售选中的物品（每次 1 个）"
	sell_btn.pressed.connect(_on_bag_sell)
	ops.add_child(sell_btn)
	_bag_ops["sell"] = sell_btn
	var lock_btn := _button("锁定")
	lock_btn.custom_minimum_size = Vector2(72, 30)
	lock_btn.tooltip_text = "锁定 / 解锁选中物品（锁定后禁止使用与出售）"
	lock_btn.pressed.connect(_on_bag_lock)
	ops.add_child(lock_btn)
	_bag_ops["lock"] = lock_btn
	ops.add_child(_spacer())
	_bag_cap_label = _label("容量 0/0", 14, C_MUTED, true)
	_bag_cap_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	ops.add_child(_bag_cap_label)
	return left

func _slot(x: float, y: float, tooltip: String) -> PanelContainer:
	var s := _panel_of(_box(C_TAB, C_EDGE_LIGHT, 2, 6, 0))
	s.custom_minimum_size = Vector2(48, 48)
	s.position = Vector2(x, y)
	s.tooltip_text = tooltip
	s.mouse_filter = Control.MOUSE_FILTER_STOP
	return s

func _build_paperdoll() -> Control:
	var stage := Control.new()
	stage.custom_minimum_size = Vector2(484, 320)
	stage.mouse_filter = Control.MOUSE_FILTER_IGNORE
	var figure := TextureRect.new()
	figure.texture = _paperdoll_texture()
	figure.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	figure.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
	figure.position = Vector2(167, 60)
	figure.size = Vector2(150, 180)
	figure.mouse_filter = Control.MOUSE_FILTER_IGNORE
	stage.add_child(figure)
	_bag_figure = figure
	for slot in [["hat", 218, 8, "帽子"], ["clothes", 62, 60, "衣服"], ["necklace", 374, 60, "项链"],
			["weapon", 62, 150, "武器"], ["boots", 374, 150, "鞋子"]]:
		var sp := _slot(float(slot[1]), float(slot[2]), "%s槽：未穿戴" % str(slot[3]))
		sp.gui_input.connect(_on_slot_pressed.bind(str(slot[0])))
		_slot_panels[str(slot[0])] = sp
		stage.add_child(sp)
	for i in range(3):
		stage.add_child(_slot(122 + i * 96, 252, "法宝槽：装备系统未开放（M2c）"))
		var lab := _label(["攻", "防", "辅"][i], 12, C_MUTED)
		lab.position = Vector2(140 + i * 96, 302)
		stage.add_child(lab)
	return stage

func _on_slot_pressed(ev: InputEvent, slot: String) -> void:
	if not (ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT):
		return
	var info: Dictionary = EquipStore.slots.get(slot, {})
	if info.is_empty():
		return
	_selected_item = str(info.get("id", ""))
	_refresh_bag()

## 站台槽渲染已穿戴装备（图标 + 品质底色，与背包格一致）；空槽保持纸底。
func _refresh_equips() -> void:
	for slot in _slot_panels:
		var sp: PanelContainer = _slot_panels[slot]
		for c in sp.get_children():
			c.queue_free()
		var info: Dictionary = EquipStore.slots.get(slot, {})
		if info.is_empty():
			sp.add_theme_stylebox_override("panel", _box(C_TAB, C_EDGE_LIGHT, 2, 6, 0))
			sp.tooltip_text = "%s槽：未穿戴" % slot
			continue
		var qc := _quality_color(info)
		sp.add_theme_stylebox_override("panel", _box(qc, qc, 2, 6, 0))
		sp.tooltip_text = "%s（点击查看详情/卸下）" % str(info.get("name", ""))
		var tex := _item_icon(str(info.get("id", "")))
		if tex != null:
			var icon := TextureRect.new()
			icon.texture = tex
			icon.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
			icon.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
			icon.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
			icon.offset_left = 4
			icon.offset_top = 4
			icon.offset_right = -4
			icon.offset_bottom = -4
			icon.mouse_filter = Control.MOUSE_FILTER_IGNORE
			sp.add_child(icon)

func _paperdoll_texture() -> Texture2D:
	var key := SessionStore.faction + SessionStore.gender
	var path := "res://assets/sprites/walk/%s/walk_1.png" % key
	if ResourceLoader.exists(path):
		return load(path)
	return load("res://assets/sprites/player/idle_0.png")

## 登录/换角回填门派性别后重建站台皮肤（面板构建早于登录回包）。
func _refresh_paperdoll() -> void:
	if _bag_figure != null:
		_bag_figure.texture = _paperdoll_texture()

func _build_bag_right() -> Control:
	var right := VBoxContainer.new()
	right.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	right.add_theme_constant_override("separation", 8)
	var stats := HBoxContainer.new()
	stats.add_theme_constant_override("separation", 48)
	right.add_child(stats)
	var cols := [
		[["level", "等级"], ["hp", "气血"], ["patk", "物攻"], ["pdef", "物防"], ["speed", "速度"], ["dodge", "闪避"], ["combo", "连击"]],
		[["faction", "门派"], ["mp", "法力"], ["matk", "法攻"], ["mdef", "法防"], ["hit", "命中"], ["crit", "暴击"], ["resist", "抗封印"]],
	]
	for col in cols:
		var vb := VBoxContainer.new()
		vb.add_theme_constant_override("separation", 4)
		stats.add_child(vb)
		for row in col:
			var hb := HBoxContainer.new()
			hb.add_theme_constant_override("separation", 8)
			vb.add_child(hb)
			var name_lab := _label("%s：" % str(row[1]), 14, C_MUTED)
			name_lab.custom_minimum_size = Vector2(70, 0)
			hb.add_child(name_lab)
			var val := _label("—", 14, C_INK, true)
			_bag_stats[str(row[0])] = val
			hb.add_child(val)
	_resonance_label = _label("品质共鸣： 未激活 （同品质 3 件起）", 13, C_MUTED)
	right.add_child(_resonance_label)
	_attrs_note = _label("属性数值待服务端下发（M2c）", 11, C_MUTED)
	right.add_child(_attrs_note)
	var detail := _panel_of(_box(C_DETAIL, C_EDGE, 2, 6, 12))
	detail.size_flags_vertical = Control.SIZE_EXPAND_FILL
	right.add_child(detail)
	_bag_detail = VBoxContainer.new()
	_bag_detail.add_theme_constant_override("separation", 6)
	detail.add_child(_bag_detail)
	return right

func _item_icon(id: String) -> Texture2D:
	var path := str(_item_icons.get(id, ""))
	if path != "" and ResourceLoader.exists(path):
		return load(path)
	return null

func _quality_color(info: Dictionary) -> Color:
	return QUALITY_COLORS.get(str(info.get("quality", "凡品")), C_EDGE)

func _refresh_bag() -> void:
	var wallet: Dictionary = QuestStore.wallet
	_bag_gold_label.text = "金币：%s" % _fmt(int(wallet.get("gold", 0)))
	_bag_yuan_label.text = "元宝：%s" % _fmt(int(wallet.get("yuanbao", 0)))
	_bag_cap_label.text = "容量 %d/%d" % [int(QuestStore.bag.get("used", 0)), int(QuestStore.bag.get("capacity", 0))]
	_bag_stats["level"].text = str(QuestStore.level)
	_bag_stats["faction"].text = SessionStore.faction
	_refresh_attrs()
	_refresh_equips()
	for c in _bag_grid.get_children():
		c.queue_free()
	var items: Array = QuestStore.bag.get("items", [])
	var cap := int(QuestStore.bag.get("capacity", 0))
	for i in range(cap):
		var info: Dictionary = items[i] if i < items.size() else {}
		_bag_grid.add_child(_bag_cell(info))
	if _selected_item != "":
		var found := false
		for it in items:
			if str(it.get("id", "")) == _selected_item:
				found = true
		if not found and _selected_item not in EquipStore.equipped_ids():
			_selected_item = ""
	_refresh_detail()

func _on_equip_changed() -> void:
	if _bag_panel.visible:
		_refresh_bag()

## 属性两列读服务端视图（基线+装备加成）；万分制字段换算百分比（UI §3）。
func _refresh_attrs() -> void:
	var a: Dictionary = EquipStore.attrs
	_attrs_note.visible = a.is_empty()
	for key in _bag_stats:
		if key == "level" or key == "faction":
			continue
		var lab: Label = _bag_stats[key]
		if not a.has(key):
			lab.text = "—"
			continue
		var v := int(a[key])
		if key in ["crit", "combo", "resist"]:
			lab.text = "%.1f%%" % (float(v) / 100.0)
		else:
			lab.text = _fmt(v)
	_refresh_resonance()

## 品质共鸣行：文案直接取服务端 suit 明细，客户端不重算规则（V1.29 配置表 §4）。
func _refresh_resonance() -> void:
	var info: Dictionary = EquipStore.suit
	if int(info.get("tier", 0)) <= 0:
		var best := _best_quality_count()
		_resonance_label.text = ("品质共鸣： 未激活 （同品质 %d/%d 件）" % [best, 3]) if best > 0 \
			else "品质共鸣： 未激活 （同品质 3 件起）"
		return
	var parts: Array = []
	if float(info.get("all_pct", 0)) > 0:
		parts.append("全属性 +%s%%" % _pct_txt(info.get("all_pct", 0)))
	var mains: Array = info.get("main_attrs", [])
	var main_pct := float(info.get("main_pct", 0))
	if main_pct > 0 and mains.size() > 0:
		var names: Array = []
		for k in mains:
			names.append(_suit_attr(str(k)))
		parts.append("%s +%s%%" % ["/".join(names), _pct_txt(main_pct)])
	# 副项与门派微调同属性时合并展示（剑宗暴击 3.5+3=6.5、符观封印命中 7+3=10，配置表 §4 口径）。
	var extras: Dictionary = {}
	var sub := str(info.get("sub_attr", ""))
	if sub != "" and float(info.get("sub_pct", 0)) > 0:
		extras[sub] = float(info.get("sub_pct", 0))
	var tweak := str(info.get("tweak_attr", ""))
	if tweak != "" and float(info.get("tweak_pct", 0)) > 0:
		extras[tweak] = float(extras.get(tweak, 0.0)) + float(info.get("tweak_pct", 0))
	for k in extras:
		parts.append("%s +%s%%" % [_suit_attr(str(k)), _pct_txt(extras[k])])
	var tier_txt := "一档" if int(info.get("tier", 0)) == 1 else "二档"
	_resonance_label.text = "品质共鸣： %s · %s %d 件 ｜ %s" % [
		tier_txt, str(info.get("quality", "")), int(info.get("count", 0)), "  ".join(parts)]

## 未激活时给出「还差几件」的提示。
func _best_quality_count() -> int:
	var counts: Dictionary = {}
	for slot in EquipStore.slots:
		var q := str((EquipStore.slots[slot] as Dictionary).get("quality", "凡品"))
		counts[q] = int(counts.get(q, 0)) + 1
	var best := 0
	for q in counts:
		best = maxi(best, int(counts[q]))
	return best

func _suit_attr(key: String) -> String:
	return str(SUIT_ATTR_NAME.get(key, key))

func _pct_txt(v: Variant) -> String:
	var f := float(v)
	if is_equal_approx(f, round(f)):
		return "%d" % int(round(f))
	return "%.1f" % f

func _bag_cell(info: Dictionary) -> Control:
	var filled := not info.is_empty()
	var id := str(info.get("id", ""))
	var selected := filled and id == _selected_item
	# 用 Panel 而非 PanelContainer：容器会重排子控件，数量角标的锚点定位会失效。
	# 格底按品质色填充（用户定案 2026-11-10，覆盖定稿纸底）；空格保持纸底。
	var cell := Panel.new()
	cell.add_theme_stylebox_override("panel", _box(
		_quality_color(info) if filled else C_TAB,
		C_ACC if selected else (_quality_color(info) if filled else C_EDGE_LIGHT), 3 if selected else 2, 6, 0))
	cell.custom_minimum_size = Vector2(44, 44)
	cell.mouse_filter = Control.MOUSE_FILTER_STOP
	if filled:
		cell.tooltip_text = "%s ×%d" % [str(info.get("name", id)), int(info.get("count", 0))]
		var tex := _item_icon(id)
		if tex != null:
			var icon := TextureRect.new()
			icon.texture = tex
			icon.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
			icon.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
			icon.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
			icon.offset_left = 4
			icon.offset_top = 4
			icon.offset_right = -4
			icon.offset_bottom = -4
			icon.mouse_filter = Control.MOUSE_FILTER_IGNORE
			cell.add_child(icon)
		else:
			var lab := _label(str(info.get("category", "其他")).left(1), 16, C_MUTED)
			lab.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
			lab.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
			lab.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
			lab.mouse_filter = Control.MOUSE_FILTER_IGNORE
			cell.add_child(lab)
		var count := int(info.get("count", 0))
		if count > 1:
			var badge := _label(str(count), 10, C_TITLE_TEXT)
			badge.add_theme_stylebox_override("normal", _box(Color(0.23, 0.17, 0.1, 0.8), Color(0, 0, 0, 0), 0, 3, 2))
			badge.set_anchors_preset(Control.PRESET_BOTTOM_RIGHT)
			badge.offset_left = -20
			badge.offset_top = -16
			badge.offset_right = -2
			badge.offset_bottom = -2
			badge.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
			badge.mouse_filter = Control.MOUSE_FILTER_IGNORE
			cell.add_child(badge)
		if bool(info.get("locked", false)):
			var lock_badge := _label("锁", 10, C_ACC)
			lock_badge.add_theme_stylebox_override("normal", _box(Color(1, 1, 1, 0.88), C_ACC, 1, 3, 2))
			lock_badge.set_anchors_preset(Control.PRESET_TOP_RIGHT)
			lock_badge.offset_left = -18
			lock_badge.offset_top = 2
			lock_badge.offset_right = -2
			lock_badge.offset_bottom = 16
			lock_badge.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
			lock_badge.mouse_filter = Control.MOUSE_FILTER_IGNORE
			cell.add_child(lock_badge)
		cell.gui_input.connect(func(ev: InputEvent) -> void:
			if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
				_selected_item = id
				_refresh_bag())
	else:
		cell.mouse_filter = Control.MOUSE_FILTER_IGNORE
	return cell

func _refresh_detail() -> void:
	for c in _bag_detail.get_children():
		c.queue_free()
	# 详情卡每次重建，旧控件引用必须清空，避免 _set_sell_qty 操作已释放节点。
	_sell_qty_label = null
	_sell_gain_label = null
	_sell_minus = null
	_sell_plus = null
	_sell_all = null
	var info: Dictionary = {}
	for it in QuestStore.bag.get("items", []):
		if str(it.get("id", "")) == _selected_item:
			info = it
	var equipped_slot := EquipStore.slot_of_item(_selected_item)
	if info.is_empty() and equipped_slot != "":
		info = (EquipStore.slots[equipped_slot] as Dictionary).duplicate()
		info["slot"] = equipped_slot
		info["count"] = 1
	if info.is_empty():
		_bag_detail.add_child(_label("选中左侧物品查看详情", 14, C_MUTED))
		return
	var head := HBoxContainer.new()
	head.add_theme_constant_override("separation", 12)
	_bag_detail.add_child(head)
	var icon_wrap := _panel_of(_box(C_TAB, C_EDGE, 2, 6, 4))
	icon_wrap.custom_minimum_size = Vector2(64, 64)
	head.add_child(icon_wrap)
	var tex := _item_icon(_selected_item)
	if tex != null:
		var icon := TextureRect.new()
		icon.texture = tex
		icon.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
		icon.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
		icon_wrap.add_child(icon)
	var titles := VBoxContainer.new()
	titles.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	titles.add_theme_constant_override("separation", 2)
	head.add_child(titles)
	titles.add_child(_label(str(info.get("name", _selected_item)), 18, _quality_color(info), true))
	titles.add_child(_label("%s · 单格上限 %d" % [str(info.get("category", "其他")), int(info.get("stack", 1))], 13, C_MUTED))
	var sep := HSeparator.new()
	sep.add_theme_constant_override("separation", 8)
	_bag_detail.add_child(sep)
	_bag_detail.add_child(_label("数量    ×%d" % int(info.get("count", 0)), 14, C_INK, true))
	if info.has("sell"):
		var sell := int(info.get("sell", 0))
		_bag_detail.add_child(_label("售价    %s" % (("%d 金币" % sell) if sell > 0 else "不可出售"), 14, C_INK))
		if sell > 0 and not bool(info.get("locked", false)):
			var cap := maxi(int(info.get("count", 1)), 1)
			# 换选中目标时数量回到 1，别把上个物品的份数带过来。
			if _sell_qty_for != _selected_item:
				_sell_qty_for = _selected_item
				_sell_qty = 1
			_sell_qty = clampi(_sell_qty, 1, cap)
			_bag_detail.add_child(_build_sell_qty_row(sell, cap))
	if bool(info.get("locked", false)):
		_bag_detail.add_child(_label("状态    已锁定（禁止使用 / 出售）", 13, C_ACC))
	var category := str(info.get("category", ""))
	if category == "" and info.has("slot"):
		category = "装备 · %s" % str(info.get("slot", ""))
	_bag_detail.add_child(_label("类别    %s" % (category if category != "" else "其他"), 14, C_INK))
	var stats: Dictionary = info.get("stats", {})
	if not stats.is_empty():
		var parts: Array = []
		for k in stats:
			parts.append("%s +%d" % [str(STAT_NAME.get(k, k)), int(stats[k])])
		_bag_detail.add_child(_label("加成    %s" % " · ".join(parts), 13, C_INK))
	var desc := str(info.get("desc", ""))
	if desc != "":
		var d := _label(desc, 12, C_MUTED)
		d.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
		d.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		_bag_detail.add_child(d)
	var act := HBoxContainer.new()
	act.add_theme_constant_override("separation", 8)
	act.add_child(_spacer())
	if equipped_slot != "":
		var rm := _button("卸下", false)
		rm.custom_minimum_size = Vector2(88, 30)
		rm.pressed.connect(_remove_selected.bind(equipped_slot))
		act.add_child(rm)
	elif str(info.get("slot", "")) != "":
		var wear := _button("穿戴", true)
		wear.custom_minimum_size = Vector2(88, 30)
		wear.pressed.connect(_wear_selected.bind(_selected_item))
		act.add_child(wear)
	if info.get("use") != null:
		var use_btn := _button("使用", true)
		use_btn.custom_minimum_size = Vector2(88, 30)
		use_btn.pressed.connect(_use_selected.bind(_selected_item))
		act.add_child(use_btn)
	if act.get_child_count() > 1:
		_bag_detail.add_child(act)

## 出售数量行：－ / 数量 / ＋ / 全部 + 可得金币。
## 上限取「该格数量」而非同名物品总和——服务端 SellItem 只从第一格扣。
func _build_sell_qty_row(unit: int, cap: int) -> Control:
	var h := HBoxContainer.new()
	h.add_theme_constant_override("separation", 6)
	h.add_child(_label("出售数量", 14, C_INK))
	_sell_minus = _button("－")
	_sell_minus.custom_minimum_size = Vector2(28, 26)
	_sell_minus.pressed.connect(func() -> void: _set_sell_qty(_sell_qty - 1))
	h.add_child(_sell_minus)
	_sell_qty_label = _label(str(_sell_qty), 14, C_INK, true)
	_sell_qty_label.custom_minimum_size = Vector2(36, 26)
	_sell_qty_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_sell_qty_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	h.add_child(_sell_qty_label)
	_sell_plus = _button("＋")
	_sell_plus.custom_minimum_size = Vector2(28, 26)
	_sell_plus.pressed.connect(func() -> void: _set_sell_qty(_sell_qty + 1))
	h.add_child(_sell_plus)
	_sell_all = _button("全部", false)
	_sell_all.custom_minimum_size = Vector2(52, 26)
	_sell_all.pressed.connect(func() -> void: _set_sell_qty(cap))
	h.add_child(_sell_all)
	_sell_gain_label = _label("可得 %d 金币" % (_sell_qty * unit), 13, C_ACC)
	_sell_gain_label.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_sell_gain_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_RIGHT
	_sell_gain_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	h.add_child(_sell_gain_label)
	_sync_sell_qty_ui(unit, cap)
	return h

func _set_sell_qty(n: int) -> void:
	var info := _selected_bag_info()
	var cap := maxi(int(info.get("count", 1)), 1)
	_sell_qty = clampi(n, 1, cap)
	_sync_sell_qty_ui(int(info.get("sell", 0)), cap)

func _sync_sell_qty_ui(unit: int, cap: int) -> void:
	if _sell_qty_label == null or not is_instance_valid(_sell_qty_label):
		return
	_sell_qty_label.text = str(_sell_qty)
	_sell_gain_label.text = "可得 %d 金币" % (_sell_qty * unit)
	_sell_minus.disabled = _sell_qty <= 1
	_sell_plus.disabled = _sell_qty >= cap
	_sell_all.disabled = _sell_qty >= cap

func _wear_selected(id: String) -> void:
	await EquipStore.wear(id)
	await QuestStore.refresh_bag()

func _remove_selected(slot: String) -> void:
	await EquipStore.remove(slot)

func _use_selected(id: String) -> void:
	await QuestStore.use_item(id)

## 选中物品在背包中的那条记录（已穿戴装备不在 bag 内，返回空）。
func _selected_bag_info() -> Dictionary:
	for it in QuestStore.bag.get("items", []):
		if str(it.get("id", "")) == _selected_item:
			return it
	return {}

func _on_bag_sort() -> void:
	await QuestStore.sort_bag()

func _on_bag_sell() -> void:
	if _selected_item == "":
		_show_toast("先选中要出售的物品")
		return
	var info := _selected_bag_info()
	if info.is_empty():
		_show_toast("该物品不在背包中")
		return
	if bool(info.get("locked", false)):
		_show_toast("物品已锁定，先解锁再出售")
		return
	if int(info.get("sell", 0)) <= 0:
		_show_toast("该物品不可出售")
		return
	# 按选择器数量出售；服务端只从第一格扣，故上限就是该格数量。
	var qty := clampi(_sell_qty, 1, maxi(int(info.get("count", 1)), 1))
	var err := await QuestStore.sell_item(_selected_item, qty)
	if err == "":
		_sell_qty = 1

func _on_bag_lock() -> void:
	if _selected_item == "":
		_show_toast("先选中要锁定的物品")
		return
	var info := _selected_bag_info()
	if info.is_empty():
		_show_toast("该物品不在背包中")
		return
	await QuestStore.toggle_lock(_selected_item, not bool(info.get("locked", false)))

# ---- 提示 ----

func _build_toast() -> void:
	_toast = _label("", 15, C_TITLE_TEXT, true)
	# 不透明深褐药丸：原先 0.88 透明度会让底下的属性列/页签文字透出来，看着像串层。
	_toast.add_theme_stylebox_override("normal", _box(Color(0.23, 0.17, 0.1, 1.0), C_EDGE, 1, 6, 12))
	_toast.visible = false
	_toast.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_toast.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	_toast.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(_toast)
	_toast.set_anchors_preset(Control.PRESET_CENTER_TOP)

func _show_toast(msg: String) -> void:
	if msg == "" or not Settings.notify:
		return
	_toast.text = msg
	# 宽度随文案自适应：原固定 440px 会把「已锁定」三个字铺成一条、压住整行属性。
	var font: Font = _toast.get_theme_font("font")
	var fs: int = _toast.get_theme_font_size("font_size")
	var w: float = font.get_string_size(msg, HORIZONTAL_ALIGNMENT_LEFT, -1, fs).x + 36.0
	w = clampf(w, 96.0, 560.0)
	_toast.offset_left = -w * 0.5
	_toast.offset_right = w * 0.5
	_toast.offset_top = 54.0
	_toast.offset_bottom = 88.0
	_toast.visible = true
	_toast_timer = 2.6

func _on_reward(rewards: Dictionary) -> void:
	var parts: Array = []
	if int(rewards.get("exp", 0)) > 0:
		parts.append("经验 +%d" % int(rewards.get("exp", 0)))
	if int(rewards.get("xiuwei", 0)) > 0:
		parts.append("修为 +%d" % int(rewards.get("xiuwei", 0)))
	if int(rewards.get("gold", 0)) > 0:
		parts.append("金币 +%d" % int(rewards.get("gold", 0)))
	for it in rewards.get("items", []):
		var info: Dictionary = it
		parts.append("%s ×%d" % [str(info.get("name", info.get("id", ""))), int(info.get("count", 0))])
	if parts.is_empty():
		return
	_show_toast("任务完成：" + "，".join(parts))

# ---- 刷新 ----

func _refresh() -> void:
	_refresh_tracker()
	if _panel.visible:
		_refresh_panel()
	if _bag_panel.visible:
		_refresh_bag()

func _refresh_tracker() -> void:
	var q := QuestStore.tracked()
	if q.is_empty():
		_tracker_title.text = "任务"
		_tracker_body.text = "暂无进行中的任务（按 J 查看/接取）"
		return
	var st := str(q.get("state", ""))
	_tracker_title.text = "%s · %s" % [str(q.get("name", "")), str(QuestStore.STATE_TEXT.get(st, st))]
	_tracker_body.text = QuestStore.progress_text(q)
	_tracker_title.add_theme_color_override("font_color", C_OK if st == "completable" else C_ACC)