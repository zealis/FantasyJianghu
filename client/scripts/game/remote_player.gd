extends Node2D
## 渲染层 · 远端玩家表现（V1.27 同屏展示范围口径 + 行走精灵化）：
## 按「门派+性别」取 assets/sprites/walk/<key>/walk_0..3.png 四帧迈步循环；
## 静止用 walk_1（并腿站立帧），移动播放 walk，朝向按位移方向 flip_h。
## 名牌（【称号】+名字通则）+ 坐骑/宠物挂点；视口外裁剪只留名字。

## 展示降级档：3=全开；2=关闭他人宠物；1=再关闭他人坐骑（V1.27 顺序）。
## M1 无坐骑宠物系统（挂点恒空），接入 GL 开关于 M3。
static var degradation_level := 3

const WALK_DIR := "res://assets/sprites/walk/%s/walk_%d.png"
const WALK_FPS := 8.0
const DEFAULT_SKIN := "惊雷殿男"
## 与服务端门派表、assets/sprites/walk 目录一一对应。
const SKIN_KEYS := [
	"惊雷殿男", "惊雷殿女", "断岳剑宗男", "断岳剑宗女",
	"玄水宫男", "玄水宫女", "璇玑星宫男", "璇玑星宫女",
	"破军枪府男", "破军枪府女", "织梦谷男", "织梦谷女",
	"缚灵符观男", "缚灵符观女", "金刚禅院男", "金刚禅院女",
]
## 帧集按皮肤缓存，同屏多人共用（只存纹理引用，不复制像素）。
static var _frame_cache: Dictionary = {}

var char_id: int = 0
var target_x: float = 0.0
var target_y: float = 0.0
var skin_key: String = ""
## 横版默认 0.5/-75/-158；俯视主世界由 world_topdown_v7 注入 0.28/-32/-96。
var sprite_scale: float = 0.5
var sprite_offset_y: float = -75.0
var plate_offset_y: float = -158.0

var _body: Node2D
var _sprite: AnimatedSprite2D
var _plate: Label
var _mount_slot: Node2D
var _pet_slot: Node2D
var _has_mount := false
var _has_pet := false
var _anim := ""

func setup(id: int, display_name: String, x: float, y: float, info: Dictionary = {}) -> void:
	char_id = id
	target_x = x
	target_y = y
	position = Vector2(x, y)
	skin_key = _resolve_skin(info)

	# 身形（行走精灵，缩放到与本人一致）。
	_body = Node2D.new()
	add_child(_body)
	_sprite = AnimatedSprite2D.new()
	_sprite.sprite_frames = _skin_frames(skin_key)
	_sprite.scale = Vector2(sprite_scale, sprite_scale)
	_sprite.position = Vector2(0, sprite_offset_y)
	_body.add_child(_sprite)
	_play("idle")

	# 坐骑/宠物挂点（站位口径：坐骑在身形下方左右、宠物伴随侧后）。
	_pet_slot = Node2D.new()
	_pet_slot.position = Vector2(-34, 0)
	add_child(_pet_slot)
	_mount_slot = Node2D.new()
	_mount_slot.position = Vector2(34, 0)
	add_child(_mount_slot)

	# 名牌（【称号】+名字通则；M1 无称号——称号位预留独立着色元素）。
	# 精灵头顶约在 -145，名牌抬到 -158 避免压住发簪。
	_plate = Label.new()
	_plate.text = display_name
	_plate.position = Vector2(-40, plate_offset_y)
	_plate.custom_minimum_size = Vector2(80, 20)
	_plate.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_plate.add_theme_font_size_override("font_size", 14)
	_plate.add_theme_color_override("font_color", COLOR_NAME)
	# 名字会压在草地/屋顶等深色背景上，加宣纸色描边保证可读。
	_plate.add_theme_color_override("font_outline_color", Color("f6efdf"))
	_plate.add_theme_constant_override("outline_size", 3)
	add_child(_plate)

	_apply_degradation()

const COLOR_NAME := Color("3b2e1e")

## 皮肤解析：服务端下发 faction/gender 时用真实值；暂未下发（3/1/3/6 仅 id/name/x/y）
## 时按 char_id 稳定分配，保证同一玩家在别人屏幕上形象固定、且不再是方块。
func _resolve_skin(info: Dictionary) -> String:
	var fac := str(info.get("faction", ""))
	var gen := str(info.get("gender", ""))
	if not fac.is_empty() and not gen.is_empty():
		var key: String = fac + gen
		if ResourceLoader.exists(WALK_DIR % [key, 0]):
			return key
	var idx: int = absi(char_id) % SKIN_KEYS.size()
	return SKIN_KEYS[idx]

func _skin_frames(key: String) -> SpriteFrames:
	if _frame_cache.has(key):
		return _frame_cache[key]
	var frames := SpriteFrames.new()
	frames.remove_animation("default")
	frames.add_animation("idle")
	frames.set_animation_speed("idle", 1.0)
	frames.set_animation_loop("idle", false)
	var stand: Texture2D = load(WALK_DIR % [key, 1])
	if stand != null:
		frames.add_frame("idle", stand)
	frames.add_animation("walk")
	frames.set_animation_speed("walk", WALK_FPS)
	frames.set_animation_loop("walk", true)
	for i in range(4):
		var tex: Texture2D = load(WALK_DIR % [key, i])
		if tex == null:
			push_warning("[remote] 缺少行走帧 %s #%d，回落默认皮肤" % [key, i])
			return _skin_frames(DEFAULT_SKIN)
		frames.add_frame("walk", tex)
	_frame_cache[key] = frames
	return frames

## 只在状态切换时 play，避免每帧重复调用重置动画。
func _play(anim: String) -> void:
	if _sprite == null:
		return
	if anim == _anim and _sprite.is_playing():
		return
	_anim = anim
	_sprite.play(anim)

## 名牌刷新（【称号】+名字通则；title 空则仅名字）。
func set_nameplate(title: String, char_name: String) -> void:
	if title.is_empty():
		_plate.text = char_name
	else:
		_plate.text = "【%s】%s" % [title, char_name]

## 应用同屏伴生数据（坐骑/宠物，M1 恒 null → 挂点清空）。
func apply_companion(info: Dictionary) -> void:
	_has_mount = info.get("mount") != null
	_has_pet = info.get("pet") != null
	# M3：按信息实例化站立骑乘/伴随宠物素材。当前仅维护可见性口径。
	_apply_degradation()

## 视口外裁剪：精灵隐藏、名字保留（V1.27 性能降级线）。
func set_culled(culled: bool) -> void:
	_body.visible = not culled
	_apply_degradation()

func _apply_degradation() -> void:
	if _pet_slot:
		_pet_slot.visible = _has_pet and degradation_level >= 2
	if _mount_slot:
		_mount_slot.visible = _has_mount and degradation_level >= 1

func _process(delta: float) -> void:
	var dx := target_x - position.x
	var dy := target_y - position.y
	position.x = lerpf(position.x, target_x, minf(1.0, 8.0 * delta))
	position.y = lerpf(position.y, target_y, minf(1.0, 8.0 * delta))
	if _sprite == null:
		return
	if absf(dx) > 1.0 or absf(dy) > 1.0:
		_sprite.flip_h = dx < 0.0
		_play("walk")
	else:
		_play("idle")
