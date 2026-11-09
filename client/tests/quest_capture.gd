extends Node
## 生成任务/背包界面实拍图：真实 GUI 窗口渲染，不依赖桌面输入注入。
## 运行：
##   Godot_v4.7.2-stable_win64_console.exe --path client res://tests/quest_capture.tscn
## 产物：client/tests/quest_panel.png / quest_bag.png

const OUT_PANEL := "D:/project/Fantasy-Jianghu/client/tests/quest_panel.png"
const OUT_BAG := "D:/project/Fantasy-Jianghu/client/tests/quest_bag.png"

func _ready() -> void:
	await get_tree().process_frame
	# 背景用真实清河村场景（相机跟随玩家），截图更接近实机观感。
	add_child(preload("res://scenes/world/qinghecun.tscn").instantiate())
	await get_tree().process_frame

	# 造服务端形状的数据（与 11/1 / 5/1 回包一致）。
	QuestStore.level = 1
	QuestStore.exp = 120
	QuestStore.wallet = {"gold": 500, "xiuwei": 20, "yuanbao": 0}
	QuestStore.daily = {
		"normal": {"used": 1, "limit": 50},
		"sect": {"used": 0, "limit": 20},
		"dungeon": {"used": 0, "limit": 10},
	}
	QuestStore.quests = [
		{
			"id": "main_001", "name": "归村", "type": "main", "chapter": 1,
			"state": "completable", "desc": "闭关归来，村口已非旧日模样。先向何伯问清村中近况。",
			"giver": {"map_id": "清河村", "npc_id": "hebo"},
			"objectives": [{"kind": "talk", "target": "hebo", "cur": 1, "need": 1}],
			"rewards": {"exp": 120, "xiuwei": 20, "gold": 500,
				"items": [{"id": "pill_xiaohuan", "name": "小还丹", "count": 2}]},
		},
		{
			"id": "main_002", "name": "拜见教头", "type": "main", "chapter": 1,
			"state": "available", "desc": "何伯让你去寻教头·铁山，他或许知道村中惨祸的来路。",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "talk", "target": "tieshan", "cur": 0, "need": 1}],
			"rewards": {"exp": 180, "xiuwei": 30, "gold": 600,
				"items": [{"id": "stone_qianghua", "name": "强化石", "count": 3}]},
		},
		{
			"id": "main_003", "name": "试手木桩", "type": "main", "chapter": 1,
			"state": "locked", "desc": "教头要先看你手上功夫。与他切磋木桩人，胜一场再谈别的。",
			"reason": "前置任务未完成",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "kill", "target": "muzhuang", "cur": 0, "need": 1}],
			"rewards": {"exp": 300, "xiuwei": 50, "gold": 1200,
				"items": [{"id": "token_zhenbao", "name": "珍宝令", "count": 1}]},
		},
		{
			"id": "daily_n_001", "name": "村口巡逻", "type": "daily_normal",
			"state": "active", "desc": "每日例行：在清河村走上一圈，看看有无异常。",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "visit", "target": "清河村", "cur": 1, "need": 1}],
			"rewards": {"exp": 60, "xiuwei": 5, "gold": 200, "items": []},
		},
		{
			"id": "daily_d_001", "name": "肃清山贼", "type": "daily_dungeon",
			"state": "available", "desc": "副本日常：山贼盘踞村道，肃清一伙。",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "kill", "target": "shanzei", "cur": 0, "need": 1}],
			"rewards": {"exp": 200, "xiuwei": 40, "gold": 800,
				"items": [{"id": "stone_qianghua", "name": "强化石", "count": 2}]},
		},
	]
	QuestStore.bag = {
		"capacity": 60, "used": 3,
		"items": [
			{"id": "pill_xiaohuan", "name": "小还丹", "category": "丹药", "count": 2, "stack": 99},
			{"id": "stone_qianghua", "name": "强化石", "category": "材料", "count": 8, "stack": 999},
			{"id": "token_zhenbao", "name": "珍宝令", "category": "凭证", "count": 1, "stack": 999},
		],
	}

	var ui_layer := CanvasLayer.new()
	ui_layer.layer = 5
	add_child(ui_layer)
	var ui: Control = preload("res://scripts/ui/quest_ui.gd").new()
	ui_layer.add_child(ui)
	QuestStore.changed.emit()
	await _wait(4)

	# 1) 任务面板 + 追踪条。
	ui._panel.visible = true
	ui._refresh_panel()
	await _wait(6)
	if not await _capture(OUT_PANEL):
		return

	# 2) 背包面板。
	ui._panel.visible = false
	ui._bag_panel.visible = true
	ui._refresh_bag()
	await _wait(6)
	if not await _capture(OUT_BAG):
		return
	print("[quest-capture] DONE")
	get_tree().quit(0)

func _wait(frames: int) -> void:
	for i in range(frames):
		await get_tree().process_frame

func _capture(path: String) -> bool:
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null:
		printerr("[quest-capture] 取帧失败: ", path)
		get_tree().quit(1)
		return false
	var err := img.save_png(path)
	if err != OK:
		printerr("[quest-capture] 保存失败: ", path, " err=", err)
		get_tree().quit(1)
		return false
	print("[quest-capture] saved ", path, " ", img.get_width(), "x", img.get_height())
	return true
