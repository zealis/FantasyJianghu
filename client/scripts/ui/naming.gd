extends Control
## UI 层 · 角色命名界面（UI 规范 §9 定稿：角色命名界面_v2.png）。
## 创角第一步：左实景舞台 + 人物形象与性别切换；右角色名输入（2–6 汉字）
## + 随机取名 + 江湖预览条（【称号】+名字通则）+ 现成名示例。下一步进入选择宗门。

signal naming_done(name: String, gender: String)

const COLOR_PAPER := Color("f6efdf")
const COLOR_INK := Color("3b2e1e")
const COLOR_ACCENT := Color("c46a2b")

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _gender := "男"
var _hero: TextureRect
var _name_edit: LineEdit
var _preview: Label

const SAMPLE_NAMES := ["云溪", "陆沉舟", "苏晚吟", "沈鹤鸣", "顾长风", "白惊鸿"]

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_build_bg()
	_build_stage()
	_build_form()

func _build_bg() -> void:
	var bg := TextureRect.new()
	bg.texture = load("res://assets/maps/qinghecun.png")
	bg.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	bg.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(bg)

func _panel() -> PanelContainer:
	var p := PanelContainer.new()
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
	sb.border_color = Color("a88b62")
	sb.content_margin_left = 20
	sb.content_margin_right = 20
	sb.content_margin_top = 16
	sb.content_margin_bottom = 16
	p.add_theme_stylebox_override("panel", sb)
	return p

func _build_stage() -> void:
	var stage := _panel()
	stage.anchor_left = 0.0
	stage.anchor_top = 0.0
	stage.anchor_right = 0.45
	stage.anchor_bottom = 1.0
	stage.offset_left = 60
	stage.offset_top = 60
	stage.offset_right = -20
	stage.offset_bottom = -80
	add_child(stage)
	_hero = TextureRect.new()
	_hero.texture = load("res://assets/heroes/断岳剑宗_男.png")
	_hero.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	_hero.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_CENTERED
	_hero.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_hero.offset_top = 10
	_hero.offset_bottom = -10
	stage.add_child(_hero)

func _build_form() -> void:
	var form := _panel()
	form.anchor_left = 0.45
	form.anchor_top = 0.0
	form.anchor_right = 1.0
	form.anchor_bottom = 1.0
	form.offset_left = 20
	form.offset_top = 60
	form.offset_right = -60
	form.offset_bottom = -80
	add_child(form)
	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 10)
	form.add_child(root)

	var title := Label.new()
	title.text = "创建角色 · 命名"
	title.add_theme_font_size_override("font_size", 22)
	title.add_theme_color_override("font_color", COLOR_INK)
	root.add_child(title)

	# 性别切换（男/女 分段钮）。
	var gender_row := HBoxContainer.new()
	gender_row.add_theme_constant_override("separation", 10)
	root.add_child(gender_row)
	var lab := Label.new()
	lab.text = "性别"
	lab.add_theme_color_override("font_color", COLOR_INK)
	gender_row.add_child(lab)
	for g in ["男", "女"]:
		var b := Button.new()
		b.text = g
		b.toggle_mode = true
		b.button_pressed = g == _gender
		b.pressed.connect(_on_gender.bind(g, b))
		b.custom_minimum_size = Vector2(72, 32)
		gender_row.add_child(b)

	_name_edit = LineEdit.new()
	_name_edit.placeholder_text = "角色名（2–6 个汉字）"
	_name_edit.custom_minimum_size = Vector2(0, 36)
	_name_edit.text_changed.connect(func(_t: String) -> void: _refresh_preview())
	root.add_child(_name_edit)

	var rand_row := HBoxContainer.new()
	rand_row.add_theme_constant_override("separation", 8)
	root.add_child(rand_row)
	var rand := Button.new()
	rand.text = "随机取名"
	rand.pressed.connect(_on_random_name)
	rand_row.add_child(rand)
	var lab2 := Label.new()
	lab2.text = "现成名："
	lab2.add_theme_color_override("font_color", COLOR_INK)
	rand_row.add_child(lab2)
	for n in SAMPLE_NAMES.slice(0, 3):
		var sb := Button.new()
		sb.text = n
		sb.flat = true
		sb.add_theme_color_override("font_color", COLOR_ACCENT)
		sb.pressed.connect(func() -> void:
			_name_edit.text = n
			_refresh_preview())
		rand_row.add_child(sb)

	# 江湖预览条（【称号】+名字通则；无称号时仅名字）。
	var preview_panel := PanelContainer.new()
	var psb := StyleBoxFlat.new()
	psb.bg_color = Color("fdf8ec")
	psb.border_width_left = 1
	psb.border_width_right = 1
	psb.border_width_top = 1
	psb.border_width_bottom = 1
	psb.border_color = Color("a88b62")
	psb.content_margin_left = 10
	psb.content_margin_right = 10
	psb.content_margin_top = 6
	psb.content_margin_bottom = 6
	preview_panel.add_theme_stylebox_override("panel", psb)
	root.add_child(preview_panel)
	_preview = Label.new()
	_preview.text = "江湖预览：云溪"
	_preview.add_theme_color_override("font_color", COLOR_INK)
	preview_panel.add_child(_preview)

	var spacer := Control.new()
	spacer.size_flags_vertical = Control.SIZE_EXPAND_FILL
	root.add_child(spacer)

	var next_btn := Button.new()
	next_btn.text = "下一步 · 选择宗门"
	next_btn.custom_minimum_size = Vector2(0, 44)
	var sb2 := StyleBoxFlat.new()
	sb2.bg_color = COLOR_ACCENT
	sb2.corner_radius_top_left = 6
	sb2.corner_radius_top_right = 6
	sb2.corner_radius_bottom_left = 6
	sb2.corner_radius_bottom_right = 6
	next_btn.add_theme_stylebox_override("normal", sb2)
	next_btn.add_theme_color_override("font_color", Color("fdf6ea"))
	next_btn.add_theme_font_size_override("font_size", 18)
	next_btn.pressed.connect(_on_next)
	root.add_child(next_btn)

func _on_gender(g: String, b: Button) -> void:
	_gender = g
	_hero.texture = load("res://assets/heroes/断岳剑宗_%s.png" % g)
	_refresh_preview()

func _on_random_name() -> void:
	var rng := RandomNumberGenerator.new()
	rng.randomize()
	_name_edit.text = SAMPLE_NAMES[rng.randi_range(0, SAMPLE_NAMES.size() - 1)]
	_refresh_preview()

func _refresh_preview() -> void:
	# 【称号】+名字通则（§9）：称号在前、独立着色——创角期无称号仅显示名字。
	var n := _name_edit.text.strip_edges()
	_preview.text = "江湖预览：%s" % (n if n != "" else "——")

func _on_next() -> void:
	var n := _name_edit.text.strip_edges()
	var regex := RegEx.new()
	regex.compile("^[\\x{4e00}-\\x{9fff}]{2,6}$")
	if regex.search(n) == null:
		_preview.text = "江湖预览：角色名须为 2–6 个汉字"
		return
	naming_done.emit(n, _gender)
