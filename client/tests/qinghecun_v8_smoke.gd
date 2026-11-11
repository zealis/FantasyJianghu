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
	var door_nodes: Array = []
	var portal_nodes: Array = []
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

	# 碰撞：用户手画 2 组整片障碍 + 2 组布棚摊位占地投影（2026-11-11 精修）。
	var walls: Node = w.get_node_or_null("Collisions")
	if walls == null or walls.get_child_count() != 4:
		return _fail("碰撞块数量不符（应 2 手画 + 2 摊位）: %d" % (walls.get_child_count() if walls else -1))
	for c in walls.get_children():
		if not (c is StaticBody2D) or c.get_child_count() == 0 or not (c.get_child(0) is CollisionPolygon2D):
			return _fail("碰撞体不是 StaticBody2D + CollisionPolygon2D")
		if str(c.get_meta("kind", "")) != "wall_base":
			return _fail("碰撞 kind 不符: %s" % str(c.get_meta("kind", "")))

	var doors: Node = w.get_node_or_null("Interactions")
	if doors == null or doors.get_child_count() != 4:
		return _fail("交互热区数量不符（应 3 门洞 + 1 传送点）")
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
			Vector2(1900, 700), Vector2(100, 600),
			# 布棚摊位篷顶 / 菜筐不得可站（用户 2026-11-11 实拍反馈）。
			Vector2(555, 730), Vector2(550, 800), Vector2(680, 775), Vector2(680, 830),
			Vector2(680, 877)]
	for pt in blocked_pts:
		if not _inside(walls, pt):
			return _fail("不可走点未被障碍覆盖: %s" % str(pt))
	# 广场 / NPC / 门洞 / 怪物 / 传送点不得被误封。
	var open_pts := [Vector2(950, 780), Vector2(960, 700), Vector2(940, 560),
			Vector2(780, 680), Vector2(1250, 700), Vector2(1130, 452),
			Vector2(1484, 306), Vector2(1352, 1014), Vector2(600, 850), Vector2(468, 972),
			# 摊位前土路 / 广场石板不得被误封。
			Vector2(600, 870), Vector2(680, 900), Vector2(700, 700), Vector2(1500, 700)]
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

	# --- 战斗结束处置（V1.43 击杀重刷）：胜利隐藏 → 原区重刷；失败短冷却 ---
	var respawn_bak := float(wolf.get("respawn_seconds"))
	wolf.set("respawn_seconds", 0.2)
	wolf.call("on_battle_result", true)
	if wolf.visible or float(wolf.get("_respawn_left")) <= 0.0:
		return _fail("战斗胜利后明雷怪物应隐藏并进入重刷倒计时")
	for i in range(45):
		await get_tree().process_frame
	if not wolf.visible:
		return _fail("明雷怪物未按间隔在原区重刷")
	wolf.call("on_battle_result", false)
	if float(wolf.get("_cooldown")) < 4.0 or not wolf.visible:
		return _fail("战斗未击杀时怪物应保留并进入短冷却")
	wolf.set("respawn_seconds", respawn_bak)
	wolf.set("_cooldown", 0.0)

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

	# --- 门洞 / 传送点交互（文档 §4：名牌 + 走近浮现进入钮）---
	var hotspots := doors.get_children()
	door_nodes = []
	portal_nodes = []
	for h in hotspots:
		if str(h.get("kind")) == "door":
			door_nodes.append(h)
		else:
			portal_nodes.append(h)
		if h.get_node_or_null("NamePlate") == null or h.get_node_or_null("EnterButton") == null:
			return _fail("热区缺 UI 名牌或进入钮: %s" % h.name)
		if (h.get_node("EnterButton") as Button).visible:
			return _fail("未靠近时进入钮不应显示: %s" % h.name)
	if door_nodes.size() != 3 or portal_nodes.size() != 1:
		return _fail("门洞/传送点分类不符: %d / %d" % [door_nodes.size(), portal_nodes.size()])

	# 走近村中大宅 → 提示与进入钮浮现、交互焦点写入 WorldStore。
	p.position = Vector2(1130, 452)
	for i in range(3):
		await get_tree().process_frame
	var d0 = door_nodes[0]
	if str(d0.get("hotspot_id")) != "door_dazhai":
		return _fail("门洞 id 不符: %s" % str(d0.get("hotspot_id")))
	if not (d0.get_node("EnterButton") as Button).visible or not (d0.get_node("Prompt") as Label).visible:
		return _fail("走近门洞未浮现功能名与进入钮")
	if WorldStore.near_hotspot != "door_dazhai":
		return _fail("交互焦点未写入 WorldStore.near_hotspot: %s" % WorldStore.near_hotspot)

	# 触发门洞 → 门内对话按 dialogue key 打开（不误发 10/1 talk 记账）。
	var seen: Array = []
	WorldStore.dialogue_requested.connect(func(k: String) -> void: seen.append(k))
	WorldStore.request_hotspot("door_dazhai")
	await get_tree().process_frame
	if seen.size() != 1 or str(seen[0]) != "door_dazhai":
		return _fail("门洞未打开门内对话: %s" % str(seen))
	var dlg: Dictionary = (w as Node).get("logic")
	if not dlg.get("dialogues", {}).has("door_dazhai"):
		return _fail("v8 逻辑缺 door_dazhai 门内对话")

	# 传送点 → 独立场景不上网，按配置本地落点（主流程走 3/9 服务端权威落点）。
	WorldStore.request_hotspot("field_gate")
	for i in range(4):
		await get_tree().process_frame
	if p.position.distance_to(Vector2(1500, 700)) > 1.0:
		return _fail("传送点未落到配置点: %s" % str(p.position))
	if WorldStore.near_hotspot == "field_gate":
		return _fail("传送后交互焦点未刷新")
	print("[qinghecun_v8] PASS 门洞热区交互 / 传送点落点 / 名牌与进入钮")

	print("[qinghecun_v8] PASS 扩张版13 / 2组手画障碍 + 2组摊位碰撞 / 篷顶不可站 / V1.43 明雷节奏 + 击杀重刷 / 门洞热区 / 传送点 / Y-Sort / zoom 1.6")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[qinghecun_v8] FAIL: " + msg)
	get_tree().quit(1)
