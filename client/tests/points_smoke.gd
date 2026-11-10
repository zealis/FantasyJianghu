extends Node2D
## 加点面板冒烟（M2c 切片四 / B3）：三栏渲染 + 步进/预加/推荐/预览/重置，
## 并导出实拍图 outputs/points_panel.png。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/points_smoke.tscn

const OUT := "D:/project/Fantasy-Jianghu/outputs/points_panel.png"
const PanelScript := preload("res://scripts/ui/point_panel.gd")

var _panel: Control
var _fail := 0

func _ready() -> void:
	QuestStore.level = 28
	QuestStore.exp = 320
	QuestStore.exp_next = 520
	QuestStore.unspent_points = 8
	QuestStore.points = {"str": 60, "spi": 30, "agi": 20, "con": 16}
	QuestStore.stats = {
		"hp": 3240, "mp": 1180, "patk": 846, "matk": 156,
		"pdef": 402, "mdef": 318, "speed": 210, "pdodge": 0, "pcrit": 1250,
	}
	SessionStore.player_name = "青衫客"
	SessionStore.faction = "断岳剑宗"
	SessionStore.gender = "男"
	SessionStore.title = "星核守护者"

	var bg := ColorRect.new()
	bg.color = Color("6f7f6a")
	bg.size = Vector2(1280, 720)
	add_child(bg)

	_panel = Control.new()
	_panel.set_script(PanelScript)
	add_child(_panel)
	await get_tree().process_frame
	await get_tree().process_frame

	_check_initial()
	_check_stepper()
	_check_preset()
	_check_preview()
	_check_reset()
	await _capture()
	print("[points_smoke] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _check(cond: bool, label: String) -> void:
	print("[points_smoke] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _check_initial() -> void:
	_check(_panel._value_labels.size() == 4, "四属性行齐全")
	_check(_panel._avail_label.text == "8", "可分配点数显示 8")
	_check((_panel._value_labels["str"] as Label).text == "60", "力量当前值 60")
	_check(_panel._confirm.disabled, "未预加时确认按钮置灰")
	_check(_panel._preview_now["patk"].text == "846", "属性预览取服务端 patk=846")
	_check(_panel._portrait.texture != null, "左栏立绘已加载")
	_check(_panel._portrait.texture.resource_path.contains("heroes/single"),
		"立绘使用 heroes/single 正面图")

func _check_stepper() -> void:
	_panel._add("str", 1)
	_panel._add("str", 1)
	_panel._add("str", 1)
	_panel._add("str", 1)
	_check((_panel._value_labels["str"] as Label).text == "64", "力量 +4 后显示 64")
	_check(_panel._avail_label.text == "4", "剩余可分配 4")
	# 点到上限后不再增加。
	for i in range(8):
		_panel._add("agi", 1)
	_check(int(_panel._pending["agi"]) == 4, "预加总量受剩余点数限制")
	_check(int(_panel._pending_sum()) == 8, "预加合计 8")
	_check(not _panel._confirm.disabled, "有预加时确认按钮可用")
	# 减号回退。
	_panel._add("agi", -1)
	_check(int(_panel._pending["agi"]) == 3, "减号回退一点")

func _check_preset() -> void:
	_panel._reset_pending()
	_panel._pick_preset(0)
	_panel._fill_all()
	_check(int(_panel._pending_sum()) == 8, "一键加点用满 8 点")
	_check(int(_panel._pending["str"]) == 6 and int(_panel._pending["agi"]) == 2,
		"3力1敏循环填充（力6 敏2）")

func _check_preview() -> void:
	# 力6 敏2：物攻 +12、速度 +2。
	_check(_panel._preview_after["patk"].text == "→ 858", "物攻预览 846→858")
	_check(_panel._preview_delta["patk"].text == "(+12)", "物攻增量 +12")
	_check(_panel._preview_after["speed"].text == "→ 212", "速度预览 210→212")
	_check(_panel._preview_delta["speed"].text == "(+2)", "速度增量 +2")

func _check_reset() -> void:
	_panel._reset_pending()
	_check(int(_panel._pending_sum()) == 0, "重置清空预加")
	_check(_panel._avail_label.text == "8", "重置后可分配回到 8")
	_check(_panel._confirm.disabled, "重置后确认置灰")

func _capture() -> void:
	# 实拍用一组预加：力4 + 敏2，展示橙字预加与绿色增量。
	_panel._add("str", 4)
	_panel._add("agi", 2)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		_check(false, "实拍截图为空")
		return
	_check(img.save_png(OUT) == OK, "加点面板实拍已导出")
