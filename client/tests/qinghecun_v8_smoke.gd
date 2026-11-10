extends Node
## 清河村扩张版13（1920×1280）无头冒烟：V1.43 明雷节奏 + 用户手画 2 组障碍块。
## 运行：Godot_v4.7.2-stable_win64_console.exe --headless --path client res://tests/qinghecun_v8_smoke.tscn

const Scene := preload("res://scenes/world/qinghecun_v8.tscn")

func _inside(walls: Node, pt: Vector2) -> bool:
	for c in walls.get_children():
		if not (c is StaticBody2D) or c.get_child_count() == 0:
			continue
		var shape: Node = c.get_child(0)
		if shape is CollisionPolygon2D and Geometry2D.is_point_in_polygon(pt, (shape as CollisionPolygon2D).polygon):
			return true
	return false

func _ready() -> void:
	var w := Scene.instantiate()
	add_child(w)
	await get_tree().physics_frame
	await get_tree().process_frame

	var map := w.get_node_or_null("Map") as Sprite2D
	if map == null or map.texture == null:
		return _fail("扩张版底图未加载")
	var sz: Vector2 = map.texture.get_size()
	if not (absf(sz.x - 1920.0) < 1.0 and absf(sz.y - 1280.0) < 1.0):
		return _fail("底图尺寸不符（应 1920×1280）: %s" % str(sz))
	if map.get_parent() != w:
		return _fail("整图底图不应参与 Y-Sort 排序")
	if str(w.get_meta("logic_path", "")) != "res://data/qinghecun_v8_logic.json":
		return _fail("场景未指向 v8 逻辑数据")

	# 用户手画：2 组 wall_base 凹多边形，Godot BUILD_SOLIDS 凸分解。
	var walls: Node = w.get_node_or_null("Collisions")
	if walls == null or walls.get_child_count() != 2:
		return _fail("手画障碍块数量不符（应 2）: %d" % (walls.get_child_count() if walls else -1))
	for c in walls.get_children():
		if not (c is StaticBody2D) or c.get_child_count() == 0 or not (c.get_child(0) is CollisionPolygon2D):
			return _fail("碰撞体不是 StaticBody2D + CollisionPolygon2D")
		if str(c.get_meta("kind", "")) != "wall_base":
			return _fail("碰撞 kind 不符: %s" % str(c.get_meta("kind", "")))

	var doors: Node = w.get_node_or_null("Interactions")
	if doors == null or doors.get_child_count() != 3:
		return _fail("门洞交互热区数量不符")
	var ys: Node2D = w.get_node_or_null("YSortLayer") as Node2D
	if ys == null or not ys.y_sort_enabled:
		return _fail("精灵间 Y-Sort 未启用")
	if ys.get_node_or_null("Monster_wolf_1") == null:
		return _fail("明雷怪物未生成")

	var p := ys.get_node_or_null("Player") as CharacterBody2D
	if p == null:
		return _fail("玩家节点缺失")
	var cam := p.get_node_or_null("Camera") as Camera2D
	if cam == null or absf(cam.zoom.x - 1.6) > 0.01 or cam.limit_right != 1920 or cam.limit_bottom != 1280:
		return _fail("镜头 zoom 1.6 / 1920×1280 边界未设置")
	var spr := p.get_node_or_null("BodySprite") as AnimatedSprite2D
	if spr == null or absf(spr.scale.x - 0.28) > 0.01:
		return _fail("人物缩放未按门洞锚 0.28 生效")
	if absf(p.speed - 200.0) > 0.01:
		return _fail("移速未按 200 图内 px/s 生效")
	if p.position.distance_to(Vector2(950, 780)) > 1.0:
		return _fail("出生点不符")

	# 静态几何：河面 / 农田 / 建筑 / 山体必须落在障碍内。
	var blocked_pts := [Vector2(150, 1000), Vector2(1700, 200), Vector2(420, 380),
			Vector2(300, 1100), Vector2(1400, 950), Vector2(1800, 1100),
			Vector2(1900, 700), Vector2(100, 600)]
	for pt in blocked_pts:
		if not _inside(walls, pt):
			return _fail("不可走点未被障碍覆盖: %s" % str(pt))
	# 广场 / NPC / 门洞 / 怪物 / 传送点不得被误封。
	var open_pts := [Vector2(950, 780), Vector2(960, 700), Vector2(940, 560),
			Vector2(780, 680), Vector2(1250, 700), Vector2(1130, 452),
			Vector2(1484, 306), Vector2(1352, 1014), Vector2(600, 850), Vector2(468, 972)]
	for pt in open_pts:
		if _inside(walls, pt):
			return _fail("可走点被误封为障碍: %s" % str(pt))

	# V1.43 明雷节奏：静止为主，每 3–4s 才横移一次约 1s / 40px。
	var wolf := ys.get_node_or_null("Monster_wolf_1") as Node2D
	var x0 := wolf.position.x
	for i in range(60):
		await get_tree().process_frame
	if absf(wolf.position.x - x0) > 2.0:
		return _fail("V1.43 明雷怪物应静止为主，不应持续游走")
	wolf.set("idle_min", 0.1)
	wolf.set("idle_max", 0.2)
	wolf.set("move_time", 0.3)
	wolf.set("_timer", 0.05)
	for i in range(90):
		await get_tree().process_frame
	if absf(wolf.position.x - x0) < 5.0:
		return _fail("V1.43 明雷怪物间歇横移未生效")
	if absf(wolf.position.x - x0) > 61.0:
		return _fail("明雷怪物越出移动范围")
	if wolf.get_node_or_null("Sprite2D") == null or wolf.get_node_or_null("BodySprite") != null:
		return _fail("明雷怪物不应带动作帧")

	# 四向移动（广场开阔点 940,560，四周 80px 无障碍）。
	for case in [{"dir": Vector2.RIGHT, "axis": "x", "sign": 1.0},
			{"dir": Vector2.LEFT, "axis": "x", "sign": -1.0},
			{"dir": Vector2.DOWN, "axis": "y", "sign": 1.0},
			{"dir": Vector2.UP, "axis": "y", "sign": -1.0}]:
		p.position = Vector2(940, 560)
		p.input_override = case["dir"]
		for i in range(15):
			await get_tree().physics_frame
		var delta: float = (p.position.x - 940.0) if case["axis"] == "x" else (p.position.y - 560.0)
		if delta * float(case["sign"]) < 15.0:
			return _fail("四向移动失效: dir=%s delta=%.1f" % [str(case["dir"]), delta])
	p.input_override = Vector2.ZERO

	# 广场内点按可达：spawn → 教头旁 (1250,700)。
	p.position = Vector2(950, 780)
	p.set("_target", Vector2(1250, 700))
	for i in range(100):
		await get_tree().physics_frame
	p.set("_target", Vector2.INF)
	if p.position.distance_to(Vector2(1250, 700)) > 60.0:
		return _fail("广场内点按未到达目标: %s" % str(p.position))
	if _inside(walls, p.position):
		return _fail("广场通行途中穿进了障碍: %s" % str(p.position))

	# 点按河面 / 山体不得把人送进障碍（扩张版西南方向）。
	p.position = Vector2(950, 780)
	p.set("_target", Vector2(150, 1000))
	for i in range(120):
		await get_tree().physics_frame
	p.set("_target", Vector2.INF)
	if _inside(walls, p.position):
		return _fail("点按障碍区把人物送了进去: %s" % str(p.position))
	if p.position.x < 500.0:
		return _fail("西南障碍边界未挡住人物: %s" % str(p.position))

	print("[qinghecun_v8] PASS 扩张版13 1920×1280 / 手画2组障碍 / V1.43 明雷静止节奏 / 广场通行 / 障碍阻挡 / 门洞热区 / Y-Sort / zoom 1.6")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[qinghecun_v8] FAIL: " + msg)
	get_tree().quit(1)
