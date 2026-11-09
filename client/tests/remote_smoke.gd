extends Node2D
## 远程玩家精灵化冒烟（真实 Godot 渲染）。
## 校验：AnimatedSprite2D 替换方块、真实门派/性别优先、缺字段按 id 稳定分配、
## 移动播放 walk / 停下回 idle、朝向翻转、视口外隐藏精灵但保留名牌；并导出实拍图。
## 运行：Godot_v4.7.2-stable_win64_console.exe --path client res://tests/remote_smoke.tscn

const OUT := "D:/project/Fantasy-Jianghu/outputs/remote_players.png"
const RemoteScript := preload("res://scripts/game/remote_player.gd")
const CASES := [
	{"id": 1001, "name": "云溪", "faction": "惊雷殿", "gender": "男"},
	{"id": 1002, "name": "青蘅", "faction": "惊雷殿", "gender": "女"},
	{"id": 1003, "name": "岳沉舟", "faction": "断岳剑宗", "gender": "男"},
	{"id": 1004, "name": "苏挽月", "faction": "织梦谷", "gender": "女"},
	{"id": 1005, "name": "玄水行者", "faction": "玄水宫", "gender": "男"},
	{"id": 1006, "name": "无名客", "faction": "", "gender": ""},
]

var _nodes: Array = []
var _fail := 0

func _ready() -> void:
	_build_stage()
	for i in range(CASES.size()):
		var c: Dictionary = CASES[i]
		var n := Node2D.new()
		n.set_script(RemoteScript)
		add_child(n)
		n.setup(int(c.id), str(c.name), 150.0 + i * 190.0, 500.0, c)
		# 测试内手动定步长驱动，避免引擎首帧 delta 过大把 lerp 一步拉满。
		n.set_process(false)
		_nodes.append(n)
	await get_tree().process_frame

	_check_skin()
	await _check_motion()
	_check_cull()
	await _capture()
	print("[remote_smoke] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _build_stage() -> void:
	var bg := ColorRect.new()
	bg.color = Color("efe7d6")
	bg.size = Vector2(1280.0, 720.0)
	add_child(bg)
	var ground := ColorRect.new()
	ground.color = Color("c9b895")
	ground.size = Vector2(1280.0, 6.0)
	ground.position = Vector2(0.0, 500.0)
	add_child(ground)

func _check(cond: bool, label: String) -> void:
	print("[remote_smoke] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

func _check_skin() -> void:
	var n0: Node2D = _nodes[0]
	_check(n0._sprite is AnimatedSprite2D, "远程玩家使用 AnimatedSprite2D（不再是方块）")
	_check(n0._sprite.sprite_frames.get_frame_count("walk") == 4, "行走动画 4 帧")
	_check(n0._sprite.sprite_frames.get_frame_count("idle") == 1, "站立动画 1 帧（walk_1 并腿帧）")
	_check(n0.skin_key == "惊雷殿男", "服务端下发门派/性别时用真实皮肤")
	_check((_nodes[2] as Node2D).skin_key == "断岳剑宗男", "不同门派取不同皮肤")
	# 1006 % 16 = 14 → 金刚禅院男；同一 id 重复解析结果必须一致。
	_check((_nodes[5] as Node2D).skin_key == "金刚禅院男", "缺字段按 char_id 稳定分配")
	var probe := Node2D.new()
	probe.set_script(RemoteScript)
	add_child(probe)
	probe.setup(1006, "探针", 0.0, 0.0, {"faction": "", "gender": ""})
	var first: String = probe.skin_key
	probe.setup(1006, "探针2", 0.0, 0.0, {"faction": "", "gender": ""})
	_check(first == probe.skin_key, "同 id 重复 setup 皮肤稳定")
	probe.queue_free()

func _check_motion() -> void:
	var n0: Node2D = _nodes[0]
	n0.target_x = n0.position.x + 400.0
	for i in range(12):
		n0._process(0.016)
	_check(n0._sprite.animation == "walk", "向右移动播放 walk（x=%d）" % int(n0.position.x))
	_check(n0._sprite.flip_h == false, "向右移动不翻转")
	n0.target_x = n0.position.x - 400.0
	for i in range(12):
		n0._process(0.016)
	_check(n0._sprite.flip_h == true, "向左移动翻转朝向")
	n0.target_x = n0.position.x
	for i in range(3):
		n0._process(0.016)
	_check(n0._sprite.animation == "idle", "停下切回站立帧")
	await get_tree().process_frame

func _check_cull() -> void:
	var n1: Node2D = _nodes[1]
	n1.set_culled(true)
	_check(n1._body.visible == false, "视口外隐藏精灵")
	_check(n1._plate.visible == true, "视口外保留名牌")
	n1.set_culled(false)
	_check(n1._body.visible == true, "回到视口内恢复精灵")

func _capture() -> void:
	# 一半向左、一半向右走，实拍不同门派皮肤与朝向。
	for i in range(_nodes.size()):
		var n: Node2D = _nodes[i]
		n.target_x = n.position.x + (400.0 if i % 2 == 0 else -400.0)
		for k in range(6):
			n._process(0.016)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var img := get_viewport().get_texture().get_image()
	if img == null or img.is_empty():
		_check(false, "实拍截图为空")
		return
	_check(img.save_png(OUT) == OK, "远程玩家实拍已导出")
