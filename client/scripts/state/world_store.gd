extends Node
## 状态层 · world 域（前端规范 §3.3）。存服务端下发的权威世界状态与本地 UI 状态。
## 位置广播/远端玩家来自服务端推送（3/3 移动广播、3/6 加入、3/5 离场）。

signal dialogue_requested(npc_id: String)
signal near_npc_changed(npc_id: String)
signal snapshot_applied
signal remote_changed

var current_map: String = "清河村"
var map_peace: bool = true
var near_npc_id: String = ""
# 进场快照（服务端权威）：本人出生位置与场景逻辑数据。
var self_x: float = 400.0
var self_y: float = 900.0
var ground_y: float = 900.0
var walk_bounds: Vector2 = Vector2(80, 2272)
# 俯视二维边界与出生点（服务端 3/1 bounds / spawn，缺省回落横版口径）。
var bounds_x: Vector2 = Vector2(80, 2272)
var bounds_y: Vector2 = Vector2(0, 1008)
var spawn: Vector2 = Vector2(400, 900)
var line_no: int = 1  # 当前分线（V1.27）
# 远端玩家（服务端权威）：{char_id: {name, x, y}}
var remote_players: Dictionary = {}

func _ready() -> void:
	Network.packet_received.connect(_on_packet)

## 应用 3/1 进场快照（服务端权威状态，客户端不自行结算）。
func apply_snapshot(snapshot: Dictionary) -> void:
	current_map = str(snapshot.get("map_id", current_map))
	var self_info: Dictionary = snapshot.get("self", {})
	self_x = float(self_info.get("x", self_x))
	self_y = float(self_info.get("y", self_y))
	ground_y = float(snapshot.get("ground_y", ground_y))
	var walk: Dictionary = snapshot.get("walk", {})
	walk_bounds = Vector2(float(walk.get("x_min", walk_bounds.x)), float(walk.get("x_max", walk_bounds.y)))
	var bd: Dictionary = snapshot.get("bounds", {})
	bounds_x = Vector2(float(bd.get("x_min", walk_bounds.x)), float(bd.get("x_max", walk_bounds.y)))
	bounds_y = Vector2(float(bd.get("y_min", bounds_y.x)), float(bd.get("y_max", bounds_y.y)))
	var sp: Dictionary = snapshot.get("spawn", {})
	spawn = Vector2(float(sp.get("x", self_x)), float(sp.get("y", self_y)))
	remote_players.clear()
	for p in snapshot.get("players", []):
		var info: Dictionary = p
		remote_players[int(info.get("id", 0))] = {
			"name": str(info.get("name", "?")),
			"x": float(info.get("x", self_x)),
			"y": float(info.get("y", ground_y)),
			# 服务端暂未下发门派/性别：留空字段，客户端按 char_id 稳定分配皮肤。
			"faction": str(info.get("faction", "")),
			"gender": str(info.get("gender", "")),
			"mount": info.get("mount"),  # V1.27 同屏范围预留（M3 前恒 null）
			"pet": info.get("pet"),
		}
	line_no = int(snapshot.get("line_no", line_no))
	snapshot_applied.emit()
	remote_changed.emit()

## 请求分线列表（3/7），返回回包（code/lines/current）。
func fetch_lines() -> Dictionary:
	return await Network.send_and_wait(3, 7, {"map_id": current_map})

## 切线（3/8）：成功后服务端回快照，应用之。
func switch_line(line_no_target: int) -> String:
	var resp: Dictionary = await Network.send_and_wait(3, 8, {"line_no": line_no_target})
	var code := int(resp.get("code", -1))
	if code == 0:
		apply_snapshot(resp)
		return ""
	return str(resp.get("msg", "切线失败"))

func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	match [main_code, sub_code]:
		[3, 3]:  # 3/3 移动广播（服务端已排除本人）
			var id := int(payload.get("id", 0))
			if remote_players.has(id):
				remote_players[id]["x"] = float(payload.get("x", 0.0))
				remote_players[id]["y"] = float(payload.get("y", ground_y))
				remote_changed.emit()
		[3, 6]:  # 3/6 加入推送
			var id := int(payload.get("id", 0))
			remote_players[id] = {
				"name": str(payload.get("name", "?")),
				"x": float(payload.get("x", 0.0)),
				"y": float(payload.get("y", ground_y)),
				"faction": str(payload.get("faction", "")),
				"gender": str(payload.get("gender", "")),
				"mount": payload.get("mount"),  # V1.27 同屏范围预留
				"pet": payload.get("pet"),
			}
			remote_changed.emit()
		[3, 5]:  # 3/5 离场推送
			remote_players.erase(int(payload.get("id", 0)))
			remote_changed.emit()

func set_near_npc(npc_id: String) -> void:
	if near_npc_id != npc_id:
		near_npc_id = npc_id
		near_npc_changed.emit(npc_id)

## 仅当当前记录的正是自己时才清除，避免多 NPC 互相覆盖邻近状态。
func clear_near_npc(npc_id: String) -> void:
	if near_npc_id == npc_id:
		set_near_npc("")

func request_dialogue(npc_id: String) -> void:
	dialogue_requested.emit(npc_id)

func location_banner() -> String:
	return "%s · %s" % [current_map, "和平" if map_peace else "战斗"]
