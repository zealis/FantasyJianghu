extends Control
## UI 层 · 选择宗门界面（UI 规范 §9 定稿）：八派卡片 4×2，驻地实景头图 +
## 系别色标（物理朱橙/法术紫/封印青/肉盾绿）+ 特色一句话 + 代表秘籍；
## 底部汇总已选门派的驻地/武器/同系门派；确认创建（首期不可转派）。

signal creation_confirmed(faction: String)

const COLOR_PAPER := Color("f6efdf")
const COLOR_INK := Color("3b2e1e")
const COLOR_ACCENT := Color("c46a2b")
# 系别色标（V1.27/GDD §5.3）。
const CATEGORY_COLORS := {"朱橙": Color("c46a2b"), "紫": Color("7a5bbf"), "青": Color("3f9d8f"), "绿": Color("4e8a4f")}

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _factions: Array = []  # 服务端 2/1 下发的八派明细
var _selected := ""
var _summary: Label
var _confirm_btn: Button

func _ready() -> void:
	theme = _theme
	set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	_build_bg()
	_build_body()

func _build_bg() -> void:
	var bg := TextureRect.new()
	bg.texture = load("res://assets/maps/qinghecun.png")
	bg.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	bg.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(bg)

func _build_body() -> void:
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
	sb.border_color = Color("a88b62")
	sb.content_margin_left = 24
	sb.content_margin_right = 24
	sb.content_margin_top = 16
	sb.content_margin_bottom = 12
	panel.add_theme_stylebox_override("panel", sb)
	panel.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	panel.offset_left = 120
	panel.offset_top = 40
	panel.offset_right = -120
	panel.offset_bottom = -40
	add_child(panel)

	var root := VBoxContainer.new()
	root.add_theme_constant_override("separation", 8)
	panel.add_child(root)

	var title := Label.new()
	title.text = "选择宗门（八选一 · 首期不可转派）"
	title.add_theme_font_size_override("font_size", 22)
	title.add_theme_color_override("font_color", COLOR_INK)
	root.add_child(title)

	# 4×2 卡片网格。
	var grid := GridContainer.new()
	grid.columns = 4
	grid.add_theme_constant_override("h_separation", 10)
	grid.add_theme_constant_override("v_separation", 10)
	grid.size_flags_vertical = Control.SIZE_EXPAND_FILL
	root.add_child(grid)
	for f in _factions:
		grid.add_child(_faction_card(f))

	# 底部汇总 + 确认。
	_summary = Label.new()
	_summary.text = "已选：未选择（点击卡片）"
	_summary.add_theme_color_override("font_color", COLOR_INK)
	root.add_child(_summary)
	_confirm_btn = Button.new()
	_confirm_btn.text = "确认创建角色"
	_confirm_btn.custom_minimum_size = Vector2(0, 42)
	_confirm_btn.disabled = true
	var sb2 := StyleBoxFlat.new()
	sb2.bg_color = COLOR_ACCENT
	sb2.corner_radius_top_left = 6
	sb2.corner_radius_top_right = 6
	sb2.corner_radius_bottom_left = 6
	sb2.corner_radius_bottom_right = 6
	_confirm_btn.add_theme_stylebox_override("normal", sb2)
	_confirm_btn.add_theme_color_override("font_color", Color("fdf6ea"))
	_confirm_btn.pressed.connect(func() -> void: creation_confirmed.emit(_selected))
	root.add_child(_confirm_btn)

func _faction_card(f: Dictionary) -> PanelContainer:
	var card := PanelContainer.new()
	card.custom_minimum_size = Vector2(220, 210)
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color("fdf8ec")
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = Color("a88b62")
	sb.content_margin_left = 6
	sb.content_margin_right = 6
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	card.add_theme_stylebox_override("panel", sb)
	var v := VBoxContainer.new()
	v.add_theme_constant_override("separation", 4)
	card.add_child(v)

	# 驻地实景头图。
	var head := TextureRect.new()
	head.texture = load("res://assets/sects/驻地_%s.png" % f.get("name", ""))
	head.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	head.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	head.custom_minimum_size = Vector2(0, 92)
	v.add_child(head)

	# 名字 + 系别色标。
	var name_row := HBoxContainer.new()
	v.add_child(name_row)
	var chip := ColorRect.new()
	chip.color = CATEGORY_COLORS.get(str(f.get("color", "朱橙")), COLOR_ACCENT)
	chip.custom_minimum_size = Vector2(10, 18)
	name_row.add_child(chip)
	var name_lab := Label.new()
	name_lab.text = str(f.get("name", "?"))
	name_lab.add_theme_font_size_override("font_size", 16)
	name_lab.add_theme_color_override("font_color", COLOR_INK)
	name_row.add_child(name_lab)
	var cat := Label.new()
	cat.text = " %s" % str(f.get("category", ""))
	cat.add_theme_font_size_override("font_size", 12)
	cat.add_theme_color_override("font_color", CATEGORY_COLORS.get(str(f.get("color", "朱橙")), COLOR_ACCENT))
	name_row.add_child(cat)

	var feature := Label.new()
	feature.text = str(f.get("feature", ""))
	feature.autowrap_mode = TextServer.AUTOWRAP_WORD_SMART
	feature.add_theme_font_size_override("font_size", 11)
	feature.add_theme_color_override("font_color", COLOR_INK)
	feature.custom_minimum_size = Vector2(0, 42)
	v.add_child(feature)

	var secret := Label.new()
	secret.text = "代表秘籍：%s" % str(f.get("secret", ""))
	secret.add_theme_font_size_override("font_size", 11)
	secret.add_theme_color_override("font_color", Color("7a6a52"))
	v.add_child(secret)

	# 点选高亮 + 汇总刷新。
	card.mouse_filter = Control.MOUSE_FILTER_STOP
	card.gui_input.connect(func(ev: InputEvent) -> void:
		if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
			_select(f, card, sb))
	return card

func _select(f: Dictionary, card: PanelContainer, sb: StyleBoxFlat) -> void:
	_selected = str(f.get("name", ""))
	# 简化高亮：改边框色（重置由重绘承担——M1.5 可接受）。
	sb.border_color = COLOR_ACCENT
	var same_category := []
	for other in _factions:
		if other.get("name", "") != _selected and other.get("category", "") == f.get("category", ""):
			same_category.append(other.get("name", ""))
	_summary.text = "已选：%s · %s系 · 驻地 %s · 武器 %s · 同系：%s" % [
		_selected, f.get("category", ""), f.get("residence", ""), f.get("weapon", ""), "、".join(same_category)]
	_confirm_btn.disabled = false
