extends Node2D
## 渲染层 · NPC（前端规范 §4 交互）。靠近触发对话按钮/快捷键；任务标记分级。
## 对话内容读场景逻辑 JSON（剧情基线），不硬编码。

@export var npc_id: String = "hebo"
var _name_plate: Label
var _prompt: Label
var _cfg: Dictionary = {}

func _ready() -> void:
	add_to_group("npc")
	var logic := SceneLogic.get_logic()
	for n in logic.get("npcs", []):
		if n.get("id") == npc_id:
			_cfg = n
	position = Vector2(float(_cfg.get("x", 700)), SceneLogic.ground_y())
	_build_visual()

func _build_visual() -> void:
	var body := Sprite2D.new()
	body.texture = load("res://assets/sprites/player/idle_0.png")
	body.scale = Vector2(0.5, 0.5)
	body.position = Vector2(0, -75)
	body.modulate = Color(0.8, 0.7, 0.55)
	add_child(body)
	_name_plate = Label.new()
	_name_plate.text = String(_cfg.get("name", "NPC"))
	_name_plate.position = Vector2(-40, -170)
	_name_plate.add_theme_color_override("font_color", Color("3D2C1C"))
	add_child(_name_plate)
	var marker := Label.new()
	marker.text = "!"
	marker.position = Vector2(-6, -205)
	marker.add_theme_color_override("font_color", Color("D9A441"))
	marker.add_theme_font_size_override("font_size", 28)
	add_child(marker)
	_prompt = Label.new()
	_prompt.text = "E 对话"
	_prompt.position = Vector2(-30, -140)
	_prompt.add_theme_color_override("font_color", Color("DE6423"))
	_prompt.visible = false
	add_child(_prompt)

func _process(_delta: float) -> void:
	var player := get_tree().get_first_node_in_group("player") as Node2D
	if player == null:
		return
	var near := absf(player.position.x - position.x) < 120.0
	# 每个 NPC 都会写全局 near_npc_id；旧实现「不近就写空串」会让后处理的
	# NPC 清掉前一个 NPC 刚写入的值，导致 E 键对话几乎永远失效。
	if near:
		WorldStore.set_near_npc(npc_id)
	else:
		WorldStore.clear_near_npc(npc_id)
	_prompt.visible = near

func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		if _prompt.visible and get_global_mouse_position().distance_to(global_position) < 160.0:
			WorldStore.request_dialogue(npc_id)