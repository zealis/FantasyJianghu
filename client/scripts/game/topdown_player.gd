extends CharacterBody2D
## 俯视 2D 试点 · 玩家四向移动（UI 规范 V1.38 / 前端规范 §4）。
## 原作式行走口径：人物恒为侧面像；左右移动播行走帧并按方向翻转，
## 前后（南北）移动不设专门帧、以前进方向侧的侧面像平移（沿用 16 位主角行走帧，不补帧）。
## 本脚本只做表现与碰撞，不接协议；服务端位置校验对接属下一阶段。

const SPEED := 260.0
const WALK_FPS := 8.0
const WALK_SKIN_DIR := "res://assets/sprites/walk/"
const LEGACY_DIR := "res://assets/sprites/player/"

var _sprite: AnimatedSprite2D
var _anim_name := "idle"
var _facing_right := true
var _skin_key := ""
var _target: Vector2 = Vector2.INF
## 测试/实拍用的输入注入（空值时读真实键盘），避免依赖桌面输入注入。
var input_override := Vector2.ZERO

func _ready() -> void:
	motion_mode = CharacterBody2D.MOTION_MODE_FLOATING
	# 俯视碰撞用脚底小圆（约半个身位），与静态建筑/河流多边形相抵。
	var shape := CollisionShape2D.new()
	var circ := CircleShape2D.new()
	circ.radius = 18.0
	shape.shape = circ
	shape.position = Vector2(0, -8)
	add_child(shape)
	_sprite = AnimatedSprite2D.new()
	_sprite.sprite_frames = _build_frames()
	_sprite.scale = Vector2(0.5, 0.5)
	# 脚底对齐节点原点（贴图上移半个身位），Y-Sort 按节点 y 排序才正确。
	_sprite.position = Vector2(0, -70)
	add_child(_sprite)
	_sprite.play("idle")
	var cam := Camera2D.new()
	cam.name = "Camera"
	cam.position_smoothing_enabled = true
	add_child(cam)

func _skin() -> String:
	var f := SessionStore.faction if SessionStore.faction != "" else "惊雷殿男"
	var g := SessionStore.gender if SessionStore.gender != "" else ""
	var key := f + g
	if not ResourceLoader.exists(WALK_SKIN_DIR + key + "/walk_0.png"):
		key = "惊雷殿男"
	return key

func _frame_path(anim: String, idx: int) -> String:
	if anim == "walk":
		var skin := "%s%s/walk_%d.png" % [WALK_SKIN_DIR, _skin_key, idx]
		if ResourceLoader.exists(skin):
			return skin
	return "%s%s_%d.png" % [LEGACY_DIR, anim, idx]

func _build_frames() -> SpriteFrames:
	_skin_key = _skin()
	var frames := SpriteFrames.new()
	frames.remove_animation("default")
	for anim in ["idle", "walk"]:
		frames.add_animation(anim)
		frames.set_animation_speed(anim, WALK_FPS)
		frames.set_animation_loop(anim, true)
		for i in range(4):
			var path := _frame_path(anim, i)
			var tex: Texture2D = load(path)
			if tex != null:
				frames.add_frame(anim, tex)
	return frames

## 只在状态切换时 play()，否则每帧重置 frame 会「只有一个动作」。
func _play(anim: String) -> void:
	if anim == _anim_name and _sprite.is_playing():
		return
	_anim_name = anim
	_sprite.play(anim)

func _dir_from_keys() -> Vector2:
	var v := Vector2.ZERO
	if Input.is_physical_key_pressed(KEY_A) or Input.is_physical_key_pressed(KEY_LEFT):
		v.x -= 1.0
	if Input.is_physical_key_pressed(KEY_D) or Input.is_physical_key_pressed(KEY_RIGHT):
		v.x += 1.0
	if Input.is_physical_key_pressed(KEY_W) or Input.is_physical_key_pressed(KEY_UP):
		v.y -= 1.0
	if Input.is_physical_key_pressed(KEY_S) or Input.is_physical_key_pressed(KEY_DOWN):
		v.y += 1.0
	return v.normalized()

func _unhandled_input(event: InputEvent) -> void:
	# 点按目标点直线行走（试点版：遇碰撞体自然停下，避碰撞寻路留下一阶段）。
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_target = get_global_mouse_position()
	if event is InputEventKey and event.pressed and not event.echo and event.keycode == KEY_ESCAPE:
		_target = Vector2.INF

func _physics_process(_delta: float) -> void:
	var dir := input_override if input_override != Vector2.ZERO else _dir_from_keys()
	if dir == Vector2.ZERO and not is_inf(_target.x):
		var diff := _target - global_position
		if diff.length() < 6.0:
			_target = Vector2.INF
		else:
			dir = diff.normalized()
	if dir == Vector2.ZERO:
		velocity = Vector2.ZERO
		move_and_slide()
		_play("idle")
		return
	velocity = dir * SPEED
	move_and_slide()
	# 左右移动播行走帧；前后移动只平移侧面像（不补上下方向帧）。
	if absf(dir.x) > absf(dir.y):
		_play("walk")
		_facing_right = dir.x > 0.0
		_sprite.flip_h = not _facing_right
	else:
		_play("idle")
