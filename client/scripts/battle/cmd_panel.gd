extends Control
## UI 层 · 战斗指令台（严格按 ui-mockups/战斗界面.png 定稿 + UI 规范 V1.27 §5）：
## 宣纸米底横贯全宽；左角色卡（头像/名字/气血/真气）、中间上排秘籍卡、
## 下排六键（普攻/秘籍/法宝/物品/防御/逃跑）、右侧朱橙圆环倒计时与「请选择指令」。
## 注：法宝/物品/倍速/自动 的协议位在 M2a 未实现，按定稿保留位置并置灰，不做假功能。

signal cmd_submitted(cmd: String, skill_id: String)
## 4v4 两段式：选招后进入待选目标，点敌方单位才发动。
signal cmd_selected(cmd: String, skill_id: String)

# 规范 §2.1 宣纸体系令牌；角色卡取色直接来自 ui-mockups/战斗界面.png（定稿优先）。
const C_PAPER := Color("F5ECD7")   # 定稿宣纸底
const C_PAPER_2 := Color("F5ECD7") # 角色卡与指令台同底，靠金棕细边框区分
const C_EDGE := Color("8A6D3B")    # 定稿金棕细边
const C_BROWN := Color("5C422A")
const C_INK := Color("3A2E1C")     # 名字深墨
const C_INK_2 := Color("6B573A")   # 数值中墨
const C_ACC := Color("DE6423")
const C_ACC_DEEP := Color("B34A14")
# 定稿角色卡：气血绿 / 真气蓝 / 槽底浅褐（不是 §2 的橙 / 青令牌）。
const C_HP := Color("60BA5C")
const C_MP := Color("4A80B6")
const C_BAR_BG := Color("D8CBA8")

var _theme: Theme = preload("res://resources/theme_paper.tres")
const AvatarFrames := preload("res://scripts/ui/avatar_frames.gd")
var _skills: Array = []
var _submitted := false
var _buttons: Array = []
var _skill_cards: Array = []
var _cd_label: Label
var _cd_value: float = 30.0
var _name_label: Label
var _hp_bar: ProgressBar
var _hp_text: Label
var _mp_bar: ProgressBar
var _mp_text: Label
var _avatar: TextureRect
var _card_row: HBoxContainer
var _hint_label: Label
var requires_target := false  # 敌方 > 1 时由 battle.gd 置 true
var _pending := false
var auto_play := false  # 自动战斗：复用倒计时归零自动普攻

func _ready() -> void:
	theme = _theme
	mouse_filter = Control.MOUSE_FILTER_STOP
	var panel := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = C_PAPER
	sb.border_width_top = 3
	sb.border_color = C_BROWN
	sb.content_margin_left = 18
	sb.content_margin_right = 18
	sb.content_margin_top = 10
	sb.content_margin_bottom = 12
	panel.add_theme_stylebox_override("panel", sb)
	panel.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(panel)
	var row := HBoxContainer.new()
	row.add_theme_constant_override("separation", 16)
	panel.add_child(row)
	row.add_child(_build_player_card())

	var center := VBoxContainer.new()
	center.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	# 垂直居中：把剩余高度均分到上下，避免六键（普攻/秘籍…）下方堆一大片空白。
	center.alignment = BoxContainer.ALIGNMENT_CENTER
	center.add_theme_constant_override("separation", 12)
	row.add_child(center)
	_card_row = HBoxContainer.new()
	_card_row.alignment = BoxContainer.ALIGNMENT_CENTER
	_card_row.add_theme_constant_override("separation", 10)
	center.add_child(_card_row)
	center.add_child(_build_cmd_row())
	row.add_child(_build_timer())

## 左侧角色卡（按定稿逐项对齐）：金棕细边框宣纸卡 + 独立金框方头像 +
## 大号墨色名字 → 绿气血条 → 气血数值 → 蓝真气条 → 真气数值。
func _build_player_card() -> Control:
	var card := PanelContainer.new()
	var sb := StyleBoxFlat.new()
	sb.bg_color = C_PAPER_2
	sb.set_corner_radius_all(10)
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = C_EDGE
	sb.content_margin_left = 10
	sb.content_margin_right = 10
	sb.content_margin_top = 8
	sb.content_margin_bottom = 8
	card.add_theme_stylebox_override("panel", sb)
	card.custom_minimum_size = Vector2(250, 0)
	var hbox := HBoxContainer.new()
	hbox.add_theme_constant_override("separation", 12)
	card.add_child(hbox)
	# 头像：定稿是带独立金棕细框的方形 Q 版头像（此前无框且贴边）。
	var frame := PanelContainer.new()
	var fsb := StyleBoxFlat.new()
	fsb.bg_color = Color("FCF6E6")
	fsb.set_corner_radius_all(5)
	fsb.border_width_left = 2
	fsb.border_width_right = 2
	fsb.border_width_top = 2
	fsb.border_width_bottom = 2
	fsb.border_color = C_EDGE
	fsb.content_margin_left = 3
	fsb.content_margin_right = 3
	fsb.content_margin_top = 3
	fsb.content_margin_bottom = 3
	frame.add_theme_stylebox_override("panel", fsb)
	# 关键：不要让 HBox 把头像框竖向撑成长条，否则正方形素材会被裁成窄条。
	frame.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	hbox.add_child(frame)
	_avatar = TextureRect.new()
	_avatar.custom_minimum_size = Vector2(68, 68)
	_avatar.size_flags_vertical = Control.SIZE_SHRINK_CENTER
	_avatar.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	# 完整展示整张方形头像（不裁切、不拉伸），与定稿的方形头像一致。
	_avatar.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
	frame.add_child(_avatar)
	var col := VBoxContainer.new()
	col.size_flags_horizontal = Control.SIZE_EXPAND_FILL
	col.add_theme_constant_override("separation", 3)
	hbox.add_child(col)
	_name_label = Label.new()
	_name_label.text = "侠客"
	_name_label.add_theme_font_size_override("font_size", 21)
	_name_label.add_theme_color_override("font_color", C_INK)
	col.add_child(_name_label)
	_hp_bar = _stat_bar(C_HP)
	col.add_child(_hp_bar)
	_hp_text = _stat_label()
	col.add_child(_hp_text)
	_mp_bar = _stat_bar(C_MP)
	col.add_child(_mp_bar)
	_mp_text = _stat_label()
	col.add_child(_mp_text)
	return card

func _stat_bar(color: Color) -> ProgressBar:
	var bar := ProgressBar.new()
	bar.max_value = 1.0
	bar.value = 1.0
	bar.show_percentage = false
	bar.custom_minimum_size = Vector2(0, 12)
	var fill := StyleBoxFlat.new()
	fill.bg_color = color
	fill.set_corner_radius_all(6)
	var back := StyleBoxFlat.new()
	back.bg_color = C_BAR_BG
	back.set_corner_radius_all(6)
	bar.add_theme_stylebox_override("fill", fill)
	bar.add_theme_stylebox_override("background", back)
	return bar

func _stat_label() -> Label:
	var l := Label.new()
	l.text = "—"
	l.add_theme_font_size_override("font_size", 14)
	l.add_theme_color_override("font_color", C_INK_2)
	return l

## 下排六键（定稿：秘籍为朱橙主按钮）。
func _build_cmd_row() -> HBoxContainer:
	var row := HBoxContainer.new()
	row.alignment = BoxContainer.ALIGNMENT_CENTER
	row.add_theme_constant_override("separation", 12)
	for item in [
		["普攻", "attack", true], ["秘籍", "skill", true], ["法宝", "fabao", false],
		["物品", "item", false], ["防御", "defend", true], ["逃跑", "flee", true],
	]:
		var label: String = item[0]
		var cmd: String = item[1]
		var enabled: bool = item[2]
		var btn := Button.new()
		btn.text = label
		btn.custom_minimum_size = Vector2(104, 38)
		btn.add_theme_font_size_override("font_size", 14)
		if cmd == "skill":
			var acc := StyleBoxFlat.new()
			acc.bg_color = C_ACC
			acc.set_corner_radius_all(8)
			btn.add_theme_stylebox_override("normal", acc)
			var hover := StyleBoxFlat.new()
			hover.bg_color = C_ACC_DEEP
			hover.set_corner_radius_all(8)
			btn.add_theme_stylebox_override("hover", hover)
			btn.add_theme_stylebox_override("pressed", hover)
			btn.add_theme_color_override("font_color", Color.WHITE)
		else:
			var paper := StyleBoxFlat.new()
			paper.bg_color = Color("fbf5e6")
			paper.set_corner_radius_all(8)
			paper.border_width_left = 1
			paper.border_width_right = 1
			paper.border_width_top = 1
			paper.border_width_bottom = 1
			paper.border_color = C_BROWN
			btn.add_theme_stylebox_override("normal", paper)
			btn.add_theme_color_override("font_color", C_INK)
		if not enabled:
			btn.disabled = true
			btn.tooltip_text = "M2a 协议未开放此指令"
		else:
			btn.pressed.connect(_on_cmd.bind(cmd))
			_buttons.append(btn)
		row.add_child(btn)
	return row

## 右侧朱橙圆环倒计时 + 「请选择指令」。
func _build_timer() -> Control:
	var col := VBoxContainer.new()
	col.alignment = BoxContainer.ALIGNMENT_CENTER
	col.add_theme_constant_override("separation", 4)
	var ring := PanelContainer.new()
	ring.custom_minimum_size = Vector2(54, 54)
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0, 0, 0, 0)
	sb.set_corner_radius_all(27)
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = C_ACC
	ring.add_theme_stylebox_override("panel", sb)
	_cd_label = Label.new()
	_cd_label.text = "30"
	_cd_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_cd_label.vertical_alignment = VERTICAL_ALIGNMENT_CENTER
	_cd_label.add_theme_font_size_override("font_size", 22)
	_cd_label.add_theme_color_override("font_color", C_ACC_DEEP)
	ring.add_child(_cd_label)
	col.add_child(ring)
	_hint_label = Label.new()
	_hint_label.text = "请选择指令"
	_hint_label.add_theme_font_size_override("font_size", 11)
	_hint_label.add_theme_color_override("font_color", C_INK_2)
	col.add_child(_hint_label)
	return col

## 头像素材自带一圈画框（顶部深色横杆最显眼），外层已有金棕细框，
## 直接贴原图会叠成「双框 / 头顶一根横线」。这里按统一比例裁掉四周画框，只留人物。
func _trim_avatar_frame(tex: Texture2D) -> Texture2D:
	# 裁框实现抽到 scripts/ui/avatar_frames.gd，HUD 角色卡与战斗指令台共用同一口径。
	return AvatarFrames.trim(tex)

## 4/1 回包的 self（name/hp/max_hp/mp/max_mp）落到角色卡。
func set_player(info: Dictionary) -> void:
	_name_label.text = str(info.get("name", "侠客"))
	var hp_max: float = maxf(float(info.get("max_hp", 1)), 1.0)
	var mp_max: float = maxf(float(info.get("max_mp", 1)), 1.0)
	var hp: float = float(info.get("hp", hp_max))
	var mp: float = float(info.get("mp", mp_max))
	_hp_bar.max_value = hp_max
	_hp_bar.value = hp
	_hp_text.text = "气血 %d/%d" % [int(hp), int(hp_max)]
	_mp_bar.max_value = mp_max
	_mp_bar.value = mp
	_mp_text.text = "真气 %d/%d" % [int(mp), int(mp_max)]
	var f := SessionStore.faction if SessionStore.faction != "" else "断岳剑宗"
	var g := SessionStore.gender if SessionStore.gender != "" else "男"
	var path := "res://assets/avatars/%s_%s.png" % [f, g]
	if ResourceLoader.exists(path):
		_avatar.texture = _trim_avatar_frame(load(path))

## 自动战斗开关：只缩短倒计时，不伪造托管协议；发出的仍是合法 4/2 普攻。
func set_auto(on: bool) -> void:
	auto_play = on
	if on and not _submitted:
		_cd_value = minf(_cd_value, 0.15)
	if on:
		show_hint("自动战斗中")

## 4v4 由 battle.gd 设置：true 时普攻/秘籍必须先选目标再发动。
func set_requires_target(on: bool) -> void:
	requires_target = on
	_pending = false
	show_hint("请选择指令")

func show_hint(text: String) -> void:
	if _hint_label == null:
		return
	_hint_label.text = text
	_hint_label.add_theme_color_override("font_color", C_ACC_DEEP if text != "请选择指令" else C_INK_2)

func is_pending() -> bool:
	return _pending

## 取消待选目标（Esc）。
func cancel_pending() -> void:
	_pending = false
	show_hint("请选择指令")

## 玩家点目标、指令真正发出时锁住指令台（不重复 emit）。
func mark_submitted() -> void:
	_pending = false
	_submitted = true
	_set_buttons_enabled(false)
	show_hint("指令已发出")

## 每回合复位（battle.gd 在 turn_end / 帧播完时调用）。
func show_cmd() -> void:
	visible = true
	_submitted = false
	_pending = false
	show_hint("自动战斗中" if auto_play else "请选择指令")
	# 自动模式把等待压到 0.15s，倒计时归零时走 auto=true 直接发动（跳过两段式选目标）。
	_cd_value = 0.15 if auto_play else 30.0
	if _cd_label:
		_cd_label.text = str(ceili(_cd_value))
	_set_buttons_enabled(true)

func set_skill_list(skills: Array) -> void:
	_skills = skills
	_rebuild_cards()

## 秘籍卡排：每张卡为「名字 + 真气 N」；真气不足置灰并标差额；不足 4 张补占位。
func _rebuild_cards() -> void:
	if _card_row == null:
		return
	for c in _card_row.get_children():
		c.queue_free()
	_skill_cards.clear()
	var usable: Array = []
	for sk in _skills:
		var info: Dictionary = sk
		if str(info.get("category", "")) == "心法":
			continue
		usable.append(info)
	for i in range(4):
		var card := Button.new()
		card.custom_minimum_size = Vector2(148, 34)
		card.add_theme_font_size_override("font_size", 12)
		var paper := StyleBoxFlat.new()
		paper.bg_color = Color("fbf5e6")
		paper.set_corner_radius_all(7)
		paper.border_width_left = 1
		paper.border_width_right = 1
		paper.border_width_top = 1
		paper.border_width_bottom = 1
		paper.border_color = C_EDGE
		card.add_theme_stylebox_override("normal", paper)
		card.add_theme_color_override("font_color", C_INK)
		if i < usable.size():
			var info: Dictionary = usable[i]
			card.text = "%s   真气 %d" % [str(info.get("name", "?")), int(info.get("mp_cost", 0))]
			var sid := str(info.get("id", ""))
			var cost := int(info.get("mp_cost", 0))
			if cost > int(_mp_bar.value):
				card.disabled = true
				card.text += "  (不足)"
			else:
				card.pressed.connect(func() -> void: _submit("skill", sid))
				_skill_cards.append(card)
		else:
			card.text = "—"
			card.disabled = true
		_card_row.add_child(card)
	if usable.is_empty():
		var first: Button = _card_row.get_child(0) as Button
		first.text = "暂无可使用的秘籍"

func _set_buttons_enabled(on: bool) -> void:
	for b in _buttons:
		if is_instance_valid(b):
			b.disabled = not on
	for c in _skill_cards:
		if is_instance_valid(c):
			c.disabled = not on

func _process(delta: float) -> void:
	if not visible or _submitted or _cd_value <= 0:
		return
	_cd_value -= delta
	if _cd_label:
		_cd_label.text = str(maxi(ceili(_cd_value), 0))
	if _cd_value <= 0:
		# 倒计时归零自动普攻；服务端 30s 托管仍是权威兜底。
		_on_cmd("attack", true)

func _on_cmd(cmd: String, auto: bool = false) -> void:
	if _submitted and not _pending:
		return
	if cmd == "skill":
		# 定稿的秘籍是常驻卡排，点击「秘籍」只做高亮提示，实际出招直接点卡。
		_card_row.modulate = Color(1.0, 0.85, 0.65)
		await get_tree().create_timer(0.35).timeout
		if is_instance_valid(_card_row):
			_card_row.modulate = Color.WHITE
		return
	_submit(cmd, "", auto)

func _submit(cmd: String, skill_id: String, auto: bool = false) -> void:
	if _submitted:
		return
	# 4v4 两段式：点普攻/秘籍后先进入待选目标，点敌方单位才发动。
	# 倒计时归零的自动普攻（auto）与防御/逃跑不走这条，避免卡住回合。
	if not auto and requires_target and (cmd == "attack" or cmd == "skill"):
		_pending = true
		show_hint("请选择攻击目标")
		cmd_selected.emit(cmd, skill_id)
		return
	_pending = false
	_submitted = true
	_set_buttons_enabled(false)
	show_hint("指令已发出")
	cmd_submitted.emit(cmd, skill_id)
