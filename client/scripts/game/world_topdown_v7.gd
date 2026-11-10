extends Node2D
## 2.5D 斜俯视定稿图试点 · 清河村 v7（UI 规范 V1.42 / 前端规范 §4）。
## 场景与逻辑分离：底图在 tscn，墙基线碰撞 / 岸线 / 树干基点 / 门洞热区 /
## NPC 点位 / 明雷怪物 / 比例与镜头参数全部读 JSON。
## V1.42 口径：
##  - 碰撞口径（用户 2026-11-11 手画定案）：不再分建筑/水体/树干三类，
##    而是用 4 组 wall_base 凹多边形把建筑占地、河面、山体、院墙等所有不可走地形整片围合，
##    中间留出连贯的石板路；交给 CollisionPolygon2D 的 BUILD_SOLIDS 做凸分解。
##    这样屋顶、桥拱洞口、下游河面天然都在障碍内，不必再单独描岸线或墙基线；
##  - 精灵间 Y-Sort（按脚底 y），整图底图不参与排序；
##  - 镜头 zoom 1.6、禁止整图同屏；人物 62 图内 px（门洞 ×0.9）、移速 200 px/s。
## 未接入（下一阶段）：点按避碰撞寻路、服务端 x/y 校验、NPC 对话与任务、主流程切换。

## 逻辑数据默认走 v7；场景可用 metadata/logic_path 指向其它版本（如扩张版 v8）。
const DEFAULT_LOGIC := "res://data/qinghecun_v7_logic.json"
const MonsterScript := preload("res://scripts/game/topdown_monster.gd")
const NpcScript := preload("res://scripts/game/topdown_npc.gd")
const RemotePlayerScript := preload("res://scripts/game/remote_player.gd")

var logic: Dictionary = {}
var _hint: Label
var _in_main := false
var _remote_nodes := {}
var _logic_path := DEFAULT_LOGIC

func _ready() -> void:
	_logic_path = str(get_meta("logic_path", DEFAULT_LOGIC))
	var parsed: Variant = JSON.parse_string(FileAccess.get_file_as_string(_logic_path))
	if not (parsed is Dictionary):
		push_error("[topdown] 逻辑数据读取失败: " + _logic_path)
		return
	logic = parsed
	_build_collisions()
	_build_interactions()
	_build_npcs()
	_build_monsters()
	_setup_player_and_camera()
	_build_hint()
	WorldStore.remote_changed.connect(_sync_remote)
	_sync_remote()

func _build_collisions() -> void:
	var layer := get_node_or_null("Collisions") as Node2D
	if layer == null:
		layer = Node2D.new()
		layer.name = "Collisions"
		add_child(layer)
	for raw in logic.get("collisions", []):
		var c: Dictionary = raw
		var body := StaticBody2D.new()
		body.name = "Wall_" + str(c.get("name", ""))
		body.set_meta("kind", str(c.get("kind", "wall_base")))
		var poly := CollisionPolygon2D.new()
		if c.has("polygon"):
			var pts := PackedVector2Array()
			for xy in c.get("polygon", []):
				pts.append(Vector2(float((xy as Array)[0]), float((xy as Array)[1])))
			poly.polygon = pts
		else:
			# 树干基点：以中心 + 半径生成八边形小碰撞，树冠不入碰撞。
			var center: Array = c.get("center", [0, 0])
			var r := float(c.get("radius", 10))
			var pts2 := PackedVector2Array()
			for i in range(8):
				var ang := TAU * float(i) / 8.0
				pts2.append(Vector2(float(center[0]) + cos(ang) * r, float(center[1]) + sin(ang) * r))
			poly.polygon = pts2
		body.add_child(poly)
		layer.add_child(body)

func _build_interactions() -> void:
	var layer := get_node_or_null("Interactions") as Node2D
	if layer == null:
		layer = Node2D.new()
		layer.name = "Interactions"
		add_child(layer)
	for raw in logic.get("interactions", []):
		var z: Dictionary = raw
		var area := Area2D.new()
		area.name = "Door_" + str(z.get("name", ""))
		var door: Array = z.get("door", [0, 0])
		area.position = Vector2(float(door[0]), float(door[1]))
		var shape := CollisionShape2D.new()
		var circ := CircleShape2D.new()
		circ.radius = float(z.get("radius", 60))
		shape.shape = circ
		area.add_child(shape)
		layer.add_child(area)

func _build_npcs() -> void:
	var ys := get_node_or_null("YSortLayer") as Node2D
	if ys == null:
		return
	var rd: Dictionary = logic.get("render", {})
	for raw in logic.get("npcs", []):
		var info: Dictionary = raw
		var node := Node2D.new()
		node.name = "NPC_" + str(info.get("id", ""))
		node.position = Vector2(float(info.get("x", 0)), float(info.get("y", 0)))
		# 承接横版 npc.gd：靠近显示「E 对话」、点按/E 触发 WorldStore 对话与任务记账。
		node.set_script(NpcScript)
		node.set("npc_id", str(info.get("id", "")))
		node.set("cfg", info)
		node.set("render", rd)
		ys.add_child(node)

func _build_monsters() -> void:
	var ys := get_node_or_null("YSortLayer") as Node2D
	if ys == null:
		return
	var tex := load("res://assets/sprites/monsters/qingya_wolf.png") as Texture2D
	for raw in logic.get("monsters", []):
		var m: Dictionary = raw
		var node := Node2D.new()
		node.name = "Monster_" + str(m.get("id", ""))
		node.position = Vector2(float(m.get("x", 0)), float(m.get("y", 0)))
		node.set_script(MonsterScript)
		var sp := Sprite2D.new()
		sp.name = "Sprite2D"
		if tex != null:
			sp.texture = tex
		var sc := float(m.get("scale", 0.157))
		sp.scale = Vector2(sc, sc)
		sp.position = Vector2(0, -26)
		node.add_child(sp)
		# set_script 后、add_child 前注入配置：_ready() 里会按这些值初始化静止计时器。
		node.set("range_x", float(m.get("range", 60)))
		node.set("move_speed", float(m.get("speed", 40)))
		# V1.43 明雷节奏：静止 3–4s → 横移约 1s / 40px → 停。冒烟可用小值加速验证。
		node.set("idle_min", float(m.get("idle_min", 3.0)))
		node.set("idle_max", float(m.get("idle_max", 4.0)))
		node.set("move_time", float(m.get("move_time", 1.0)))
		# V1.41 明雷接触开战：仅在主流程（有 Main 节点）时生效，独立试点不触发。
		node.set("battle_monster", str(m.get("battle_monster", "")))
		ys.add_child(node)

func _setup_player_and_camera() -> void:
	var p := get_node_or_null("YSortLayer/Player") as CharacterBody2D
	if p == null:
		return
	var spawn: Array = logic.get("spawn", [520, 700])
	p.position = Vector2(float(spawn[0]), float(spawn[1]))
	p.configure(logic.get("render", {}))
	# 仅主流程开启服务端位置同步：独立冒烟/实拍场景不触碰网络。
	p.set("net_sync", bool(logic.get("net_sync", false)) and _in_main_ready())
	var cam := p.get_node_or_null("Camera") as Camera2D
	if cam != null:
		var c: Dictionary = logic.get("camera", {})
		cam.limit_left = int(c.get("left", 0))
		cam.limit_top = int(c.get("top", 0))
		cam.limit_right = int(c.get("right", 1536))
		cam.limit_bottom = int(c.get("bottom", 1024))
		var z := float(c.get("zoom", 1.6))
		cam.zoom = Vector2(z, z)

func _build_hint() -> void:
	var canvas := CanvasLayer.new()
	canvas.layer = 3  # 主流程 UILayer 之上、战斗/对话层之下
	add_child(canvas)
	_in_main = get_tree().root.find_child("DialogueLayer", true, false) != null
	_hint = Label.new()
	_hint.add_theme_font_size_override("font_size", 12 if _in_main else 15)
	_hint.add_theme_color_override("font_color", Color("F6EFDF"))
	var panel := PanelContainer.new()
	panel.position = Vector2(430, 660) if _in_main else Vector2(12, 10)
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.12, 0.10, 0.08, 0.80)
	sb.set_corner_radius_all(6)
	sb.content_margin_left = 12
	sb.content_margin_right = 12
	sb.content_margin_top = 6
	sb.content_margin_bottom = 6
	panel.add_theme_stylebox_override("panel", sb)
	panel.add_child(_hint)
	canvas.add_child(panel)
	_refresh_hint()

func _refresh_hint() -> void:
	if _hint == null:
		return
	var p := get_node_or_null("YSortLayer/Player") as CharacterBody2D
	var xy := p.position if p != null else Vector2.ZERO
	if _in_main:
		_hint.text = "WASD 移动 | E 对话 | 左键点按 | 坐标 (%d, %d)" % [int(xy.x), int(xy.y)]
		return
	var label := str(logic.get("map_label", "清河村 · 2.5D 斜俯视"))
	var z := float(logic.get("camera", {}).get("zoom", 1.6))
	var ch := int(logic.get("render", {}).get("character_height_px", 62))
	_hint.text = "%s | WASD/方向键 | 左键点按 | Esc 取消 | zoom %.1f · 人物 %dpx | 坐标 (%d, %d)" % [label, z, ch, int(xy.x), int(xy.y)]

func _process(_delta: float) -> void:
	_refresh_hint()

func _in_main_ready() -> bool:
	return get_tree().root.find_child("DialogueLayer", true, false) != null

## 同屏远端玩家：放进 YSortLayer 参与前后遮挡，按服务端权威 x/y 插值移动。
func _sync_remote() -> void:
	var ys := get_node_or_null("YSortLayer") as Node2D
	if ys == null:
		return
	var rd: Dictionary = logic.get("render", {})
	var sc := float(rd.get("sprite_scale", 0.28))
	for id in WorldStore.remote_players:
		var info: Dictionary = WorldStore.remote_players[id]
		var node := _remote_nodes.get(id) as Node2D
		if node == null or not is_instance_valid(node):
			node = Node2D.new()
			node.name = "Remote_%d" % int(id)
			node.set_script(RemotePlayerScript)
			node.set("sprite_scale", sc)
			node.set("sprite_offset_y", float(rd.get("sprite_offset_y", -32)))
			node.set("plate_offset_y", -96.0)
			ys.add_child(node)
			node.setup(int(id), str(info.get("name", "?")),
					float(info.get("x", 0.0)), float(info.get("y", 0.0)), info)
			_remote_nodes[id] = node
		node.set("target_x", float(info.get("x", 0.0)))
		node.set("target_y", float(info.get("y", 0.0)))
		node.call("apply_companion", info)
	var gone := []
	for id in _remote_nodes:
		if not WorldStore.remote_players.has(id):
			var stale = _remote_nodes[id]
			if is_instance_valid(stale):
				stale.queue_free()
			gone.append(id)
	for id in gone:
		_remote_nodes.erase(id)
