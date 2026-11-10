extends Node2D
## 渲染层 · 玩家（前端规范 §4 移动）。左右行走 + 点按沿路径段 + 镜头横向跟随。
## 本地预测移动仅用于表现；服务端权威坐标（3/2 回包）偏差 >16px 时纠偏对齐。
## 移动以 10Hz 上报服务端校验（防瞬移由服务端钳制，开发技术规范 §4 world）。

const SPEED := 320.0
const REPORT_INTERVAL := 0.1
const CORRECT_TOLERANCE := 16.0
const WALK_FPS := 8.0
## 角色专属行走帧：assets/sprites/walk/<门派><性别>/walk_0..3.png。
## 首测只放「惊雷殿男_行走修正版」；未提供的角色回落通用帧，不影响其他人物。
const WALK_SKIN_DIR := "res://assets/sprites/walk/"
const LEGACY_DIR := "res://assets/sprites/player/"

var _sprite: AnimatedSprite2D
var _target_x: float = NAN
var _camera: Camera2D
var _report_accum: float = 0.0
var _last_reported_x: float = NAN
var _anim_name := "idle"
var _walk_key := ""

func _ready() -> void:
	add_to_group("player")
	_sprite = AnimatedSprite2D.new()
	_sprite.sprite_frames = _build_frames()
	_sprite.scale = Vector2(0.5, 0.5)
	_sprite.position = Vector2(0, -75)
	add_child(_sprite)
	_sprite.play(_anim_name)
	_camera = get_node_or_null("../Camera") as Camera2D
	position.y = SceneLogic.ground_y()
	WorldStore.snapshot_applied.connect(_on_snapshot_applied)
	Network.packet_received.connect(_on_packet)
	SessionStore.session_changed.connect(_on_session_changed)

func _on_snapshot_applied() -> void:
	# 服务端权威出生点。
	position.x = WorldStore.self_x
	position.y = WorldStore.ground_y
	_last_reported_x = NAN

## 帧路径解析：行走优先取角色专属修正版（4 个不同姿势），其余回落通用帧。
func _frame_path(anim: String, idx: int) -> String:
	if anim == "walk":
		var key := SessionStore.faction + SessionStore.gender
		var skin := "%s%s/walk_%d.png" % [WALK_SKIN_DIR, key, idx]
		if ResourceLoader.exists(skin):
			return skin
	return "%s%s_%d.png" % [LEGACY_DIR, anim, idx]

func _walk_skin_key() -> String:
	return SessionStore.faction + SessionStore.gender

func _build_frames() -> SpriteFrames:
	var frames := SpriteFrames.new()
	frames.remove_animation("default")
	for anim in ["idle", "walk"]:
		frames.add_animation(anim)
		frames.set_animation_speed(anim, WALK_FPS)
		frames.set_animation_loop(anim, true)
		for i in range(4):
			var path := _frame_path(anim, i)
			var tex: Texture2D = load(path)
			if tex == null:
				push_warning("[player] 缺少帧贴图: %s" % path)
			frames.add_frame(anim, tex)
	_walk_key = _walk_skin_key()
	return frames

## 登录/换角后门派或性别变化 → 重建行走帧，保持贴图与角色一致。
func _on_session_changed() -> void:
	if _walk_key != _walk_skin_key():
		var anim := _anim_name
		_anim_name = ""
		_sprite.sprite_frames = _build_frames()
		_play_anim(anim)

## 关键修复：只在状态切换时调用 play()。
## 每帧重复 play(同一动画) 会把 frame 重置回 0，导致「行走只有一个动作」。
func _play_anim(anim: String) -> void:
	if anim == _anim_name and _sprite.is_playing():
		return
	_anim_name = anim
	_sprite.play(anim)

## 任务「前往」：自动走到指定 x（复用点按沿路径段移动，手动按键即取消）。
func walk_to(x: float) -> void:
	_target_x = clampf(x, WorldStore.walk_bounds.x, WorldStore.walk_bounds.y)

func _unhandled_input(event: InputEvent) -> void:
	if event is InputEventMouseButton and event.pressed and event.button_index == MOUSE_BUTTON_LEFT:
		_target_x = get_global_mouse_position().x
	if event is InputEventKey and event.pressed and not event.echo and event.keycode == KEY_E:
		if not WorldStore.near_npc_id.is_empty():
			WorldStore.request_dialogue(WorldStore.near_npc_id)

func _physics_process(delta: float) -> void:
	var dir := 0.0
	if Input.is_physical_key_pressed(KEY_A) or Input.is_physical_key_pressed(KEY_LEFT):
		dir -= 1.0
	if Input.is_physical_key_pressed(KEY_D) or Input.is_physical_key_pressed(KEY_RIGHT):
		dir += 1.0
	if dir != 0.0:
		_target_x = NAN
	elif not is_nan(_target_x):
		var diff := _target_x - position.x
		if absf(diff) < 4.0:
			_target_x = NAN
		else:
			dir = signf(diff)
	var bounds := WorldStore.walk_bounds
	position.x = clampf(position.x + dir * SPEED * delta, bounds.x, bounds.y)
	if dir != 0.0:
		_sprite.flip_h = dir < 0.0
		_play_anim("walk")
	else:
		_play_anim("idle")
	if _camera:
		_camera.position.x = lerpf(_camera.position.x, position.x, 0.12)
	_report_move(delta)

## 10Hz 上报移动（仅在位置变化且已连服时）。
func _report_move(delta: float) -> void:
	if Network.offline:
		return
	_report_accum += delta
	if _report_accum < REPORT_INTERVAL:
		return
	_report_accum = 0.0
	if is_nan(_last_reported_x) or absf(position.x - _last_reported_x) > 0.5:
		_last_reported_x = position.x
		Network.send(3, 2, {"x": position.x, "y": position.y})

## 服务端权威纠偏：偏差超容忍即对齐（不瞬移过校验）。
func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	if main_code != 3 or sub_code != 2:
		return
	var server_x := float(payload.get("x", position.x))
	# 回包等于自己上一次上报 = 确认而非纠偏。低帧率下物理按批步进，回包到达时
	# 预测位置已领先上报点 16px 以上（30fps 约 3~4 tick），误判会 snap 回并取消点按/自动走目标。
	if not is_nan(_last_reported_x) and absf(server_x - _last_reported_x) < 1.0:
		return
	if absf(server_x - position.x) > CORRECT_TOLERANCE:
		position.x = server_x
		_target_x = NAN
		_last_reported_x = server_x
