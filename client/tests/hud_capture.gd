extends Node
## HUD 角色卡实拍（验证等级徽定位，不依赖登录/服务端）。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/hud_capture.tscn
const OUT := "D:/project/Fantasy-Jianghu/outputs/hud_capture.png"
const HudScript := preload("res://scripts/ui/hud.gd")

func _ready() -> void:
	SessionStore.player_name = "云溪"
	SessionStore.level = 28
	SessionStore.faction = "惊雷殿"
	SessionStore.gender = "男"
	SessionStore.hp_ratio = 1.0
	SessionStore.mp_ratio = 1.0
	SessionStore.exp_ratio = 0.65

	var root := Control.new()
	root.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(root)
	var bg := ColorRect.new()
	bg.color = Color("6f7f6a")
	bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	root.add_child(bg)

	var hud := Control.new()
	hud.set_script(HudScript)
	root.add_child(hud)
	await get_tree().process_frame
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		push_error("[hud_capture] empty viewport")
		get_tree().quit(1)
		return
	var err := img.save_png(OUT)
	print("[hud_capture] saved=", OUT, " size=", img.get_size(), " err=", err,
		" level=", SessionStore.level)
	get_tree().quit(0 if err == OK else 1)
