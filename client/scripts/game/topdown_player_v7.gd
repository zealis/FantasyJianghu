extends CharacterBody2D
## 2.5D 斜俯视试点 · 玩家四向移动（UI 规范 V1.42 / 前端规范 §4）。
## 比例口径：人物显示高 62 图内 px（≈门洞高 ×0.9），帧实际高 220px → scale 0.28；
## 移速 200 图内 px/s；镜头 zoom 1.6 由 world_topdown_v7 设置。
## 原作式行走口径：人物恒侧面像；左右移动播行走帧并按方向翻转，
## 前后（南北）移动不设专门帧、以前进方向侧的侧面像平移（沿用 16 位行走帧，不补帧）。

const WALK_FPS := 8.0
const WALK_SKIN_DIR := "res://assets/sprites/walk/"
const LEGACY_DIR := "res://assets/sprites/player/"

var speed := 200.0
var _sprite: AnimatedSprite2D
var _anim_name := "idle"
var _facing_right := true
var _skin_key := ""
var _target: Vector2 = Vector2.INF
## 测试/实拍用的输入注入（空值时读真实键盘）。
var input_override := Vector2.ZERO

func _ready() -> void:
	add_to_group("player")
	motion_mode = CharacterBody2D.MOTION_MODE_FLOATING
	# 脚底小圆：人物仅 62px 高，半径 8px 与门洞/墙基线尺度相称。
	var shape := CollisionShape2D.new()
	var circ := CircleShape2D.new()
	circ.radius = 8.0
	shape.shape = circ
	shape.position = Vector2(0, -4)
	add_child(shape)
	_sprite = AnimatedSprite2D.new()
	_sprite.name = "BodySprite"
	_sprite.sprite_frames = _build_frames()
	add_child(_sprite)
	_sprite.play("idle")
	var cam := Camera2D.new()
	cam.name = "Camera"
	cam.position_smoothing_enabled = true
	add_child(cam)
	# 默认配置（world 的 configure() 随后按 JSON 覆盖）。
	_apply_render(0.28, -32.0)

## 由场景逻辑数据驱动比例与速度；换图不得改动脚本。
func configure(render: Dictionary) -> void:
	speed = float(render.get("speed", 200.0))
	var sc := float(render.get("sprite_scale", 0.28))
	var oy := float(render.get("sprite_offset_y", -32))
	_apply_render(sc, oy)

func _apply_render(sc: float, oy: float) -> void:
	_sprite.scale = Vector2(sc, sc)
	_sprite.position = Vector2(0, oy)

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
	# 点按目标点直线行走（试点版：遇墙基线/岸线自然停下，避碰撞寻路留下一阶段）。
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_target = get_global_mouse_position()
	if event is InputEventKey and event.pressed and not event.echo and event.keycode == KEY_ESCAPE:
		_target = Vector2.INF
	if event is InputEventKey and event.pressed and not event.echo and event.keycode == KEY_E:
		# 承接横版 player.gd：靠近 NPC 时 E 触发对话（俯视 NPC 写入 near_npc_id）。
		if WorldStore.near_npc_id != "":
			WorldStore.request_dialogue(WorldStore.near_npc_id)

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
	velocity = dir * speed
	move_and_slide()
	if absf(dir.x) > absf(dir.y):
		_play("walk")
		_facing_right = dir.x > 0.0
		_sprite.flip_h = not _facing_right
	else:
		_play("idle")
