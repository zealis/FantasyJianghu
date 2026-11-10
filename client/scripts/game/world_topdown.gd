extends Node2D
## 俯视 2D 整图试点 · 清河村 v3（UI 规范 V1.38 / 前端规范 §4）。
## 场景与逻辑分离：底图在 tscn，碰撞多边形 / NPC 点位 / 出生点 / 镜头边界全部读 JSON。
## 试点范围：整图一次加载、StaticBody2D + CollisionPolygon2D 碰撞层（桥面留口）、
## Y-Sort 深度、四向移动 + 双向镜头、原作式侧面像行走口径。
## 未接入（下一阶段）：点按避碰撞寻路、服务端 x/y 位置校验、NPC 对话与任务、明雷怪物、主流程切换。

const LOGIC := "res://data/qinghecun_v3_logic.json"

var logic: Dictionary = {}
var _hint: Label

func _ready() -> void:
	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(LOGIC))
	if not (parsed is Dictionary):
		push_error("[topdown] 逻辑数据读取失败: " + LOGIC)
		return
	logic = parsed
	_build_collisions()
	_build_npcs()
	_setup_spawn_and_camera()
	_build_hint()

func _build_collisions() -> void:
	var layer := get_node_or_null("Collisions") as Node2D
	if layer == null:
		layer = Node2D.new()
		layer.name = "Collisions"
		add_child(layer)
	for c in logic.get("collisions", []):
		var body := StaticBody2D.new()
		body.name = "Wall_" + str((c as Dictionary).get("name", ""))
		var poly := CollisionPolygon2D.new()
		var pts := PackedVector2Array()
		for xy in (c as Dictionary).get("polygon", []):
			pts.append(Vector2(float((xy as Array)[0]), float((xy as Array)[1])))
		poly.polygon = pts
		body.add_child(poly)
		layer.add_child(body)

func _build_npcs() -> void:
	var ys := get_node_or_null("YSortLayer") as Node2D
	if ys == null:
		return
	var idx := 0
	for n in logic.get("npcs", []):
		var info: Dictionary = n
		var node := Node2D.new()
		node.name = "NPC_" + str(info.get("id", ""))
		node.position = Vector2(float(info.get("x", 0)), float(info.get("y", 0)))
		var sp := Sprite2D.new()
		var tex := load("res://assets/sprites/player/idle_0.png") as Texture2D
		if tex != null:
			sp.texture = tex
			sp.scale = Vector2(0.5, 0.5)
			sp.position = Vector2(0, -70)
			# 试点占位：两个 NPC 用不同色调区分，正式立绘另出（不与精灵图混用）。
			sp.modulate = Color(0.82, 0.92, 1.0) if idx == 0 else Color(1.0, 0.90, 0.76)
			node.add_child(sp)
		var lab := Label.new()
		lab.text = str(info.get("name", "?"))
		lab.position = Vector2(-40, -118)
		lab.custom_minimum_size = Vector2(80, 20)
		lab.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
		lab.add_theme_font_size_override("font_size", 12)
		lab.add_theme_color_override("font_color", Color("F6EFDF"))
		var plate := StyleBoxFlat.new()
		plate.bg_color = Color(0.12, 0.10, 0.08, 0.78)
		plate.set_corner_radius_all(4)
		lab.add_theme_stylebox_override("normal", plate)
		node.add_child(lab)
		ys.add_child(node)
		idx += 1

func _setup_spawn_and_camera() -> void:
	var p := get_node_or_null("YSortLayer/Player") as CharacterBody2D
	if p == null:
		return
	var spawn: Array = logic.get("spawn", [430, 1080])
	p.position = Vector2(float(spawn[0]), float(spawn[1]))
	var cam := p.get_node_or_null("Camera") as Camera2D
	if cam != null:
		var c: Dictionary = logic.get("camera", {})
		cam.limit_left = int(c.get("left", 0))
		cam.limit_top = int(c.get("top", 0))
		cam.limit_right = int(c.get("right", 1920))
		cam.limit_bottom = int(c.get("bottom", 1280))

func _build_hint() -> void:
	var canvas := CanvasLayer.new()
	add_child(canvas)
	_hint = Label.new()
	_hint.add_theme_font_size_override("font_size", 15)
	_hint.add_theme_color_override("font_color", Color("F6EFDF"))
	var panel := PanelContainer.new()
	panel.position = Vector2(12, 10)
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.12, 0.10, 0.08, 0.80)
	sb.set_corner_radius_all(6)
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	panel.add_theme_stylebox_override("panel", sb)
	panel.add_child(_hint)
	canvas.add_child(panel)
	_refresh_hint()

func _refresh_hint() -> void:
	if _hint == null:
		return
	var p := get_node_or_null("YSortLayer/Player") as CharacterBody2D
	var xy := p.position if p != null else Vector2.ZERO
	_hint.text = "清河村 · 俯视 2D 试点 | WASD/方向键 四向移动 | 左键点按 | Esc 取消 | 坐标 (%d, %d)" % [int(xy.x), int(xy.y)]

func _process(_delta: float) -> void:
	_refresh_hint()
