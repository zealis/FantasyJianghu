extends Area2D
## 2.5D 俯视通用交互热区：门洞（进入 → 门内对话）与传送点（前往 → 3/9 权威传送）。
## 文档 §4「交互」条：建筑以 UI 名牌 + 热区标识（走近浮现功能名与进入钮）；
## 「整图加载」条：相邻地点以传送点衔接。
## 由 world_topdown_v7.gd 注入 id/kind/label/dialogue_key/portal_id 后加入场景树。
## 距离判定走 _process（不依赖物理层），Area2D 的圆形形状只作热区范围标记与冒烟断言。

signal activated(hotspot_id: String)

var hotspot_id: String = ""
var kind: String = "door"          # door | portal
var label: String = ""
var radius: float = 70.0
var dialogue_key: String = ""      # 门洞：直接按 dialogue key 打开对话
var portal_id: String = ""         # 传送点：只把 id 交给服务端
var to: Vector2 = Vector2.ZERO     # 传送点：独立冒烟/实拍（离线）时的本地落点

var _near := false
var _plate: Label
var _prompt: Label
var _btn: Button

func _ready() -> void:
	add_to_group("hotspot")
	_build_visual()

func _build_visual() -> void:
	var accent := Color("D9A441") if kind == "door" else Color("6FC7C0")
	# 常驻名牌：功能名（文档要求「建筑以 UI 名牌标识」）。
	_plate = Label.new()
	_plate.name = "NamePlate"
	_plate.text = label if label != "" else "交互点"
	_plate.position = Vector2(-70, -152)
	_plate.custom_minimum_size = Vector2(128, 22)
	_plate.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_plate.add_theme_font_size_override("font_size", 13)
	_plate.add_theme_color_override("font_color", Color("F6EFDF"))
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.12, 0.10, 0.08, 0.78)
	sb.set_corner_radius_all(4)
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = accent
	_plate.add_theme_stylebox_override("normal", sb)
	add_child(_plate)

	# 走近才浮现：快捷键提示 + 可点「进入 / 前往」钮（文档「走近浮现功能名与进入钮」）。
	_prompt = Label.new()
	_prompt.name = "Prompt"
	_prompt.text = "E 进入" if kind == "door" else "E 前往"
	_prompt.position = Vector2(-90, -126)
	_prompt.custom_minimum_size = Vector2(180, 20)
	_prompt.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_prompt.add_theme_font_size_override("font_size", 12)
	_prompt.add_theme_color_override("font_color", Color("F6EFDF"))
	_prompt.add_theme_color_override("font_outline_color", Color("2B1C10"))
	_prompt.add_theme_constant_override("outline_size", 3)
	_prompt.visible = false
	add_child(_prompt)

	_btn = Button.new()
	_btn.name = "EnterButton"
	_btn.text = "进入" if kind == "door" else "前往"
	# 放在人物右上侧，避免盖住头顶；深褐底 + 金边 + 米色字与名牌同风格。
	_btn.position = Vector2(66, -120)
	_btn.custom_minimum_size = Vector2(76, 28)
	_btn.add_theme_font_size_override("font_size", 13)
	for state in ["normal", "hover", "pressed"]:
		var bs := StyleBoxFlat.new()
		bs.bg_color = Color(0.16, 0.12, 0.09, 0.92) if state == "normal" else Color(0.30, 0.20, 0.10, 0.96)
		bs.set_corner_radius_all(5)
		bs.border_width_left = 2
		bs.border_width_right = 2
		bs.border_width_top = 2
		bs.border_width_bottom = 2
		bs.border_color = accent
		_btn.add_theme_stylebox_override(state, bs)
	_btn.add_theme_color_override("font_color", Color("F6EFDF"))
	_btn.add_theme_color_override("font_hover_color", Color("FFE6A8"))
	_btn.visible = false
	_btn.pressed.connect(_on_enter_pressed)
	add_child(_btn)

func _process(_delta: float) -> void:
	var player := get_tree().get_first_node_in_group("player") as Node2D
	if player == null:
		return
	var near := global_position.distance_to(player.global_position) <= radius
	if near == _near:
		return
	_near = near
	_prompt.visible = near
	_btn.visible = near
	if near:
		WorldStore.set_near_hotspot(hotspot_id)
	else:
		WorldStore.clear_near_hotspot(hotspot_id)

func _on_enter_pressed() -> void:
	if _near:
		activated.emit(hotspot_id)

## 点击热区附近也可触发（与 NPC 交互同口径：鼠标位置落在范围内才受理）。
func _unhandled_input(event: InputEvent) -> void:
	if not _near:
		return
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		if get_global_mouse_position().distance_to(global_position) <= radius:
			activated.emit(hotspot_id)
