extends Node2D
## 渲染层 · 世界场景挂载：按 WorldStore.remote_players 同步远端玩家节点，
## 并按相机视口做"视口外只留名字"裁剪（V1.27 性能降级线）。

@onready var _container: Node2D = $RemotePlayers
@onready var _camera: Camera2D = $Camera

const RemotePlayerScript := preload("res://scripts/game/remote_player.gd")
const CULL_BUFFER := 200.0  # 视口外缓冲（px），避免边界闪烁

func _ready() -> void:
	WorldStore.remote_changed.connect(_sync_remote)
	_sync_remote()

func _sync_remote() -> void:
	var seen := {}
	for id in WorldStore.remote_players:
		seen[id] = true
		var info: Dictionary = WorldStore.remote_players[id]
		var node := _container.get_node_or_null(str(id))
		if node == null:
			node = Node2D.new()
			node.name = str(id)
			node.set_script(RemotePlayerScript)
			_container.add_child(node)
			node.setup(id, info.get("name", "?"), info.get("x", 0.0), info.get("y", 900.0), info)
		node.target_x = info.get("x", node.target_x)
		node.apply_companion(info)
	for child in _container.get_children():
		if not seen.has(int(child.name)):
			child.queue_free()

func _process(_delta: float) -> void:
	# 视口裁剪：相机中心 ± 半视宽 + 缓冲。视口外 set_culled(true)（只留名牌）。
	var center_x := _camera.get_screen_center_position().x
	var half_width := 640.0 + CULL_BUFFER
	for child in _container.get_children():
		child.set_culled(absf(child.target_x - center_x) > half_width)
