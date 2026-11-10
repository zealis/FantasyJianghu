extends Node2D
## 俯视主世界 NPC（承接横版 npc.gd 的对话 / 任务记账交互，改为 x/y 四向距离）。
## 对话内容仍由 SceneLogic 当前逻辑数据的 dialogues 驱动，本脚本只负责「靠近 + 触发」。
## 由 world_topdown_v7.gd 注入 npc_id / cfg / render 后加入场景树。

var npc_id: String = ""
var cfg: Dictionary = {}
var render: Dictionary = {}
var _prompt: Label
var _near := false

func _ready() -> void:
	add_to_group("npc")
	_build_visual()

func _build_visual() -> void:
	var sc := float(render.get("sprite_scale", 0.28))
	var oy := float(render.get("sprite_offset_y", -32))
	var body := Sprite2D.new()
	body.texture = load("res://assets/sprites/player/idle_0.png")
	body.scale = Vector2(sc, sc)
	body.position = Vector2(0, oy)
	body.modulate = Color(0.82, 0.92, 1.0) if npc_id == "hebo" else Color(1.0, 0.90, 0.76)
	add_child(body)

	var plate := Label.new()
	plate.text = str(cfg.get("name", "NPC"))
	plate.position = Vector2(-40, -96)
	plate.custom_minimum_size = Vector2(80, 20)
	plate.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	plate.add_theme_font_size_override("font_size", 12)
	plate.add_theme_color_override("font_color", Color("F6EFDF"))
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.12, 0.10, 0.08, 0.78)
	sb.set_corner_radius_all(4)
	plate.add_theme_stylebox_override("normal", sb)
	add_child(plate)

	if str(cfg.get("dialogue", "")) != "":
		var marker := Label.new()
		marker.text = "!"
		marker.position = Vector2(-4, -120)
		marker.add_theme_font_size_override("font_size", 18)
		marker.add_theme_color_override("font_color", Color("D9A441"))
		add_child(marker)

	_prompt = Label.new()
	_prompt.text = "E 对话"
	_prompt.position = Vector2(-30, -74)
	_prompt.add_theme_font_size_override("font_size", 12)
	_prompt.add_theme_color_override("font_color", Color("F6EFDF"))
	_prompt.add_theme_color_override("font_outline_color", Color("2B1C10"))
	_prompt.add_theme_constant_override("outline_size", 3)
	_prompt.visible = false
	add_child(_prompt)

func _process(_delta: float) -> void:
	var player := get_tree().get_first_node_in_group("player") as Node2D
	if player == null:
		return
	var near := global_position.distance_to(player.global_position) < 110.0
	if near == _near:
		return
	_near = near
	_prompt.visible = near
	if near:
		WorldStore.set_near_npc(npc_id)
	else:
		WorldStore.clear_near_npc(npc_id)

func _unhandled_input(event: InputEvent) -> void:
	if not _near:
		return
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		if get_global_mouse_position().distance_to(global_position) < 130.0:
			WorldStore.request_dialogue(npc_id)
