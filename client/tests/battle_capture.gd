extends Node
## 生成战斗界面实拍图：真实 GUI 窗口渲染（非 headless），不依赖桌面输入注入/MCP。
## 运行：
##   Godot_v4.7.2-stable_win64_console.exe --path client res://tests/battle_capture.tscn
## 产物：client/tests/battle_capture.png（1v1）与 battle_capture_4v4.png（M2a-2 4v4 演练）

const OUT_1V1 := "D:/project/Fantasy-Jianghu/client/tests/battle_capture.png"
const OUT_4V4 := "D:/project/Fantasy-Jianghu/client/tests/battle_capture_4v4.png"
const OUT_TARGET := "D:/project/Fantasy-Jianghu/client/tests/battle_target_select.png"
const OUT_FLOAT := "D:/project/Fantasy-Jianghu/client/tests/battle_float_text.png"
const OUT_BG := "D:/project/Fantasy-Jianghu/client/tests/battle_bg_linyuanzhen.png"

func _ready() -> void:
	await get_tree().process_frame
	var layer := preload("res://scenes/battle/battle.tscn").instantiate()
	add_child(layer)

	# --- 1v1 定稿画面 ---
	layer.enter_battle({
		"battle_id": "capture-1",
		"turn_version": 0,
		"self": {"id": 1, "name": "雷震子", "side": 0, "hp": 320, "max_hp": 320, "mp": 180, "max_mp": 180},
		"enemy": {"id": 2, "name": "木桩人", "side": 1, "hp": 200, "max_hp": 200, "mp": 0, "max_mp": 0},
		"skills": [{"id": "tianlei_po", "name": "天雷破", "category": "单体", "mp_cost": 150, "level_req": 10}],
	})
	layer._play_event({"type": "order", "flags": {
		"ids": [1, 2], "names": ["雷震子", "木桩人"], "slots": [0, 0]}})
	layer._play_event({"type": "act", "actor": 1, "target": 2, "flags": {"skill": "天雷破"}})
	layer._float_text(layer._units.get(2), "37", Color("C0491B"))
	if not await _capture(OUT_1V1, layer):
		return

	# --- M2a-2 4v4 演练画面（站位 / 行动顺序条 / 感电 / 反伤 / 目标黄圈） ---
	var units: Array = []
	for i in range(4):
		units.append({"id": 100 + i, "name": "同伴%d" % (i + 1) if i > 0 else "雷震子",
			"side": 0, "slot": i, "hp": 760, "max_hp": 900, "mp": 180, "max_mp": 220, "reflect_pct": 0})
	var enemy_names := ["木桩人", "野狼", "山贼", "荆棘武士"]
	for i in range(4):
		units.append({"id": 200 + i, "name": enemy_names[i], "side": 1, "slot": i,
			"hp": 700 - i * 60, "max_hp": 700, "mp": 0, "max_mp": 0,
			"reflect_pct": 900 if i == 3 else 0})
	await get_tree().process_frame
	layer.enter_battle({
		"battle_id": "capture-4v4", "turn_version": 0, "mode": "drill", "units": units,
		"skills": [{"id": "tianlei_po", "name": "天雷破", "category": "单体", "mp_cost": 150, "level_req": 10, "targets": 1}],
	})
	layer._play_event({"type": "order", "flags": {
		"ids": [100, 101, 102, 103, 200, 201, 202, 203],
		"names": ["雷震子", "同伴2", "同伴3", "同伴4", "木桩人", "野狼", "山贼", "荆棘武士"],
		"slots": [0, 1, 2, 3, 0, 1, 2, 3]}})
	layer._play_event({"type": "act", "actor": 100, "target": 203, "flags": {"skill": "怒雷天罚"}})
	layer._play_event({"type": "shock", "actor": 100, "target": 203, "flags": {"stacks": 2, "max": 3}})
	layer._float_text(layer._units.get(203), "37", Color("C0491B"))
	layer._play_event({"type": "reflect", "actor": 203, "target": 202, "value": 12})
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img4 := get_viewport().get_texture().get_image()
	if img4 == null or img4.is_empty():
		push_error("[capture] 4v4 viewport image is empty")
		get_tree().quit(1)
		return
	var err4 := img4.save_png(OUT_4V4)
	print("[capture] saved=", OUT_4V4, " size=", img4.get_size(), " err=", err4,
		" units=", layer._units.size(), " order=", layer._order_names.size())
	# --- 两段式出招：点招后进入待选目标（4 个敌方光圈 + 提示），尚未发动 ---
	layer._cmd_panel._submit("skill", "tianlei_po")
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img5 := get_viewport().get_texture().get_image()
	if img5 != null and not img5.is_empty():
		var err5 := img5.save_png(OUT_TARGET)
		print("[capture] saved=", OUT_TARGET, " err=", err5,
			" pending=", layer._cmd_panel.is_pending(),
			" hint=", layer._cmd_panel._hint_label.text)

	# --- V1.28 飘字展示：加深 + 描边/阴影 + 特殊效果单字前缀（合成事件演示，不伪造战报） ---
	layer._clear_pending()
	layer._render_targets()
	# 清掉上一组仍在停留的飘字，保证展示图干净。
	for uid in layer._units:
		for c in layer._units[uid].get_children():
			if c is Label and c.name.begins_with("FloatText"):
				c.queue_free()
	await get_tree().process_frame
	await get_tree().process_frame
	layer._play_event({"type": "act", "actor": 100, "target": 200, "flags": {"skill": "天雷破"}})
	layer._play_event({"type": "damage", "actor": 100, "target": 200, "value": 128, "flags": {"crit": true}})
	layer._play_event({"type": "damage", "actor": 100, "target": 201, "value": 96,
		"flags": {"lifesteal": 20, "combo": true}})
	layer._play_event({"type": "damage", "actor": 100, "target": 202, "value": 74, "flags": {"magic": true}})
	layer._play_event({"type": "dot", "actor": 100, "target": 203, "value": 18})
	# 等出招位移回位，同时验证飘字停留 > 1s 仍可见。
	await get_tree().create_timer(0.8).timeout
	await RenderingServer.frame_post_draw
	var img6 := get_viewport().get_texture().get_image()
	if img6 != null and not img6.is_empty():
		print("[capture] saved=", OUT_FLOAT, " err=", img6.save_png(OUT_FLOAT))

	# --- 按地点切换战斗背景（映射表驱动）：清掉飘字后切到临渊镇 ---
	for uid2 in layer._units:
		for c in layer._units[uid2].get_children():
			if c is Label and c.name.begins_with("FloatText"):
				c.queue_free()
	layer._apply_battle_bg("临渊镇", "")
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img7 := get_viewport().get_texture().get_image()
	if img7 != null and not img7.is_empty():
		print("[capture] saved=", OUT_BG, " err=", img7.save_png(OUT_BG), " bg=", layer._bg_path)
	get_tree().quit(0 if err4 == OK else 1)

func _capture(path: String, layer: Control) -> bool:
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[capture] viewport image is empty")
		get_tree().quit(1)
		return false
	var err := img.save_png(path)
	print("[capture] saved=", path, " size=", img.get_size(), " err=", err,
		" battle.size=", layer.size)
	return err == OK
