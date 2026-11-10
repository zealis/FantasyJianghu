extends Node
## 清河村扩张版13 实拍：出生点 / 广场东北 / 广场西南明雷 / 障碍边界阻挡。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/qinghecun_v8_capture.tscn

const OUT_DIR := "D:/project/Fantasy-Jianghu/client/tests/"
const Scene := preload("res://scenes/world/qinghecun_v8.tscn")

func _ready() -> void:
	var w := Scene.instantiate()
	add_child(w)
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_DIR + "qinghecun_v8_start.png")
	var p := w.get_node_or_null("YSortLayer/Player") as CharacterBody2D
	if p == null:
		push_error("[v8] 玩家节点缺失")
		get_tree().quit(1)
		return

	# 广场东北（教头·铁山旁，稍微错开避免与 NPC 重叠）——镜头双向跟随。
	p.position = Vector2(1150, 720)
	await _settle()
	_save(OUT_DIR + "qinghecun_v8_plaza_east.png")

	# 广场西南（青牙狼明雷旁）。
	p.position = Vector2(600, 850)
	await _settle()
	_save(OUT_DIR + "qinghecun_v8_plaza_west.png")

	# 朝西南河面点按，应停在障碍边界外。
	p.position = Vector2(950, 780)
	p.set("_target", Vector2(150, 1000))
	await _physics_frames(120)
	p.set("_target", Vector2.INF)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	_save(OUT_DIR + "qinghecun_v8_blocked.png")
	print("[v8] 障碍阻挡后玩家=", p.position, "（未进河面）")
	get_tree().quit(0)

func _settle() -> void:
	# 位置突变后给镜头平滑跟随一点时间（用帧数而非定时器，实机/无头都可靠）。
	for i in range(60):
		await get_tree().process_frame
	await RenderingServer.frame_post_draw

func _physics_frames(n: int) -> void:
	for i in range(n):
		await get_tree().physics_frame

func _save(path: String) -> void:
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[v8] 视口图像为空")
		return
	var err := img.save_png(path)
	print("[v8] saved=", path, " size=", img.get_size(), " err=", err)
