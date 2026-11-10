extends Node2D
## 明雷怪物（V1.41 定案 / V1.43 移动节奏）：静止为主、不持续游走。
## 每隔 3–4 秒（间隔随机）才横移一次，单次约 1 秒、约 40px，移完即停等待下一次。
## 精灵平移、不播行走帧；树冠/建筑遮挡由 YSortLayer 承担。
## idle_min / idle_max / move_time 可由地图逻辑数据注入（冒烟测试可设小值加速）。

var range_x := 60.0
var move_speed := 40.0
var idle_min := 3.0
var idle_max := 4.0
var move_time := 1.0
var origin_x := 0.0
var dir := 1.0
var _moving := false
var _timer := 0.0
@onready var _spr: Sprite2D = $Sprite2D

func _ready() -> void:
	origin_x = position.x
	_timer = randf_range(idle_min, idle_max)

func _process(delta: float) -> void:
	if not _moving:
		_timer -= delta
		if _timer <= 0.0:
			_moving = true
			_timer = move_time
			# 朝范围内侧走；已在原点时随机选一侧。
			if absf(position.x - origin_x) < 4.0:
				dir = 1.0 if randf() < 0.5 else -1.0
			else:
				dir = 1.0 if position.x < origin_x else -1.0
			_spr.flip_h = dir < 0.0
		return
	_timer -= delta
	position.x = clampf(position.x + dir * move_speed * delta,
			origin_x - range_x, origin_x + range_x)
	if _timer <= 0.0:
		_moving = false
		_timer = randf_range(idle_min, idle_max)
