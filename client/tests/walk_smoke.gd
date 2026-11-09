extends Node2D
## 行走动画冒烟测试（真实 Godot 渲染，不依赖编辑器/MCP 输入注入）。
## 覆盖：8 门派×男/女 共 16 套修正版行走帧（每套 4 帧可加载、图像互异、尺寸 384×280）；
## 缺素材角色回落通用帧；惊雷殿男动画逐帧推进 + 分帧实拍导出。
## 运行：
##   Godot_v4.7.2-stable_win64_console.exe --path client res://tests/walk_smoke.tscn

const OUT_DIR := "D:/project/Fantasy-Jianghu/outputs/"
const PlayerScript := preload("res://scripts/game/player.gd")
const SKIN := "res://assets/sprites/walk/%s/walk_%d.png"
const LEGACY := "res://assets/sprites/player/walk_%d.png"
const FACTIONS := ["惊雷殿", "断岳剑宗", "玄水宫", "璇玑星宫", "破军枪府", "织梦谷", "缚灵符观", "金刚禅院"]
const GENDERS := ["男", "女"]

var _p: Node2D
var _sprite: AnimatedSprite2D
var _fail := 0

func _ready() -> void:
	_build_stage()
	SessionStore.faction = "惊雷殿"
	SessionStore.gender = "男"
	_p = Node2D.new()
	_p.set_script(PlayerScript)
	add_child(_p)
	await get_tree().process_frame
	_p.position = Vector2(640.0, 430.0)
	# 放大仅为了分帧实拍看得清，不改变游戏内 scale。
	_sprite = _p._sprite
	_sprite.scale = Vector2(1.0, 1.0)
	_sprite.position = Vector2(0.0, -40.0)

	await _check_all_skins()
	await _check_paths()
	await _check_textures()
	await _check_animation()
	await _capture_frames()

	var report := {
		"skins_expected": FACTIONS.size() * GENDERS.size(), "fail": _fail,
		"note": "16 套行走修正版全量导入；未提供素材的角色回落通用帧",
	}
	var f := FileAccess.open(OUT_DIR + "walk_smoke_report.json", FileAccess.WRITE)
	if f:
		f.store_string(JSON.stringify(report, "\t"))
		f.close()
	print("[walk_smoke] DONE fail=", _fail)
	get_tree().quit(0 if _fail == 0 else 1)

func _build_stage() -> void:
	var bg := ColorRect.new()
	bg.color = Color("efe7d6")
	bg.size = Vector2(1280.0, 720.0)
	bg.position = Vector2.ZERO
	add_child(bg)
	var ground := ColorRect.new()
	ground.color = Color("c9b895")
	ground.size = Vector2(1280.0, 6.0)
	ground.position = Vector2(0.0, 500.0)
	add_child(ground)

func _check(cond: bool, label: String) -> void:
	print("[walk_smoke] ", "PASS " if cond else "FAIL ", label)
	if not cond:
		_fail += 1

## 贴图降采样指纹，用于证明 4 帧是不同姿势而非同一张图微调。
func _digest(tex: Texture2D) -> int:
	if tex == null:
		return -1
	var data := tex.get_image().get_data()
	var h := 2166136261
	for i in range(0, data.size(), 97):
		h = ((h ^ int(data[i])) * 16777619) & 0x7fffffff
	return h

## 16 套全量：路径命中 + 4 帧可加载 + 尺寸一致 + 图像互异。
func _check_all_skins() -> void:
	var passed := 0
	for fac in FACTIONS:
		for g in GENDERS:
			var key: String = fac + g
			SessionStore.faction = fac
			SessionStore.gender = g
			var ok := true
			var digests := {}
			for i in range(4):
				var path: String = _p._frame_path("walk", i)
				if path != (SKIN % [key, i]):
					ok = false
				var tex: Texture2D = load(path)
				if tex == null:
					ok = false
				else:
					if tex.get_width() != 384 or tex.get_height() != 280:
						ok = false
					digests[_digest(tex)] = true
			if digests.size() != 4:
				ok = false
			if ok:
				passed += 1
			else:
				_fail += 1
			print("[walk_smoke] ", "PASS " if ok else "FAIL ", key, " 四帧")
	_check(passed == FACTIONS.size() * GENDERS.size(),
		"16 套行走修正版全部命中且四帧互异（%d/16）" % passed)
	SessionStore.faction = "惊雷殿"
	SessionStore.gender = "男"
	_p._on_session_changed()

func _check_paths() -> void:
	for i in range(4):
		_check(_p._frame_path("walk", i) == (SKIN % ["惊雷殿男", i]),
			"惊雷殿男第 %d 帧命中修正版" % i)
	# 未提供素材的角色（未来新增门派/异常数据）必须回落通用帧，不能空贴图。
	SessionStore.faction = "未开放门派"
	_check(_p._frame_path("walk", 0) == (LEGACY % 0), "缺素材角色回落通用行走帧")
	SessionStore.faction = "惊雷殿"

func _check_textures() -> void:
	var digests := []
	for i in range(4):
		var tex: Texture2D = _sprite.sprite_frames.get_frame_texture("walk", i)
		_check(tex != null, "第 %d 帧贴图加载成功" % i)
		digests.append(_digest(tex))
	var uniq := {}
	for d in digests:
		uniq[d] = true
	_check(uniq.size() == 4, "当前帧集四帧图像互不相同")
	_check(_sprite.sprite_frames.get_frame_count("walk") == 4, "walk 动画共 4 帧")
	_check(_sprite.sprite_frames.get_animation_loop("walk"), "walk 动画循环开启")

func _check_animation() -> void:
	# 模拟持续向右移动，确认 player 状态机切到 walk。
	_p._target_x = 100000.0
	await get_tree().process_frame
	await get_tree().process_frame
	_check(_p._anim_name == "walk", "移动时状态机进入 walk")
	_check(_sprite.animation == "walk", "AnimatedSprite2D 正在播放 walk")
	# 冻结物理输入，单独观察动画帧推进。
	_p.set_physics_process(false)
	_sprite.play("walk")
	var seq := []
	for i in range(10):
		await get_tree().create_timer(0.14).timeout
		seq.append(int(_sprite.frame))
	var seen := {}
	for f in seq:
		seen[f] = true
	_check(seen.size() >= 3, "单次 play 后帧自然推进 %s" % str(seq))
	# 诊断留档：每帧重复 play 在当前 Godot 版本不会锁帧，但仍坚持只在切换时 play。
	_sprite.stop()
	_sprite.set_frame(0)
	var seq_repeat := []
	for i in range(6):
		_sprite.play("walk")
		await get_tree().create_timer(0.14).timeout
		seq_repeat.append(int(_sprite.frame))
	var seen_repeat := {}
	for f in seq_repeat:
		seen_repeat[f] = true
	print("[walk_smoke] 重复 play 帧序列=", seq_repeat, " 不同帧数=", seen_repeat.size())

func _capture_frames() -> void:
	_sprite.stop()
	_sprite.flip_h = false
	for i in range(4):
		_sprite.set_frame(i)
		await get_tree().process_frame
		await RenderingServer.frame_post_draw
		var img := get_viewport().get_texture().get_image()
		if img == null or img.is_empty():
			_check(false, "第 %d 帧截图为空" % i)
			continue
		var err := img.save_png(OUT_DIR + "walk_frame_%d.png" % i)
		_check(err == OK, "第 %d 帧实拍已导出" % i)
	_sprite.flip_h = true
	_sprite.set_frame(2)
	await get_tree().process_frame
	await RenderingServer.frame_post_draw
	var flip := get_viewport().get_texture().get_image()
	if flip != null and not flip.is_empty():
		_check(flip.save_png(OUT_DIR + "walk_frame_flip.png") == OK, "向左翻转帧已导出")
