extends Node
## 2.5D 斜俯视定稿图试点实拍：出生点 / 沿主街走到石拱桥（镜头 zoom 1.6 双向跟随）。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/qinghecun_v7_capture.tscn

const OUT_START := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v7_start.png"
const OUT_BRIDGE := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v7_bridge.png"
const OUT_ROOF := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v7_roof_blocked.png"
const OUT_WATER := "D:/project/Fantasy-Jianghu/client/tests/qinghecun_v7_water_blocked.png"
const Scene := preload("res://scenes/world/qinghecun_v7.tscn")

func _ready() -> void:
	var w := Scene.instantiate()
	add_child(w)
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_START)
	var p := w.get_node_or_null("YSortLayer/Player") as CharacterBody2D
	if p == null:
		push_error("[v7] 玩家节点缺失")
		get_tree().quit(1)
		return
	# 手画障碍块后，可走石板路是斜向的：(520,700) → (867,469) 桥面。
	# 旧的「先 UP 再 RIGHT」直角路径会撞进中央大宅 / 桥西绿地，已废止。
	p.input_override = Vector2(0.8321, -0.5547)
	await _physics_frames(125)
	p.input_override = Vector2.ZERO
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_BRIDGE)
	print("[v7] 玩家终点=", p.position, " 镜头中心=", (p.get_node("Camera") as Camera2D).get_screen_center_position())

	# 从中央大宅门洞热区朝屋顶 (660,160) 点按，应被手画障碍挡在台阶前。
	p.position = Vector2(538, 413)
	p.set("_target", Vector2(660, 160))
	await _physics_frames(80)
	p.set("_target", Vector2.INF)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_ROOF)
	print("[v7] 屋顶阻挡后玩家=", p.position, "（未站上瓦片）")

	# 从东岸朝下游河面 (1048,722) 点按，应被手画障碍挡在岸上。
	p.position = Vector2(1300, 900)
	p.set("_target", Vector2(1048, 722))
	await _physics_frames(80)
	p.set("_target", Vector2.INF)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_WATER)
	print("[v7] 水面阻挡后玩家=", p.position, "（未进河面）")
	get_tree().quit(0)

func _physics_frames(n: int) -> void:
	for i in range(n):
		await get_tree().physics_frame

func _save(path: String) -> void:
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[v7] 视口图像为空")
		return
	var err := img.save_png(path)
	print("[v7] saved=", path, " size=", img.get_size(), " err=", err)
