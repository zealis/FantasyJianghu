extends Control
## UI 层 · 对话面板（UI§4.1 定案）：场景保持可见，对话框横贯下方，
## 左 NPC 名牌/立绘位，正文墨字，右侧选项列（点击即推进，不另设确认钮）。

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _lines: Array = []
var _index: int = 0
var _text: Label
var _speaker: Label
var _box: PanelContainer
var _opts_box: VBoxContainer
var _quest_box: VBoxContainer
var _cont: Button
var _npc_id: String = ""

signal dialogue_action(action: String)

var _options: Array = []

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	mouse_filter = Control.MOUSE_FILTER_IGNORE
	visible = false
	WorldStore.dialogue_requested.connect(_open)
	QuestStore.changed.connect(_refresh_quest_options)
	_build()

func _build() -> void:
	_box = PanelContainer.new()
	_box.set_anchors_and_offsets_preset(Control.PRESET_BOTTOM_WIDE)
	_box.offset_left = 24
	_box.offset_right = -24
	_box.offset_top = -180
	_box.offset_bottom = -24
	add_child(_box)
	var hbox := HBoxContainer.new()
	hbox.add_theme_constant_override("separation", 16)
	_box.add_child(hbox)
	var left := VBoxContainer.new()
	left.custom_minimum_size = Vector2(140, 0)
	hbox.add_child(left)
	_speaker = Label.new()
	_speaker.add_theme_font_size_override("font_size", 18)
	_speaker.add_theme_color_override("font_color", Color("B34A14"))
	left.add_child(_speaker)
	var portrait := TextureRect.new()
	portrait.texture = load("res://assets/sprites/player/idle_0.png")
	portrait.custom_minimum_size = Vector2(96, 96)
	portrait.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	portrait.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
	portrait.modulate = Color(0.8, 0.7, 0.55)
	left.add_child(portrait)
	_text = Label.new()
	_text.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	_text.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	_text.size_flags_vertical = Control.SIZE_EXPAND_FILL
	_text.add_theme_font_size_override("font_size", 16)
	hbox.add_child(_text)
	var opts := VBoxContainer.new()
	opts.custom_minimum_size = Vector2(140, 0)
	opts.alignment = BoxContainer.ALIGNMENT_CENTER
	hbox.add_child(opts)
	_opts_box = opts
	# 任务选项独占一列（与对话行选项分离，避免翻页时被清掉）。
	_quest_box = VBoxContainer.new()
	_quest_box.add_theme_constant_override("separation", 4)
	opts.add_child(_quest_box)
	var cont := Button.new()
	cont.text = "继续"
	cont.pressed.connect(_next)
	opts.add_child(cont)
	_cont = cont
	var leave := Button.new()
	leave.text = "离开"
	leave.pressed.connect(_close)
	opts.add_child(leave)

func _open(npc_id: String) -> void:
	_npc_id = npc_id
	var logic := SceneLogic.get_logic()
	var key := ""
	for n in logic.get("npcs", []):
		if n.get("id") == npc_id:
			key = String(n.get("dialogue", ""))
	_lines = logic.get("dialogues", {}).get(key, [])
	_index = 0
	visible = true
	_show_line()
	_refresh_quest_options()
	_sync_talk(npc_id)

## 对话即向服务端记账（10/1）：推进 talk 类任务目标，并刷新任务状态。
func _sync_talk(npc_id: String) -> void:
	await Network.send_and_wait(10, 1, {"map_id": WorldStore.current_map, "npc_id": npc_id})
	await QuestStore.refresh()
	_refresh_quest_options()

## 该 NPC 身上可接/可交的任务按钮（数据来自 11/1，客户端不自行判定）。
func _refresh_quest_options() -> void:
	if _quest_box == null:
		return
	for c in _quest_box.get_children():
		c.queue_free()
	if not visible or _npc_id == "":
		return
	for q in QuestStore.quests_for_npc(_npc_id):
		var info: Dictionary = q
		var qid := str(info.get("id", ""))
		var st := str(info.get("state", ""))
		var btn := Button.new()
		btn.name = "Quest_" + qid
		if st == "available":
			btn.text = "【任务】接取：%s" % str(info.get("name", ""))
			btn.pressed.connect(func() -> void: QuestStore.accept(qid))
		else:
			btn.text = "【任务】交付：%s" % str(info.get("name", ""))
			btn.pressed.connect(func() -> void: QuestStore.submit(qid))
		_quest_box.add_child(btn)

func _show_line() -> void:
	if _index >= _lines.size():
		_close()
		return
	var line: Dictionary = _lines[_index]
	_speaker.text = String(line.get("speaker", ""))
	_text.text = String(line.get("text", ""))
	# 选项渲染（数据驱动：对话行可带 options[{text, action}]）。
	_options = line.get("options", [])
	# 有选项的对话行隐藏「继续」，否则点它会直接跳过切磋等 action 入口。
	_cont.visible = _options.is_empty()
	for c in _opts_box.get_children():
		if c.name.begins_with("Opt_"):
			c.visible = false  # queue_free 帧末才删，先禁用避免同帧误点旧按钮
			c.queue_free()
	for i in range(_options.size()):
		var o = _options[i]
		var info: Dictionary = o
		var btn := Button.new()
		btn.name = "Opt_%d" % i
		btn.text = str(info.get("text", ""))
		var action := String(info.get("action", ""))
		btn.pressed.connect(func() -> void:
			if action != "":
				_close()
				dialogue_action.emit(action)
			else:
				_next())
		_opts_box.add_child(btn)

func _next() -> void:
	_index += 1
	_show_line()

func _close() -> void:
	visible = false