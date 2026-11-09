extends Control
## UI 层 · 任务与背包（GDD §9 / 数据全部来自 QuestStore，服务端权威）。
## 组成：右上任务追踪条（默认常驻）、J 任务面板、B 背包面板、奖励到账提示。

const COLOR_INK := Color("3b2e1e")
const COLOR_MUTED := Color("6b5a44")
const COLOR_ACCENT := Color("B34A14")
const COLOR_OK := Color("3f7a3f")

var _theme: Theme = preload("res://resources/theme_paper.tres")

var _tracker: PanelContainer
var _tracker_title: Label
var _tracker_body: Label

var _panel: PanelContainer
var _panel_body: VBoxContainer
var _daily_label: Label

var _bag_panel: PanelContainer
var _bag_body: VBoxContainer

var _toast: Label
var _toast_timer: float = 0.0

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	_build_tracker()
	_build_panel()
	_build_bag_panel()
	_build_toast()
	QuestStore.changed.connect(_refresh)
	QuestStore.reward_granted.connect(_on_reward)
	QuestStore.notified.connect(_show_toast)
	_refresh()

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
			_toggle(_panel)
			if _panel.visible:
				QuestStore.refresh()
			get_viewport().set_input_as_handled()
		KEY_B:
			_toggle(_bag_panel)
			if _bag_panel.visible:
				QuestStore.refresh_bag()
			get_viewport().set_input_as_handled()
		KEY_ESCAPE:
			if _panel.visible or _bag_panel.visible:
				_panel.visible = false
				_bag_panel.visible = false
				get_viewport().set_input_as_handled()

func _toggle(p: Control) -> void:
	var show := not p.visible
	_panel.visible = false
	_bag_panel.visible = false
	p.visible = show

# ---- 构建 ----

func _paper(bg_alpha: float = 0.94) -> PanelContainer:
	var p := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(1, 1, 1, bg_alpha)
	sb.corner_radius_top_left = 8
	sb.corner_radius_top_right = 8
	sb.corner_radius_bottom_left = 8
	sb.corner_radius_bottom_right = 8
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = Color("a88b62")
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 10
	sb.content_margin_bottom = 10
	p.add_theme_stylebox_override("panel", sb)
	return p

func _label(text: String, size: int, color: Color = COLOR_INK, wrap: bool = false) -> Label:
	var l := Label.new()
	l.text = text
	l.add_theme_font_size_override("font_size", size)
	l.add_theme_color_override("font_color", color)
	if wrap:
		l.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
		l.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	return l

## 右上追踪条：当前主线名称 + 目标进度（点击开面板）。
func _build_tracker() -> void:
	_tracker = _paper(0.88)
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
	_tracker_title = _label("任务", 15, COLOR_ACCENT)
	box.add_child(_tracker_title)
	_tracker_body = _label("", 13, COLOR_MUTED)
	_tracker_body.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	box.add_child(_tracker_body)
	_tracker.gui_input.connect(func(ev: InputEvent) -> void:
		if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
			_panel.visible = true
			_bag_panel.visible = false
			QuestStore.refresh())

## 任务面板（J）：日常计数 + 主线 / 日常列表。
func _build_panel() -> void:
	_panel = _paper()
	_panel.visible = false
	_panel.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(_panel)
	_panel.set_anchors_preset(Control.PRESET_CENTER)
	_panel.custom_minimum_size = Vector2(640, 520)
	_panel.size = Vector2(640, 520)
	_panel.offset_left = -320
	_panel.offset_right = 320
	_panel.offset_top = -260
	_panel.offset_bottom = 260
	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 8)
	_panel.add_child(root)
	var head := HBoxContainer.new()
	root.add_child(head)
	head.add_child(_label("任务", 20, COLOR_ACCENT))
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	head.add_child(spacer)
	var close := Button.new()
	close.text = "关闭 (Esc)"
	close.pressed.connect(func() -> void: _panel.visible = false)
	head.add_child(close)
	_daily_label = _label("", 13, COLOR_MUTED)
	root.add_child(_daily_label)
	var scroll := ScrollContainer.new()
	scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	scroll.custom_minimum_size = Vector2(0, 400)
	scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	root.add_child(scroll)
	_panel_body = VBoxContainer.new()
	_panel_body.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_panel_body.add_theme_constant_override("separation", 6)
	scroll.add_child(_panel_body)

## 背包面板（B）：格数占用 + 物品明细 + 钱包。
func _build_bag_panel() -> void:
	_bag_panel = _paper()
	_bag_panel.visible = false
	_bag_panel.mouse_filter = Control.MOUSE_FILTER_STOP
	add_child(_bag_panel)
	_bag_panel.set_anchors_preset(Control.PRESET_CENTER)
	_bag_panel.custom_minimum_size = Vector2(560, 460)
	_bag_panel.size = Vector2(560, 460)
	_bag_panel.offset_left = -280
	_bag_panel.offset_right = 280
	_bag_panel.offset_top = -230
	_bag_panel.offset_bottom = 230
	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 8)
	_bag_panel.add_child(root)
	var head := HBoxContainer.new()
	root.add_child(head)
	head.add_child(_label("背包", 20, COLOR_ACCENT))
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	head.add_child(spacer)
	var close := Button.new()
	close.text = "关闭 (Esc)"
	close.pressed.connect(func() -> void: _bag_panel.visible = false)
	head.add_child(close)
	var scroll := ScrollContainer.new()
	scroll.size_flags_vertical = Control.SIZE_EXPAND_FILL
	scroll.custom_minimum_size = Vector2(0, 340)
	scroll.horizontal_scroll_mode = ScrollContainer.SCROLL_MODE_DISABLED
	root.add_child(scroll)
	_bag_body = VBoxContainer.new()
	_bag_body.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_bag_body.add_theme_constant_override("separation", 4)
	scroll.add_child(_bag_body)

func _build_toast() -> void:
	_toast = _label("", 16, COLOR_OK)
	_toast.visible = false
	_toast.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	add_child(_toast)
	_toast.set_anchors_preset(Control.PRESET_CENTER_TOP)
	_toast.offset_left = -320
	_toast.offset_right = 320
	_toast.offset_top = 64
	_toast.offset_bottom = 96

func _show_toast(msg: String) -> void:
	if msg == "":
		return
	_toast.text = msg
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
	if st == "completable":
		_tracker_title.add_theme_color_override("font_color", COLOR_OK)
	else:
		_tracker_title.add_theme_color_override("font_color", COLOR_ACCENT)

func _refresh_panel() -> void:
	_daily_label.text = _daily_text()
	for c in _panel_body.get_children():
		c.queue_free()
	_section("主线")
	for q in QuestStore.main_quests():
		_panel_body.add_child(_quest_row(q))
	_section("日常")
	for q in QuestStore.daily_quests():
		_panel_body.add_child(_quest_row(q))

func _daily_text() -> String:
	var parts: Array = []
	for cat in ["normal", "sect", "dungeon"]:
		var d: Dictionary = QuestStore.daily.get(cat, {})
		parts.append("%s %d/%d" % [str(QuestStore.CATEGORY_NAME.get(cat, cat)),
			int(d.get("used", 0)), int(d.get("limit", 0))])
	return "等级 %d ｜ 经验 %d ｜ 金币 %d ｜ 修为 %d      %s" % [
		QuestStore.level, QuestStore.exp,
		int(QuestStore.wallet.get("gold", 0)), int(QuestStore.wallet.get("xiuwei", 0)),
		"   ".join(parts)]

func _section(text: String) -> void:
	var l := _label(text, 15, COLOR_ACCENT)
	_panel_body.add_child(l)

func _quest_row(q: Dictionary) -> Control:
	var row := _paper(0.6)
	row.mouse_filter = Control.MOUSE_FILTER_PASS
	var box := VBoxContainer.new()
	box.add_theme_constant_override("separation", 2)
	row.add_child(box)
	var head := HBoxContainer.new()
	box.add_child(head)
	var st := str(q.get("state", ""))
	var title := _label("【%s】%s" % [str(QuestStore.STATE_TEXT.get(st, st)), str(q.get("name", ""))], 15,
		COLOR_OK if st == "completable" else COLOR_INK)
	head.add_child(title)
	var spacer := Control.new()
	spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	head.add_child(spacer)
	if st == "available":
		var btn := Button.new()
		btn.text = "接取"
		var qid := str(q.get("id", ""))
		btn.pressed.connect(func() -> void: QuestStore.accept(qid))
		head.add_child(btn)
	elif st == "completable":
		var btn := Button.new()
		btn.text = "交付"
		var qid := str(q.get("id", ""))
		btn.pressed.connect(func() -> void: QuestStore.submit(qid))
		head.add_child(btn)
	box.add_child(_label(str(q.get("desc", "")), 12, COLOR_MUTED, true))
	var prog := QuestStore.progress_text(q)
	if prog != "":
		box.add_child(_label(prog, 12, COLOR_INK, true))
	if st == "locked":
		box.add_child(_label(str(q.get("reason", "未满足条件")), 12, COLOR_MUTED, true))
	box.add_child(_label(_reward_text(q), 12, COLOR_MUTED, true))
	return row

func _reward_text(q: Dictionary) -> String:
	var r: Dictionary = q.get("rewards", {})
	var parts: Array = []
	if int(r.get("exp", 0)) > 0:
		parts.append("经验 %d" % int(r.get("exp", 0)))
	if int(r.get("xiuwei", 0)) > 0:
		parts.append("修为 %d" % int(r.get("xiuwei", 0)))
	if int(r.get("gold", 0)) > 0:
		parts.append("金币 %d" % int(r.get("gold", 0)))
	for it in r.get("items", []):
		var info: Dictionary = it
		parts.append("%s×%d" % [str(info.get("name", "")), int(info.get("count", 0))])
	return "奖励：" + ("、".join(parts) if not parts.is_empty() else "无")

func _refresh_bag() -> void:
	for c in _bag_body.get_children():
		c.queue_free()
	var cap := int(QuestStore.bag.get("capacity", 0))
	var used := int(QuestStore.bag.get("used", 0))
	_bag_body.add_child(_label("格子 %d/%d ｜ 金币 %d ｜ 修为 %d" % [
		used, cap, int(QuestStore.wallet.get("gold", 0)), int(QuestStore.wallet.get("xiuwei", 0))],
		13, COLOR_MUTED))
	var items: Array = QuestStore.bag.get("items", [])
	if items.is_empty():
		_bag_body.add_child(_label("背包空空如也。完成任务的物品奖励会进到这里。", 13, COLOR_MUTED))
		return
	for it in items:
		var info: Dictionary = it
		var row := _paper(0.6)
		var hb := HBoxContainer.new()
		row.add_child(hb)
		hb.add_child(_label(str(info.get("name", info.get("id", ""))), 14, COLOR_INK))
		var spacer := Control.new()
		spacer.size_flags_horizontal = Control.SIZE_EXPAND_FILL
		hb.add_child(spacer)
		hb.add_child(_label("×%d" % int(info.get("count", 0)), 14, COLOR_ACCENT))
		var stack := int(info.get("stack", 0))
		if stack > 0:
			hb.add_child(_label("（单格上限 %d）" % stack, 11, COLOR_MUTED))
		_bag_body.add_child(row)
