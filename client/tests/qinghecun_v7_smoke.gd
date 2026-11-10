extends Node
## 2.5D 斜俯视定稿图试点无头冒烟（用户 2026-11-11 手画「整片障碍块」口径）。
## 运行：Godot_v4.7.2-stable_win64_console.exe --headless --path client res://tests/qinghecun_v7_smoke.tscn

const Scene := preload("res://scenes/world/qinghecun_v7.tscn")

## 点是否落在任一碰撞多边形内（静态几何断言，不依赖物理帧）。
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
		return _fail("2.5D 定稿底图未加载")
	var sz: Vector2 = map.texture.get_size()
	if not (absf(sz.x - 1536.0) < 1.0 and absf(sz.y - 1024.0) < 1.0):
		return _fail("底图尺寸不符（应 1536×1024）: %s" % str(sz))
	if map.get_parent() != w:
		return _fail("整图底图不应参与 Y-Sort 排序")

	# 手画口径：4 组 wall_base 凹多边形，Godot 用 BUILD_SOLIDS 做凸分解。
	var walls: Node = w.get_node_or_null("Collisions")
	if walls == null or walls.get_child_count() != 4:
		return _fail("手画障碍块数量不符（应 4）: %d" % (walls.get_child_count() if walls else -1))
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
	if cam == null or absf(cam.zoom.x - 1.6) > 0.01 or cam.limit_right != 1536:
		return _fail("镜头 zoom 1.6 / 边界未设置")
	var spr := p.get_node_or_null("BodySprite") as AnimatedSprite2D
	if spr == null or absf(spr.scale.x - 0.28) > 0.01:
		return _fail("人物缩放未按门洞锚 0.28 生效")
	if absf(p.speed - 200.0) > 0.01:
		return _fail("移速未按 200 图内 px/s 生效")
	if p.position.distance_to(Vector2(520, 700)) > 1.0:
		return _fail("出生点不符")

	# 静态几何：不可走点必须全部落在障碍内（屋顶 / 桥拱水面 / 下游河面 / 民居）。
	var blocked_pts := [Vector2(660, 160), Vector2(1350, 250), Vector2(871, 559),
			Vector2(1048, 722), Vector2(100, 100), Vector2(100, 700), Vector2(430, 620)]
	for pt in blocked_pts:
		if not _inside(walls, pt):
			return _fail("不可走点未被障碍覆盖: %s" % str(pt))
	# 可走点（出生点 / 桥面 / 门洞 / NPC / 传送点）不得被误封。
	var open_pts := [Vector2(520, 700), Vector2(640, 560), Vector2(871, 470),
			Vector2(600, 470), Vector2(1120, 520), Vector2(1330, 560),
			Vector2(200, 1000), Vector2(224, 977), Vector2(538, 413), Vector2(430, 636)]
	for pt in open_pts:
		if _inside(walls, pt):
			return _fail("可走点被误封为障碍: %s" % str(pt))

	# 明雷怪物（V1.43 定案）：静止为主，每 3–4s 才横移一次约 1s / 40px，移完即停。
	var wolf := ys.get_node_or_null("Monster_wolf_1") as Node2D
	var x0 := wolf.position.x
	# 无头模式下 SceneTreeTimer 不推进 _process，改用 process_frame 累计帧数。
	for i in range(60):
		await get_tree().process_frame
	if absf(wolf.position.x - x0) > 2.0:
		return _fail("V1.43 明雷怪物应静止为主，不应持续游走")
	# 注入小间隔加速验证「横移一次且不出范围」，避免冒烟空等 4 秒。
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

	# 四向移动（开阔空地 640,560，四周 70px 无障碍）。
	for case in [{"dir": Vector2.RIGHT, "axis": "x", "sign": 1.0},
			{"dir": Vector2.LEFT, "axis": "x", "sign": -1.0},
			{"dir": Vector2.DOWN, "axis": "y", "sign": 1.0},
			{"dir": Vector2.UP, "axis": "y", "sign": -1.0}]:
		p.position = Vector2(640, 560)
		p.input_override = case["dir"]
		for i in range(15):
			await get_tree().physics_frame
		var delta: float = (p.position.x - 640.0) if case["axis"] == "x" else (p.position.y - 560.0)
		if delta * float(case["sign"]) < 15.0:
			return _fail("四向移动失效: dir=%s delta=%.1f" % [str(case["dir"]), delta])
	p.input_override = Vector2.ZERO

	# 斜向沿石板路过桥：可走路是斜的，不是固定 y 的水平线。
	# (520,700) + dir(0.8321,-0.5547) × 125 帧 ≈ (867,469) 桥面。
	p.position = Vector2(520, 700)
	p.input_override = Vector2(0.8321, -0.5547)
	for i in range(125):
		await get_tree().physics_frame
	p.input_override = Vector2.ZERO
	if p.position.x < 800.0 or p.position.y > 520.0:
		return _fail("斜向石板路未走到桥面: %s" % str(p.position))
	if _inside(walls, p.position):
		return _fail("过桥途中穿进了障碍: %s" % str(p.position))

	# 站桥面朝南走，不得掉进桥拱洞口的水面（障碍上边界 y≈498）。
	p.position = Vector2(871, 470)
	p.input_override = Vector2.DOWN
	for i in range(50):
		await get_tree().physics_frame
	p.input_override = Vector2.ZERO
	# 斜向障碍边界会让 move_and_slide 沿边滑到桥西陆地，属正常物理行为；
	# 判据是「没进障碍、没滑进桥拱洞口那片水面」，而不是死守某个 y 阈值。
	if _inside(walls, p.position):
		return _fail("桥面南侧障碍未挡住（人物进了障碍）: %s" % str(p.position))
	if p.position.x > 800.0 and p.position.x < 1000.0 and p.position.y > 520.0:
		return _fail("人物滑进了桥拱洞口水面区: %s" % str(p.position))
	if p.position.distance_to(Vector2(871, 470)) > 180.0:
		return _fail("桥面南侧出现异常位移: %s" % str(p.position))

	# 点按下游河面也不得把人送进水里。
	p.position = Vector2(1300, 900)
	p.set("_target", Vector2(1048, 722))
	for i in range(80):
		await get_tree().physics_frame
	p.set("_target", Vector2.INF)
	if _inside(walls, p.position):
		return _fail("点按河面目标把人物送进了障碍: %s" % str(p.position))
	if p.position.distance_to(Vector2(1048, 722)) < 80.0:
		return _fail("点按河面目标被走到了水面点附近: %s" % str(p.position))

	print("[qinghecun_v7] PASS 手画4组障碍块 / 屋顶与水面不可达 / 斜向石板路过桥 / 可走点未误封 / 门洞热区 / Y-Sort / zoom 1.6 / 人物 0.28 / 明雷横移")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[qinghecun_v7] FAIL: " + msg)
	get_tree().quit(1)
