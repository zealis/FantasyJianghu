extends Node
## 品质共鸣行离线实拍（V1.29《套装加成配置表》V1.0）：
## 未激活 → 二档（断岳剑宗，暴击 3.5+3=6.5%）→ 二档（缚灵符观，封印命中 7+3=10%）。
## 不连服务端、不登录：直接注入 EquipStore，验证文案与排版。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/suit_capture.tscn

const OUT_DIR := "D:/project/Fantasy-Jianghu/outputs/"

var _ui

func _ready() -> void:
	await _frames(2)
	var ui_layer := CanvasLayer.new()
	ui_layer.layer = 5
	add_child(ui_layer)
	_ui = preload("res://scripts/ui/quest_ui.gd").new()
	ui_layer.add_child(_ui)
	await _frames(3)

	# ① 未激活：只穿 2 件同品质（服务端仍会返回 tier=0 的明细，不是空）。
	_fill_slots("完美", 2)
	EquipStore.suit = {"tier": 0, "quality": "", "count": 0, "all_pct": 0.0,
		"main_attrs": [], "main_pct": 0.0, "sub_attr": "", "sub_pct": 0.0,
		"tweak_attr": "", "tweak_pct": 0.0}
	_open_bag()
	await _frames(3)
	await _capture("suit_row_off.png")

	# ② 二档 · 断岳剑宗（物理）：全属性 +5%、物攻 +9%、暴击 6.5%。
	_fill_slots("完美", 5)
	EquipStore.suit = {"tier": 2, "quality": "完美", "count": 5, "all_pct": 5.0,
		"main_attrs": ["patk"], "main_pct": 9.0, "sub_attr": "pcrit", "sub_pct": 3.5,
		"tweak_attr": "pcrit", "tweak_pct": 3.0}
	_open_bag()
	await _frames(3)
	await _capture("suit_row_tier2_sword.png")

	# ③ 二档 · 缚灵符观（封印）：速度 +9%、封印命中 10%（副项 7 + 微调 3）。
	EquipStore.suit = {"tier": 2, "quality": "完美", "count": 5, "all_pct": 5.0,
		"main_attrs": ["speed"], "main_pct": 9.0, "sub_attr": "sealhit", "sub_pct": 7.0,
		"tweak_attr": "sealhit", "tweak_pct": 3.0}
	_open_bag()
	await _frames(3)
	await _capture("suit_row_tier2_talisman.png")

	print("[suit_capture] DONE")
	get_tree().quit(0)

## 填 n 件同品质装备（五槽顺序固定，便于对照）。
func _fill_slots(quality: String, n: int) -> void:
	var order := ["weapon", "clothes", "hat", "necklace", "boots"]
	EquipStore.slots = {}
	for i in range(n):
		var slot: String = order[i]
		EquipStore.slots[slot] = {"id": "eq_%d" % i, "name": "完美装备", "quality": quality,
			"stats": {}, "slot": slot}
	EquipStore.attrs = {"hp": 1050, "mp": 504, "patk": 125, "matk": 105, "pdef": 60, "mdef": 72,
		"speed": 114, "hit": 10000, "dodge": 0, "crit": 650, "combo": 0, "resist": 1000,
		"sealhit": 1000, "mcrit": 600}

## 打开背包面板（共鸣行在右栏属性列底部）。
func _open_bag() -> void:
	_ui._bag_panel.visible = true
	_refresh()

func _refresh() -> void:
	_ui._refresh_attrs()
	_ui._refresh_resonance()

func _capture(name: String) -> void:
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		printerr("[suit_capture] 截图失败: ", name)
		return
	img.save_png(OUT_DIR + name)

func _frames(n: int) -> void:
	for i in range(n):
		await get_tree().process_frame
