extends Node
## 登录/注册页实拍：真实 GUI 窗口渲染首页长卷背景，不依赖桌面输入注入。
## 运行：Godot --path client res://tests/login_capture.tscn
## 产物：client/tests/login_capture.png

const OUT := "D:/project/Fantasy-Jianghu/client/tests/login_capture.png"

func _ready() -> void:
	var login: Control = preload("res://scenes/ui/login.tscn").instantiate()
	add_child(login)
	login.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	login.size = get_viewport().get_visible_rect().size
	for i in range(4):
		await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[login-capture] viewport image is empty")
		get_tree().quit(1)
		return
	print("[login-capture] saved=", OUT, " err=", img.save_png(OUT), " size=", img.get_size())
	get_tree().quit(0)
