extends Node
## 布局回归冒烟：通过 main.tscn 实例化，验证战斗层始终跟随视口。
## 覆盖历史 bug：BattleLayer 挂在普通 Node 下 + 窗口尺寸变化被吞 → 只渲染左上角一小块。
## 运行：Godot --path client res://tests/layout_smoke.tscn --quit-after 400

func _ready() -> void:
	# 1) 极小窗口启动（历史 bug 的触发条件）。
	get_window().size = Vector2i(200, 320)
	await _frames(2)
	var main := preload("res://scenes/main.tscn").instantiate()
	add_child(main)
	await _frames(3)

	# 2) 结构性约束：战斗层必须在 CanvasLayer 下（否则锚点不跟视口）。
	if main.get_node_or_null("BattleCanvas/BattleLayer") == null:
		_fail("BattleLayer 不在 CanvasLayer 下（会退回左上角一小块的布局 bug）")
		return

	var battle: Control = main.get_node("BattleCanvas/BattleLayer")
	# 3) 恢复窗口后战斗层应自愈到视口大小。
	get_window().size = Vector2i(1092, 614)
	await _frames(4)
	main.get_node("UILayer").visible = true
	main.get_node("World").visible = true
	battle.visible = true
	battle.enter_battle({
		"battle_id": "layout", "turn_version": 0,
		"self": {"id": 1, "name": "雷震子", "side": 0, "hp": 300, "max_hp": 300, "mp": 100, "max_mp": 100},
		"enemy": {"id": 2, "name": "木桩人", "side": 1, "hp": 120, "max_hp": 120, "mp": 0, "max_mp": 0},
		"skills": [],
	})
	await _frames(10)
	var vp: Vector2 = get_viewport().get_visible_rect().size
	if absf(battle.size.x - vp.x) > 0.5 or absf(battle.size.y - vp.y) > 0.5:
		_fail("进战斗后未铺满视口: battle=%s viewport=%s" % [battle.size, vp])
		return
	if battle._bg == null or absf(battle._bg.size.x - vp.x) > 0.5:
		_fail("战斗背景未铺满: bg=%s" % [battle._bg.size if battle._bg else "nil"])
		return
	# 4) 再次缩窗，逐帧自愈应立刻跟上。
	get_window().size = Vector2i(800, 450)
	await _frames(6)
	vp = get_viewport().get_visible_rect().size
	if absf(battle.size.x - vp.x) > 0.5:
		_fail("缩窗后未自愈: battle=%s viewport=%s" % [battle.size, vp])
		return
	print("[layout-smoke] PASS 战斗层紧随视口（含缩窗/极小窗启动）battle=%s viewport=%s" % [battle.size, vp])
	get_tree().quit(0)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame

func _fail(msg: String) -> void:
	printerr("[layout-smoke] FAIL ", msg)
	get_tree().quit(1)
