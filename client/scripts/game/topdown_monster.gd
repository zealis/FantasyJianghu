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
## 击杀后原区重刷间隔（V1.43：击杀后按配置间隔在原区重刷）。
var respawn_seconds := 30.0
## 接触开战的 monster_id（由地图逻辑数据注入；空则不触发，独立试点保持纯展示）。
var battle_monster := ""
var _cooldown := 0.0
var _respawn_left := 0.0
var _main: Node = null
var origin_x := 0.0
var dir := 1.0
var _moving := false
var _timer := 0.0
@onready var _spr: Sprite2D = $Sprite2D

func _ready() -> void:
	origin_x = position.x
	_main = get_tree().root.find_child("Main", true, false)
	_timer = randf_range(idle_min, idle_max)

func _process(delta: float) -> void:
	# 被击杀后隐藏并在原区倒计时重刷；期间不移动、不触发接触开战。
	if _respawn_left > 0.0:
		_respawn_left -= delta
		if _respawn_left <= 0.0:
			_respawn_left = 0.0
			visible = true
			_cooldown = 1.0
		return
	_check_encounter(delta)
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

## V1.41 明雷制：玩家接触即切入战斗（4/1）。仅主流程 Main 存在时生效，
## 独立冒烟/实拍场景没有 Main，不会误触发网络请求。
func _check_encounter(delta: float) -> void:
	_cooldown = maxf(0.0, _cooldown - delta)
	if battle_monster == "" or _cooldown > 0.0:
		return
	if _main == null or not _main.has_method("enter_battle"):
		return
	# 双保险：主控已在一场战斗中（主世界被停用，理论上本函数不会跑）时不再触发。
	if bool(_main.get("_in_battle")):
		return
	var player := get_tree().get_first_node_in_group("player") as Node2D
	if player == null or global_position.distance_to(player.global_position) > 44.0:
		return
	_cooldown = 4.0
	_main.call("enter_battle", {"monster_id": battle_monster})

## 战斗结束回调（由 main._settle_world_monsters 调用）。
## 胜利：本怪消失，按 respawn_seconds 在原区重刷；失败/脱离：保留，短冷却后可再生效。
func on_battle_result(won: bool) -> void:
	if won:
		visible = false
		_respawn_left = maxf(0.1, respawn_seconds)
		_moving = false
		_timer = 0.0
		_cooldown = 0.0
	else:
		_cooldown = 5.0
