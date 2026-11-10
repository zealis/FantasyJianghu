extends Node
## 俯视 2D 整图试点实拍：真实窗口渲染两张（村口出生点 / 沿主街走到石桥），
## 验证双向镜头跟随与桥面通行。产物：client/tests/qinghecun_v3_start.png、qinghecun_v3_bridge.png
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/qinghecun_v3_capture.tscn

const OUT_START := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v3_start.png"
const OUT_BRIDGE := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v3_bridge.png"
const Scene := preload("res://scenes/world/qinghecun_v3.tscn")

func _ready() -> void:
	var w := Scene.instantiate()
	add_child(w)
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_START)

	var p := w.get_node_or_null("YSortLayer/Player") as CharacterBody2D
	if p == null:
		push_error("[qinghecun_v3] 玩家节点缺失")
		get_tree().quit(1)
		return
	# 主街向北到横街，再向东穿过石桥（与冒烟同路径，真实物理帧驱动）。
	p.input_override = Vector2.UP
	await _physics_frames(75)
	p.input_override = Vector2.RIGHT
	await _physics_frames(120)
	p.input_override = Vector2.ZERO
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_BRIDGE)
	print("[qinghecun_v3] 玩家终点=", p.position, " 镜头中心=", (p.get_node("Camera") as Camera2D).get_screen_center_position())
	get_tree().quit(0)

func _physics_frames(n: int) -> void:
	for i in range(n):
		await get_tree().physics_frame

func _save(path: String) -> void:
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[qinghecun_v3] 视口图像为空")
		return
	var err := img.save_png(path)
	print("[qinghecun_v3] saved=", path, " size=", img.get_size(), " err=", err)
