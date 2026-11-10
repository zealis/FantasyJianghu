extends Node
## 设置面板实拍：真实 GUI 窗口渲染，不依赖服务端与桌面输入注入。
## 运行：Godot_v4.7.2-stable_win64.exe --path client res://tests/settings_capture.tscn
## 产物：client/tests/settings_panel.png
const OUT := "D:/project/Fantasy-Jianghu/client/tests/settings_panel.png"

func _ready() -> void:
	# 注入一组「非关口」真实数据（与玩家反馈的 LV4 状态一致）：
	# 用于确认突破行不再误导成「要手动升级」。
	QuestStore.level = 4
	QuestStore.exp = 437
	QuestStore.exp_next = 800
	QuestStore.unspent_points = 0
	QuestStore.gate_ready = false
	QuestStore.next_gate = 29
	await get_tree().process_frame
	var layer := CanvasLayer.new()
	layer.layer = 5
	add_child(layer)
	var ui: Control = preload("res://scripts/ui/settings_panel.gd").new()
	layer.add_child(ui)
	ui.open()
	for i in range(6):
		await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null:
		printerr("[settings-capture] 取帧失败")
		get_tree().quit(1)
		return
	var err := img.save_png(OUT)
	if err != OK:
		printerr("[settings-capture] 保存失败 err=", err)
		get_tree().quit(1)
		return
	print("[settings-capture] saved ", OUT, " ", img.get_width(), "x", img.get_height())
	get_tree().quit(0)