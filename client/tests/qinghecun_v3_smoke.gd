extends Node
## 俯视 2D 整图试点无头冒烟：逻辑数据 / 碰撞层 / 桥面留口 / 四向移动 / Y-Sort / 镜头边界。
## 运行：Godot_v4.7.2-stable_win64_console.exe --headless --path client res://tests/qinghecun_v3_smoke.tscn
## 退出码 0=通过，1=失败。

const Scene := preload("res://scenes/world/qinghecun_v3.tscn")

func _ready() -> void:
	var w := Scene.instantiate()
	add_child(w)
	await get_tree().physics_frame
	await get_tree().physics_frame

	var map := w.get_node_or_null("Map") as Sprite2D
	if map == null or map.texture == null:
		return _fail("俯视底图未加载")
	var tex_size: Vector2 = map.texture.get_size()
	if not (absf(tex_size.x - 1920.0) < 1.0 and absf(tex_size.y - 1280.0) < 1.0):
		return _fail("底图尺寸不符: %s" % str(tex_size))

	var walls: Node = w.get_node_or_null("Collisions")
	if walls == null or walls.get_child_count() < 10:
		return _fail("碰撞多边形层未构建: %d" % (walls.get_child_count() if walls else -1))
	for c in walls.get_children():
		if not (c is StaticBody2D) or c.get_child_count() == 0 or not (c.get_child(0) is CollisionPolygon2D):
			return _fail("碰撞体不是 StaticBody2D + CollisionPolygon2D")

	var ys: Node2D = w.get_node_or_null("YSortLayer") as Node2D
	if ys == null or not ys.y_sort_enabled:
		return _fail("Y-Sort 深度层未启用")

	var p := ys.get_node_or_null("Player") as CharacterBody2D
	if p == null:
		return _fail("玩家节点缺失")
	var cam := p.get_node_or_null("Camera") as Camera2D
	if cam == null or cam.limit_right != 1920 or cam.limit_bottom != 1280:
		return _fail("双向镜头边界未设置")
	if p.position.distance_to(Vector2(430, 1080)) > 1.0:
		return _fail("出生点不符: %s" % str(p.position))

	# 四向移动：注入输入，走 15 个物理帧，验证方向。
	for case in [{"dir": Vector2.RIGHT, "axis": "x", "sign": 1.0},
			{"dir": Vector2.LEFT, "axis": "x", "sign": -1.0},
			{"dir": Vector2.DOWN, "axis": "y", "sign": 1.0},
			{"dir": Vector2.UP, "axis": "y", "sign": -1.0}]:
		p.position = Vector2(1200, 950)  # 河东农田空地，四周无障碍
		p.input_override = case["dir"]
		for i in range(15):
			await get_tree().physics_frame
		var delta: float = (p.position.x - 1200.0) if case["axis"] == "x" else (p.position.y - 950.0)
		if delta * float(case["sign"]) < 20.0:
			return _fail("四向移动失效: dir=%s delta=%.1f" % [str(case["dir"]), delta])
	p.input_override = Vector2.ZERO

	# 桥面留口：从主街沿横街向东应能穿过石桥（x 越过 900）。
	p.position = Vector2(430, 755)
	p.input_override = Vector2.RIGHT
	for i in range(130):
		await get_tree().physics_frame
	if p.position.x < 900.0:
		return _fail("桥面未留通行口（被河流碰撞挡住）: x=%.1f" % p.position.x)
	p.input_override = Vector2.ZERO

	# 河流阻挡：桥中央向北/向南都应被河段挡住。
	p.position = Vector2(950, 755)
	p.input_override = Vector2.UP
	for i in range(40):
		await get_tree().physics_frame
	if p.position.y < 700.0:
		return _fail("河流北段未阻挡: y=%.1f" % p.position.y)
	p.input_override = Vector2.DOWN
	for i in range(60):
		await get_tree().physics_frame
	if p.position.y > 810.0:
		return _fail("河流南段未阻挡: y=%.1f" % p.position.y)
	p.input_override = Vector2.ZERO

	print("[qinghecun_v3] PASS 俯视整图 / 12 组碰撞多边形 / 桥面留口 / 四向移动 / Y-Sort / 双向镜头")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[qinghecun_v3] FAIL: " + msg)
	get_tree().quit(1)
