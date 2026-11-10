extends Control
## UI 层 · 战斗场景（M2a-2）：1v1 / 4v4 站位、帧事件播放、顶部行动顺序条、指令台。
## 帧模型：4/1 快照 → 4/3 事件帧（order/damage/shock/dot/reflect/...）→ 4/4 确认 → 4/5 终局；4/7 恢复。
## M2a-2：units[] 多单位按 slot 布局，点击敌方单位设当前目标（4/2 带 target_id）。

signal battle_over

# V1.28：次级文字令牌 --fj-ink-2 由 #7A6748 改深为 #5F4B2E，并要求对比度 >= 4.5:1。
# 飘字落在明暗变化大的手绘战场上，状态色在原基础上再压深一档，并加深褐描边 + 投影。
const COLOR_DMG := Color("C0491B")      # 物伤橙红（加深）
const COLOR_CRIT := Color("9E1B10")     # 暴击深朱红（加大字号）
const COLOR_MAGIC := Color("4B2A6B")    # 法伤墨紫（规范 §5）
const COLOR_HEAL := Color("2F6B3C")     # 治疗绿（加深）
const COLOR_SKILL := Color("8A3410")     # 秘籍名书法题字（朱橙加深）
const COLOR_MISS := Color("5F4B2E")     # 次级提示 = V1.28 --fj-ink-2
const COLOR_SHOCK := Color("5B3A93")    # 控制/感电紫（加深）
const COLOR_REFLECT := Color("8A6410")   # 反伤暗金（加深）
const COLOR_PAPER := Color("f6efdf")
const COLOR_ACCENT := Color("c46a2b")
const BG_DIR := "res://assets/battle/bg_"
const BG_CFG := "res://data/battle_backgrounds.json"
const BG_LEGACY := "res://assets/battle/bg_zangjianyuan_1280.png"
# 战场可用区：顶部顺序条底 64 + 名牌约 50，底部指令台上沿 580。
# 立绘脚底落在 420 时，人物视觉中心约 339，正好落在可用区中部。
const STAGE_CENTER_Y := 420.0
const STAGE_ROW_GAP := 200.0
const STAGE_COL_GAP_SELF := 150.0
const STAGE_COL_GAP_ENEMY := 165.0
var _side_counts: Dictionary = {}

var _bg_cfg_cache: Dictionary = {}
var _bg_path := ""
const SPEEDS := [1.0, 2.0, 3.0]
var _speed_index := 0
var _speed_btn: Button
var _auto_btn: Button



## 特殊效果单字标记（V1.28 未列映射表，按战报 flags 约定；服务端新增 flag 即自动生效）。
## 例：暴击「爆 37」、吸血「吸 37」、连击+法术「连法 37」。
## 未实现的效果（吸血/反击等）只预留映射，不伪造战斗数据。
const DMG_TAGS := {
	"crit": "爆", "combo": "连", "lifesteal": "吸", "magic": "法",
	"reflect": "反", "dot": "电", "shock": "雷", "counter": "反击",
	"pierce": "破", "block": "御",
}

var _theme: Theme = preload("res://resources/theme_paper.tres")
var _stage: Node2D
var _bg: TextureRect
var _cmd_panel: Control
var _order_box: HBoxContainer
var _turn_label: Label
var _top_right: HBoxContainer
var _circle_shader: Shader = preload("res://resources/circle_avatar.gdshader")
var _units := {}          # id → node
var _unit_sides := {}     # id → side
var _queue: Array = []    # 待播放事件
var _playing := false
var _turn_version := -1
var _acked_turn := -1
var _result_label: Label
var _battle_id := ""
var _battle_over := false
var _target_id := 0
var _pending_cmd := {}  # 4v4 待选目标：{cmd, skill_id}
var _last_cmd_payload := {}  # 最近一次真正发出的 4/2 负载（调试与冒烟断言）
var _order_ids: Array = []
var _order_names: Array = []
var _event_interval := 0.45

# --- 战斗贴图（素材：资产素材/美术素材/sprites/frames_<门派><性别>、monsters/single） ---
const SPRITE_BATTLE := "res://assets/sprites/battle/"
const SPRITE_MONSTER := "res://assets/sprites/monsters/"
## 怪物配置 id → 立绘（服务器只下发显示名，故按名映射）。
## 木桩人为占位：专属立绘未产出，暂用巡山石傀（同为"非活体"造型）。
const MONSTER_ART := {
	"木桩人": "shikui",
	"野狼": "qingya_wolf",
	"山贼": "fallen_soldier",
	"荆棘武士": "mirage_flower",
}
const ALLY_HEIGHT := 162.0
const ENEMY_HEIGHT := 150.0
const ANIM_FPS := 7.0
var _unit_anim := {}   # uid → {sprite, idle:[Texture2D], attack:[Texture2D], t, f, mode}
var _unit_heights := {}  # uid → 立绘高度（飘字定位用，随贴图尺寸变化）

func _ready() -> void:
	theme = _theme
	_fit_viewport()
	# main.tscn 里 BattleLayer 的父节点是普通 Node（非 Control / CanvasLayer），
	# 窗口尺寸变化不会沿父链传播到本 Control 的锚点，_ready 时的初始小矩形会被锁死，
	# 实机就只渲染左上角一小块、单位站位全落在可视区外。显式跟随视口尺寸。
	get_viewport().size_changed.connect(_fit_viewport)
	visible = false
	Network.packet_received.connect(_on_packet)
	# 重连成功后请求 4/7 恢复（不在战斗时服务端回 code!=0，客户端忽略）。
	Network.authenticated_ok.connect(_on_authenticated)
	_build_stage()
	_build_order_bar()
	_build_cmd_panel()
	_fit_viewport()

## 逐帧自愈：可见期间校验一次尺寸。
## 历史 bug——窗口/视口尺寸变化被父链吞掉时，战斗层会永久停在 _ready 时的小矩形，
## 表现为「只渲染左上角一小块、单位全在可视区外」。这里兜底，宁可每帧比一次。
func _process(delta: float) -> void:
	if not visible:
		return
	var vp: Vector2 = get_viewport().get_visible_rect().size
	if absf(size.x - vp.x) > 0.5 or absf(size.y - vp.y) > 0.5:
		_fit_viewport()
	_tick_anim(delta)

## 强制铺满视口，并让子节点重新计算锚点（不依赖父 Control 的尺寸传播）。
func _fit_viewport() -> void:
	var vp: Vector2 = get_viewport().get_visible_rect().size
	# 窗口刚创建/被 DPI 缩放时视口尺寸可能瞬时过小，用窗口物理尺寸兜底，
	# 否则会把战斗层锁成一小块（就是历史 bug 的形态）。
	if vp.x < 64.0 or vp.y < 64.0:
		var scale: float = maxf(get_window().content_scale_factor, 0.01)
		vp = Vector2(get_window().size) / scale
	set_anchors_preset(Control.PRESET_TOP_LEFT)
	position = Vector2.ZERO
	size = vp
	if _bg:
		_bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	if _cmd_panel:
		_cmd_panel.set_anchors_and_offsets_preset(Control.PRESET_BOTTOM_WIDE)
		_cmd_panel.offset_top = -140

## 地点 → 战斗背景：优先 4/1 回包的 map_id（服务端可指定战场），否则当前世界地图；
## 演练场按 modes 指定；未登记地点回退 default，缺图再回退旧定稿图。
## 映射表 data/battle_backgrounds.json 是唯一真相，新增地图只改 JSON。
func _battle_bg_path(map_id: String, mode: String) -> String:
	if _bg_cfg_cache.is_empty():
		var data: Variant = JSON.parse_string(FileAccess.get_file_as_string(BG_CFG))
		if data is Dictionary:
			_bg_cfg_cache = data
	var def := str(_bg_cfg_cache.get("default", "common_wild"))
	var key := def
	var modes: Dictionary = _bg_cfg_cache.get("modes", {})
	var aliases: Dictionary = _bg_cfg_cache.get("aliases", {})
	if modes.has(mode):
		key = str(modes[mode])
	elif aliases.has(map_id):
		key = str(aliases[map_id])
	var path := BG_DIR + key + ".jpg"
	if ResourceLoader.exists(path):
		return path
	var fallback := BG_DIR + def + ".jpg"
	if ResourceLoader.exists(fallback):
		return fallback
	return BG_LEGACY

func _apply_battle_bg(map_id: String, mode: String) -> void:
	if _bg == null:
		return
	var path := _battle_bg_path(map_id, mode)
	if path == _bg_path:
		return
	_bg_path = path
	_bg.texture = load(path)

func _build_stage() -> void:
	# 整幅手绘战场铺满；底图按地点取自 资产素材/美术素材/战斗背景图/，
	# 由 _apply_battle_bg 在每次进战斗时按 map_id / mode 切换。
	var bg := TextureRect.new()
	bg.name = "BattleBG"
	bg.texture = load(_battle_bg_path(WorldStore.current_map, ""))
	bg.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	bg.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	bg.mouse_filter = Control.MOUSE_FILTER_IGNORE
	bg.set_anchors_and_offsets_preset(Control.PRESET_FULL_RECT)
	add_child(bg)
	_bg = bg
	# 顶部压一层淡墨，保证深色回合牌/头像框与宣纸 UI 在亮天空上可读。
	var shade_top := ColorRect.new()
	shade_top.color = Color(0.15, 0.11, 0.07, 0.26)
	shade_top.set_anchors_preset(Control.PRESET_TOP_WIDE)
	shade_top.offset_bottom = 90
	shade_top.mouse_filter = Control.MOUSE_FILTER_IGNORE
	add_child(shade_top)
	_stage = Node2D.new()
	add_child(_stage)

## 顶部信息层（定稿）：左上「第 N 回合」深色圆角牌、居中圆形头像行动序列、
## 右上「倍速 ×2」「自动·开」。UI 规范 V1.27 §5 顶部信息牌口径。
func _build_order_bar() -> void:
	# 左上回合牌
	_turn_label = Label.new()
	_turn_label.name = "TurnLabel"
	_turn_label.text = "第 1 回合"
	_turn_label.add_theme_font_size_override("font_size", 16)
	_turn_label.add_theme_color_override("font_color", Color("f6efdf"))
	var round_style := StyleBoxFlat.new()
	round_style.bg_color = Color(0.12, 0.12, 0.14, 0.92)
	round_style.set_corner_radius_all(10)
	round_style.content_margin_left = 18
	round_style.content_margin_right = 18
	round_style.content_margin_top = 6
	round_style.content_margin_bottom = 6
	var turn_panel := PanelContainer.new()
	turn_panel.add_theme_stylebox_override("panel", round_style)
	turn_panel.set_anchors_preset(Control.PRESET_TOP_LEFT)
	turn_panel.offset_left = 24
	turn_panel.offset_top = 18
	turn_panel.add_child(_turn_label)
	add_child(turn_panel)

	# 居中圆形头像序列
	var panel := PanelContainer.new()
	panel.set_anchors_preset(Control.PRESET_CENTER_TOP)
	panel.offset_left = -280
	panel.offset_right = 280
	panel.offset_top = 8
	panel.offset_bottom = 64
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.10, 0.09, 0.08, 0.86)
	sb.set_corner_radius_all(10)
	sb.border_width_left = 2
	sb.border_width_right = 2
	sb.border_width_top = 2
	sb.border_width_bottom = 2
	sb.border_color = Color("c9b586")
	sb.content_margin_left = 8
	sb.content_margin_right = 8
	sb.content_margin_top = 5
	sb.content_margin_bottom = 5
	panel.add_theme_stylebox_override("panel", sb)
	add_child(panel)
	_order_box = HBoxContainer.new()
	_order_box.alignment = BoxContainer.ALIGNMENT_CENTER
	_order_box.add_theme_constant_override("separation", 4)
	panel.add_child(_order_box)

	# 右上倍速 / 自动：纯客户端演出与托管控制，不需要 4/2 协议支持。
	_top_right = HBoxContainer.new()
	_top_right.set_anchors_preset(Control.PRESET_TOP_RIGHT)
	_top_right.offset_left = -220
	_top_right.offset_right = -24
	_top_right.offset_top = 18
	_top_right.offset_bottom = 54
	_top_right.alignment = BoxContainer.ALIGNMENT_END
	_top_right.add_theme_constant_override("separation", 10)
	add_child(_top_right)
	_speed_btn = _top_button("倍速 ×1")
	_speed_btn.pressed.connect(_on_speed_pressed)
	_top_right.add_child(_speed_btn)
	_auto_btn = _top_button("自动·关")
	_auto_btn.pressed.connect(_on_auto_pressed)
	_top_right.add_child(_auto_btn)

func _top_button(text: String) -> Button:
	var b := Button.new()
	b.text = text
	b.add_theme_font_size_override("font_size", 14)
	b.custom_minimum_size = Vector2(90, 34)
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color(0.12, 0.12, 0.14, 0.92)
	sb.set_corner_radius_all(10)
	b.add_theme_stylebox_override("normal", sb)
	b.add_theme_stylebox_override("hover", sb)
	b.add_theme_stylebox_override("pressed", sb)
	b.add_theme_color_override("font_color", Color("f6efdf"))
	return b

## 倍速：只缩短事件播放间隔（贴图表情/飘字节奏），服务端结算与协议完全不变。
func _apply_speed() -> void:
	var total := int(_side_counts.get(0, 1)) + int(_side_counts.get(1, 1))
	var base := 0.16 if total > 2 else 0.45
	_event_interval = base / float(SPEEDS[_speed_index])
	if _speed_btn:
		_speed_btn.text = "倍速 ×%d" % int(SPEEDS[_speed_index])

func _on_speed_pressed() -> void:
	_speed_index = (_speed_index + 1) % SPEEDS.size()
	_apply_speed()

## 自动：把指令台倒计时压到 0.15s，复用既有的「倒计时归零自动普攻」路径。
## 发出的仍是合法 4/2（普攻 + 默认锁定目标），不伪造托管协议。
func _on_auto_pressed() -> void:
	var on: bool = not bool(_cmd_panel.auto_play)
	_cmd_panel.set_auto(on)
	_auto_btn.text = "自动·开" if on else "自动·关"
	var sb := StyleBoxFlat.new()
	sb.bg_color = Color("B34A14") if on else Color(0.12, 0.12, 0.14, 0.92)
	sb.set_corner_radius_all(10)
	_auto_btn.add_theme_stylebox_override("normal", sb)
	_auto_btn.add_theme_stylebox_override("hover", sb)
	_auto_btn.add_theme_stylebox_override("pressed", sb)

## 圆形头像（定稿的行动序列是圆头像，不是文字标签）。
func _portrait(tex: Texture2D, active: bool) -> Control:
	var wrap := PanelContainer.new()
	wrap.custom_minimum_size = Vector2(44, 44)
	var frame := StyleBoxFlat.new()
	frame.bg_color = Color(0.20, 0.17, 0.13, 1)
	frame.set_corner_radius_all(22)
	frame.border_width_left = 2
	frame.border_width_right = 2
	frame.border_width_top = 2
	frame.border_width_bottom = 2
	frame.border_color = COLOR_ACCENT if active else Color("8d7a58")
	wrap.add_theme_stylebox_override("panel", frame)
	var pic := TextureRect.new()
	pic.texture = tex
	pic.expand_mode = TextureRect.EXPAND_IGNORE_SIZE
	pic.stretch_mode = TextureRect.STRETCH_KEEP_ASPECT_COVERED
	pic.custom_minimum_size = Vector2(38, 38)
	pic.material = ShaderMaterial.new()
	(pic.material as ShaderMaterial).shader = _circle_shader
	wrap.add_child(pic)
	return wrap

## 行动序列头像：我方取门派头像，敌方取怪物立绘。
func _portrait_tex(i: int) -> Texture2D:
	var tex: Texture2D = null
	if i < _order_ids.size():
		var uid := int(_order_ids[i])
		if _unit_sides.get(uid, 0) == 1:
			var art := str(MONSTER_ART.get(str(_order_names[i]), ""))
			if art != "" and ResourceLoader.exists(SPRITE_MONSTER + art + ".png"):
				tex = load(SPRITE_MONSTER + art + ".png")
	if tex == null:
		var f := SessionStore.faction if SessionStore.faction != "" else "断岳剑宗"
		var g := SessionStore.gender if SessionStore.gender != "" else "男"
		# 我方 4 人头像按门派轮换（与立绘一致，定稿是不同配色的头像列）。
		var pool: Array = ["惊雷殿", "断岳剑宗", "玄水宫", "璇玑星宫"]
		if i > 0:
			f = str(pool[i % pool.size()])
		var path := "res://assets/avatars/%s_%s.png" % [f, g]
		if ResourceLoader.exists(path):
			tex = load(path)
	return tex

func _build_cmd_panel() -> void:
	_cmd_panel = preload("res://scripts/battle/cmd_panel.gd").new()
	_cmd_panel.set_anchors_and_offsets_preset(Control.PRESET_BOTTOM_WIDE)
	_cmd_panel.offset_top = -140
	add_child(_cmd_panel)
	_cmd_panel.cmd_submitted.connect(_submit_cmd)
	_cmd_panel.cmd_selected.connect(_on_cmd_selected)

func _on_authenticated(_chars: Array) -> void:
	Network.send(4, Protocol.SUB_BATTLE_RESTORE, {})

## 进战斗（4/1 回包 / 4/7 恢复快照，两者 units[] 同构）。
func enter_battle(snapshot: Dictionary) -> void:
	_fit_viewport()
	_battle_id = str(snapshot.get("battle_id", ""))
	_turn_version = int(snapshot.get("turn_version", -1))
	_acked_turn = -1
	_battle_over = false
	_target_id = 0
	_queue.clear()
	_playing = false
	_order_ids.clear()
	_order_names.clear()
	_unit_anim.clear()
	_unit_heights.clear()
	_render_order(-1)
	if _result_label:
		_result_label.queue_free()
		_result_label = null
	for c in _stage.get_children():
		c.queue_free()
	_units.clear()
	_unit_sides.clear()
	var units: Array = snapshot.get("units", [])
	if units.is_empty():
		# 兼容旧 1v1 快照结构。
		for key in ["self", "enemy"]:
			var legacy: Dictionary = snapshot.get(key, {})
			if not legacy.is_empty():
				units.append(legacy)
	# 多单位时加快播放，避免 4v4 一回合事件排到十几秒（倍速倍率在 _apply_speed 里叠加）。
	var compact := units.size() > 2
	# 按阵营统计人数：站位按「本阵营有几人」判定，而不是看 slot 是否存在
	# （实机 1v1 的 units[] 也带 slot=0）。
	_side_counts = {0: 0, 1: 0}
	for u in units:
		var sd := int((u as Dictionary).get("side", 0))
		_side_counts[sd] = int(_side_counts.get(sd, 0)) + 1
	_speed_index = clampi(Settings.speed_index, 0, SPEEDS.size() - 1)
	_apply_speed()
	for u in units:
		var info: Dictionary = u
		_spawn_unit(info, int(info.get("side", 0)) == 0, compact)
	_apply_battle_bg(str(snapshot.get("map_id", WorldStore.current_map)), str(snapshot.get("mode", "")))
	visible = true
	# 角色卡（定稿左侧）用 4/1 回包的 self，先设真气再建秘籍卡才能正确判断可用性。
	var self_info: Dictionary = snapshot.get("self", {})
	if self_info.is_empty() and not units.is_empty():
		self_info = units[0]
	_cmd_panel.set_player(self_info)
	_cmd_panel.set_skill_list(snapshot.get("skills", []))
	# 4v4（敌方 > 1）启用「选招 → 选目标 → 发动」；1v1 保持点招即打。
	var enemy_count := 0
	for id in _units:
		if int(_unit_sides.get(id, 0)) == 1:
			enemy_count += 1
	_cmd_panel.set_requires_target(enemy_count > 1)
	_reset_cmd()
	_cmd_panel.set_auto(Settings.auto_battle)
	# 默认锁定首个敌方目标，玩家可直接点普攻。
	for id in _units:
		if int(_unit_sides.get(id, 0)) == 1:
			_set_target(int(id))
			break

## 战斗域包分发（4/3 帧 / 4/5 终局 / 4/7 恢复）。
func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	if main_code != 4:
		return
	match sub_code:
		Protocol.SUB_BATTLE_FRAME: _on_frame(payload)
		Protocol.SUB_BATTLE_RESULT: _on_result(payload)
		Protocol.SUB_BATTLE_RESTORE: _on_restore(payload)

func _on_frame(payload: Dictionary) -> void:
	var tv := int(payload.get("turn_version", _turn_version))
	var resend := bool(payload.get("resend", false))
	if resend and tv <= _acked_turn:
		return
	if not resend and tv < _turn_version:
		return
	_turn_version = tv
	_queue.append_array(payload.get("events", []))
	if not _playing:
		_play_next()

func _on_result(payload: Dictionary) -> void:
	if int(payload.get("code", 0)) != 0:
		return
	_show_result(int(payload.get("winner", -1)))

func _on_restore(payload: Dictionary) -> void:
	if int(payload.get("code", 0)) != 0 or not payload.has("battle_id"):
		return
	enter_battle(payload)
	var pending: Array = payload.get("pending", [])
	if not pending.is_empty():
		_queue.append_array(pending)
		if not _playing:
			_play_next()

## 立绘/战斗帧解析：我方按「门派+性别」取 idle/attack 各 4 帧，敌方按怪物名取单帧。
## 素材来源：资产素材/美术素材/sprites/frames_<门派><性别>、monsters/single。
func _art_paths(u: Dictionary, is_self: bool) -> Dictionary:
	if is_self:
		var faction: String = SessionStore.faction if SessionStore.faction != "" else "断岳剑宗"
		var gender: String = SessionStore.gender if SessionStore.gender != "" else "男"
		# 定稿我方 4 人为不同门派配色：玩家用真实门派，托管同伴按 slot 轮换。
		var ALLY_POOL: Array = ["惊雷殿", "断岳剑宗", "玄水宫", "璇玑星宫"]
		var slot := int(u.get("slot", 0))
		if slot > 0:
			var real_faction := faction
			faction = str(ALLY_POOL[slot % ALLY_POOL.size()])
			if faction == real_faction:
				faction = str(ALLY_POOL[(slot + 1) % ALLY_POOL.size()])
		var dir := SPRITE_BATTLE + faction + gender + "/"
		var idle: Array = []
		var atk: Array = []
		for i in range(4):
			var pi := dir + "idle_%d.png" % i
			var pa := dir + "attack_%d.png" % i
			if ResourceLoader.exists(pi):
				idle.append(load(pi))
			if ResourceLoader.exists(pa):
				atk.append(load(pa))
		if idle.is_empty():
			return {}
		return {"idle": idle, "attack": atk}
	var art := str(MONSTER_ART.get(str(u.get("name", "")), ""))
	if art == "":
		return {}
	var path := SPRITE_MONSTER + art + ".png"
	if not ResourceLoader.exists(path):
		return {}
	return {"idle": [load(path)], "attack": []}

## 透明区实际内容框（用于脚底对齐与等比缩放；导入纹理可能带大量留白）。
func _used_rect(tex: Texture2D) -> Rect2i:
	var img := tex.get_image()
	if img == null:
		return Rect2i(0, 0, tex.get_width(), tex.get_height())
	if img.is_compressed():
		img.decompress()
	var r := img.get_used_rect()
	if r.size.x <= 0 or r.size.y <= 0:
		return Rect2i(0, 0, tex.get_width(), tex.get_height())
	return r

## 构建身体节点：优先贴图（脚底对齐节点原点），缺图退回色块占位。
func _build_body(u: Dictionary, is_self: bool) -> Dictionary:
	var target_h := ALLY_HEIGHT if is_self else ENEMY_HEIGHT
	var paths := _art_paths(u, is_self)
	if paths.is_empty():
		var rect := ColorRect.new()
		rect.name = "Body"
		rect.color = Color("6a4a9e") if is_self else Color("8a3a3a")
		rect.size = Vector2(44, 88)
		rect.position = Vector2(-22, -88)
		return {"node": rect, "width": 44.0, "height": 88.0, "sprite": null}
	var tex: Texture2D = paths["idle"][0]
	var used := _used_rect(tex)
	var sc := target_h / maxf(float(used.size.y), 1.0)
	var half := Vector2(tex.get_width(), tex.get_height()) * 0.5
	var sprite := Sprite2D.new()
	sprite.name = "Body"
	sprite.texture = tex
	# 我方战斗帧素材面朝右；我方现在站右侧，需翻面朝左与左侧敌人对视。
	# 敌方怪物立绘为正面构图，翻不翻无视觉差异，保持不翻。
	sprite.flip_h = is_self
	sprite.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST  # 像素风不插值
	sprite.scale = Vector2(sc, sc)
	sprite.offset = Vector2(
		-(used.position.x + used.size.x * 0.5 - half.x),
		-(used.position.y + used.size.y - half.y))
	return {"node": sprite, "width": float(used.size.x) * sc, "height": target_h,
		"sprite": sprite, "idle": paths["idle"], "attack": paths["attack"]}

## 帧动画：idle 常驻循环，attack 播一轮后回 idle（怪物单帧则不动）。
func _tick_anim(delta: float) -> void:
	for uid in _unit_anim.keys():
		var a: Dictionary = _unit_anim[uid]
		var sprite_ref = a.get("sprite")
		if not is_instance_valid(sprite_ref):
			_unit_anim.erase(uid)
			continue
		var sprite: Sprite2D = sprite_ref
		var frames: Array = a["attack"] if a["mode"] == "attack" else a["idle"]
		if frames.size() <= 1:
			continue
		a["t"] += delta
		if a["t"] < 1.0 / ANIM_FPS:
			continue
		a["t"] = 0.0
		a["f"] += 1
		if a["f"] >= frames.size():
			if a["mode"] == "attack":
				a["mode"] = "idle"
				frames = a["idle"]
			a["f"] = 0
		if a["f"] < frames.size():
			sprite.texture = frames[a["f"]]

## 触发攻击帧（无攻击帧的单位保持待机，位移由 _dash_attack 表现）。
func _play_attack(uid: int) -> void:
	var a: Dictionary = _unit_anim.get(uid, {})
	if a.is_empty():
		return
	if a["attack"].size() > 0:
		a["mode"] = "attack"
		a["f"] = 0
		a["t"] = 0.0

## 血/真气条按规范 §2.1 令牌着色（默认 Godot 绿条不符合定稿）。
func _style_bar(bar: ProgressBar, color: Color) -> void:
	var fill := StyleBoxFlat.new()
	fill.bg_color = color
	fill.set_corner_radius_all(3)
	var back := StyleBoxFlat.new()
	back.bg_color = Color(0.15, 0.12, 0.10, 0.85)
	back.set_corner_radius_all(3)
	bar.add_theme_stylebox_override("fill", fill)
	bar.add_theme_stylebox_override("background", back)

## 站位口径（用户 2026-11-09 定案）：
## - 1v1：双方同一水平线对齐，不再斜对角；
## - 1v多 / 4v4：按本阵营人数围绕战场中心上下对称居中，双方同行同 y，
##   两列两行按 slot 错位，行距 200 保证名牌与立绘不重叠。
## compact 参数保留兼容旧调用，实际按 _side_counts 判定。
func _spawn_unit(u: Dictionary, is_self: bool, compact: bool = false) -> void:
	var uid := int(u.get("id", 0))
	var node := Node2D.new()
	node.name = str(uid)
	var side := 0 if is_self else 1
	var side_n := int(_side_counts.get(side, 1))
	# 用户定案：玩家自己站右、敌方/怪物/NPC 站左。
	var pos := Vector2(920.0, STAGE_CENTER_Y) if is_self else Vector2(360.0, STAGE_CENTER_Y)
	var slot := int(u.get("slot", -1))
	if side_n > 1 and slot >= 0:
		var col := slot % 2
		var row := int(slot / 2)
		var rows := ceili(float(side_n) / 2.0)
		var y := STAGE_CENTER_Y + (float(row) - float(rows - 1) * 0.5) * STAGE_ROW_GAP
		if is_self:
			pos = Vector2(865.0 + float(col) * STAGE_COL_GAP_ENEMY, y)
		else:
			pos = Vector2(330.0 - float(col) * STAGE_COL_GAP_SELF, y)
	node.position = pos
	# 身体：优先真实立绘/战斗帧，缺图退回色块（不让缺资源阻断玩法）。
	var body := _build_body(u, is_self)
	var body_h: float = body["height"]
	var body_w: float = body["width"]
	_unit_heights[uid] = body_h
	node.add_child(body["node"])
	if body.get("sprite") != null:
		_unit_anim[uid] = {"sprite": body["sprite"], "idle": body["idle"], "attack": body["attack"],
			"t": 0.0, "f": 0, "mode": "idle"}
	# 目标指示（定稿）：脚下黄色椭圆光圈 + 头顶 ▼（不是半透明方块）。
	var ring := Node2D.new()
	ring.name = "TargetRing"
	ring.visible = false
	node.add_child(ring)
	var ellipse := Line2D.new()
	ellipse.name = "FootRing"
	ellipse.width = 2.0
	ellipse.default_color = Color(0.92, 0.74, 0.25, 0.95)
	ellipse.closed = true
	var rx := body_w * 0.58
	var ry := 9.0
	for i in range(32):
		var ang := TAU * float(i) / 32.0
		ellipse.add_point(Vector2(cos(ang) * rx, -7.0 + sin(ang) * ry))
	ring.add_child(ellipse)
	var arrow := Label.new()
	arrow.name = "Arrow"
	arrow.text = "▼"
	arrow.position = Vector2(-8, -body_h - 50)
	arrow.add_theme_font_size_override("font_size", 16)
	arrow.add_theme_color_override("font_color", Color(0.95, 0.78, 0.30))
	ring.add_child(arrow)
	# 定稿名牌：深色小底 + 浅字，血条紧贴名字下方；我方绿、敌方红。
	var plate := Label.new()
	plate.text = str(u.get("name", "?"))
	plate.position = Vector2(-60, -body_h - 34)
	plate.custom_minimum_size = Vector2(120, 18)
	plate.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	plate.add_theme_font_size_override("font_size", 12)
	plate.add_theme_color_override("font_color", Color("fdf6ea"))
	var plate_bg := StyleBoxFlat.new()
	plate_bg.bg_color = Color(0.12, 0.10, 0.08, 0.72)
	plate_bg.set_corner_radius_all(4)
	plate.add_theme_stylebox_override("normal", plate_bg)
	node.add_child(plate)
	var hpbar := ProgressBar.new()
	hpbar.name = "HP"
	hpbar.max_value = float(u.get("max_hp", 1))
	hpbar.value = float(u.get("hp", 0))
	hpbar.show_percentage = false
	hpbar.position = Vector2(-38, -body_h - 17)
	hpbar.size = Vector2(76, 6)
	_style_bar(hpbar, Color("4E8A4F") if is_self else Color("BE4234"))
	node.add_child(hpbar)
	var mpbar := ProgressBar.new()
	mpbar.name = "MP"
	mpbar.max_value = maxf(float(u.get("max_mp", 1)), 1.0)
	mpbar.value = float(u.get("mp", 0))
	mpbar.show_percentage = false
	mpbar.position = Vector2(-38, -body_h - 10)
	mpbar.size = Vector2(76, 4)
	_style_bar(mpbar, Color("43B3A6"))
	node.add_child(mpbar)
	var state := Label.new()
	state.name = "State"
	state.position = Vector2(-50, -body_h - 48)
	state.custom_minimum_size = Vector2(100, 16)
	state.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	state.add_theme_font_size_override("font_size", 11)
	state.add_theme_color_override("font_color", COLOR_SHOCK)
	state.text = "反伤" if int(u.get("reflect_pct", 0)) > 0 else ""
	node.add_child(state)
	# 敌方单位可点击设为当前目标。
	if not is_self:
		var hit := ColorRect.new()
		hit.color = Color(0, 0, 0, 0)
		hit.position = Vector2(-body_w * 0.55, -body_h - 6)
		hit.size = Vector2(body_w * 1.1, body_h + 6)
		hit.mouse_filter = Control.MOUSE_FILTER_STOP
		hit.gui_input.connect(func(ev: InputEvent) -> void:
			if ev is InputEventMouseButton and ev.pressed and ev.button_index == MOUSE_BUTTON_LEFT:
				_set_target(uid))
		node.add_child(hit)
	_stage.add_child(node)
	_units[uid] = node
	_unit_sides[uid] = 0 if is_self else 1

func _set_target(uid: int) -> void:
	# 待选目标状态下点敌方单位 = 确认发动（4v4 第二段）。
	if not _pending_cmd.is_empty():
		if int(_unit_sides.get(uid, 0)) != 1:
			return  # 只能选敌方
		var info: Dictionary = _pending_cmd
		_pending_cmd = {}
		_target_id = uid
		_render_targets()
		_cmd_panel.mark_submitted()
		_submit_cmd(str(info.get("cmd", "attack")), str(info.get("skill_id", "")))
		return
	_target_id = uid
	_render_targets()

## 目标光圈：待选目标时所有敌方都亮，选中后只留选中者。
func _render_targets() -> void:
	var pending := not _pending_cmd.is_empty()
	for id in _units:
		var ring: Node = _units[id].get_node_or_null("TargetRing")
		if ring is CanvasItem:
			var is_enemy := int(_unit_sides.get(id, 0)) == 1
			(ring as CanvasItem).visible = (int(id) == _target_id) or (pending and is_enemy)

## 选招（4v4 第一段）：进入待选目标，清空旧锁定。
func _on_cmd_selected(cmd: String, skill_id: String) -> void:
	_pending_cmd = {"cmd": cmd, "skill_id": skill_id}
	_target_id = 0
	_render_targets()

func _clear_pending() -> void:
	_pending_cmd.clear()
	_cmd_panel.cancel_pending()

## 新回合 / 发动失败 / 进场时统一复位，避免待选状态跨回合残留。
func _reset_cmd() -> void:
	_clear_pending()
	_render_targets()
	_cmd_panel.show_cmd()

func _unhandled_input(event: InputEvent) -> void:
	if not visible or _pending_cmd.is_empty():
		return
	if event is InputEventKey and event.pressed and event.keycode == KEY_ESCAPE:
		_clear_pending()
		_render_targets()
		get_viewport().set_input_as_handled()

func _play_next() -> void:
	if _queue.is_empty():
		_playing = false
		if _turn_version >= 0:
			Network.send(4, Protocol.SUB_BATTLE_ACK, {"turn_version": _turn_version})
			_acked_turn = _turn_version
		if visible and not _battle_over:
			_reset_cmd()
		return
	_playing = true
	var e: Dictionary = _queue.pop_front()
	_play_event(e)
	await get_tree().create_timer(_event_interval).timeout
	if visible:
		_play_next()

func _play_event(e: Dictionary) -> void:
	var etype: String = str(e.get("type", ""))
	var actor_id: int = int(e.get("actor", 0))
	var target_id: int = int(e.get("target", 0))
	var flags: Dictionary = e.get("flags", {})
	match etype:
		"act":
			_play_attack(actor_id)
			_float_text(_units.get(actor_id), str(flags.get("skill", "")), COLOR_SKILL, true)
			_highlight_actor(actor_id)
		"damage":
			_play_attack(actor_id)  # 普攻不发 act 事件，用伤害事件驱动攻击帧
			_dash_attack(_units.get(actor_id), _units.get(target_id))
			_apply_hp(target_id, -int(e.get("value", 0)))
			var is_crit := bool(flags.get("crit", false))
			var color: Color = COLOR_CRIT if is_crit else (COLOR_MAGIC if bool(flags.get("magic", false)) else COLOR_DMG)
			_float_text(_units.get(target_id), _tagged_number(int(e.get("value", 0)), flags), color, false, is_crit)
			_highlight_actor(actor_id)
		"miss":
			_float_text(_units.get(target_id), "闪避", COLOR_MISS)
		"heal":
			_apply_hp(target_id, int(e.get("value", 0)))
			_float_text(_units.get(target_id), "疗 +" + str(e.get("value", 0)), COLOR_HEAL)
		"control":
			_float_text(_units.get(target_id), "控 " + str(flags.get("type", "被控")), COLOR_SHOCK)
		"control_fail":
			_float_text(_units.get(target_id), "未控住", COLOR_MISS)
		"fallback":
			_float_text(_units.get(actor_id), "回退/托管", COLOR_MISS)
		"defend":
			_float_text(_units.get(actor_id), "防御", COLOR_MISS)
			_highlight_actor(actor_id)
		"buff":
			_float_text(_units.get(target_id), str(flags.get("stat", "增益")), COLOR_HEAL)
		"stunned":
			_float_text(_units.get(actor_id), "被控", COLOR_MISS)
		"flee":
			_float_text(_units.get(actor_id), "脱离", COLOR_MISS)
		"flee_fail":
			_float_text(_units.get(actor_id), "逃脱失败", COLOR_MISS)
		"death":
			_set_dead(target_id)
		"shock":
			_set_state(target_id, "感电×%d" % int(flags.get("stacks", 0)))
			_float_text(_units.get(target_id), "感电+" + str(flags.get("stacks", 0)), COLOR_SHOCK)
		"dot":
			_apply_hp(target_id, -int(e.get("value", 0)))
			_float_text(_units.get(target_id), _tagged_number(int(e.get("value", 0)), flags, ["dot"]), COLOR_SHOCK)
		"reflect":
			_apply_hp(target_id, -int(e.get("value", 0)))
			_float_text(_units.get(target_id), _tagged_number(int(e.get("value", 0)), flags, ["reflect"]), COLOR_REFLECT)
		"order":
			_order_ids = flags.get("ids", [])
			_order_names = flags.get("names", [])
			_render_order(0)
		"turn_end":
			if not _battle_over:
				_reset_cmd()
		"flag", "special_deferred":
			pass  # 特效延后登记
		"end":
			_show_result(int(flags.get("winner", -1)))

## 行动顺序条：按 order 事件的顺序渲染名字，当前行动者朱橙 + ▼。
func _render_order(active: int) -> void:
	if _order_box == null:
		return
	for c in _order_box.get_children():
		c.visible = false
		c.queue_free()
	for i in range(_order_names.size()):
		var tex := _portrait_tex(i)
		if tex == null:
			continue
		var slot := _portrait(tex, i == active)
		slot.tooltip_text = str(_order_names[i])
		_order_box.add_child(slot)
	# 顺序条宽度随参战人数自适应：1v1 不出现空黑框，4v4 仍容纳 8 个圆头像。
	var frame := _order_box.get_parent() as PanelContainer
	if frame:
		# 进战斗瞬间还没有行动序，别挂一个空黑框；收到 order 事件后再显示。
		frame.visible = not _order_names.is_empty()
		var n := maxi(_order_names.size(), 1)
		var half := clampf(float(n) * 24.0 + 12.0, 62.0, 280.0)
		frame.offset_left = -half
		frame.offset_right = half
	if _turn_label:
		_turn_label.text = "第 %d 回合" % maxi(_turn_version + 1, 1)

func _highlight_actor(actor_id: int) -> void:
	if actor_id == 0 or _order_ids.is_empty():
		return
	for i in range(_order_ids.size()):
		if int(_order_ids[i]) == actor_id:
			_render_order(i)
			return

func _set_state(unit_id: int, text: String) -> void:
	var node: Node2D = _units.get(unit_id)
	if node == null:
		return
	var lab := node.get_node_or_null("State") as Label
	if lab:
		lab.text = text

func _set_dead(unit_id: int) -> void:
	var node: Node2D = _units.get(unit_id)
	if node:
		node.modulate = Color(0.4, 0.4, 0.4, 0.6)
	_set_state(unit_id, "倒下")

func _dash_attack(actor_node: Node2D, target_node: Node2D) -> void:
	if actor_node == null or target_node == null or not Settings.vfx:
		return
	var orig := actor_node.position
	# 从右往左冲时停在目标左侧，反之停在右侧；换边后不再固定 +60。
	var dir := -1.0 if orig.x > target_node.position.x else 1.0
	var tw := create_tween()
	tw.tween_property(actor_node, "position", target_node.position + Vector2(60 * dir, 40), 0.18)
	tw.tween_property(actor_node, "position", orig, 0.2)

func _apply_hp(unit_id: int, delta: int) -> void:
	var node: Node2D = _units.get(unit_id)
	if node == null:
		return
	var bar := node.get_node_or_null("HP") as ProgressBar
	if bar == null:
		return
	bar.value = clampf(bar.value + delta, 0, bar.max_value)

## 数字前拼特殊效果前缀；forced 用于事件类型本身即代表效果（dot / reflect）。
func _tagged_number(value: int, flags: Dictionary, forced: Array = []) -> String:
	var tags := ""
	for k in DMG_TAGS:
		var v: Variant = flags.get(k, false)
		var on := false
		match typeof(v):
			TYPE_BOOL: on = v
			TYPE_INT, TYPE_FLOAT: on = v > 0
			TYPE_STRING, TYPE_STRING_NAME: on = v != ""
		if forced.has(k):
			on = true
		if on:
			tags += str(DMG_TAGS[k])
	return (tags + " " if tags != "" else "") + str(value)

## 飘字：V1.28 加深 + 深褐描边/投影，停留时间由 0.8s 延长到约 1.8~2.0s。
## emphasis 用于暴击等（规范 §5「暴击加大加粗」），above 用于技能名浮在名牌之上。
func _float_text(node: Node2D, text: String, color: Color, above: bool = false, emphasis: bool = false) -> void:
	if node == null or not Settings.float_text:
		return
	var h: float = _unit_heights.get(int(str(node.name)), 150.0)
	var lab := Label.new()
	lab.name = "FloatText"
	lab.text = text
	lab.add_theme_font_size_override("font_size", 30 if emphasis else 22)
	lab.add_theme_color_override("font_color", color)
	# 手绘战场明暗变化大：浅米描边把字从暗处「抠」出来，深褐投影在亮天空上压出立体感。
	lab.add_theme_color_override("font_outline_color", Color("F6EFDF"))
	lab.add_theme_constant_override("outline_size", 4 if emphasis else 3)
	lab.add_theme_color_override("font_shadow_color", Color(0.15, 0.08, 0.03, 0.85))
	lab.add_theme_constant_override("shadow_offset_x", 2)
	lab.add_theme_constant_override("shadow_offset_y", 2)
	# 同单位已有飘字时错开，避免停留变长后 4v4 密集事件叠成一团。
	var live := 0
	for c in node.get_children():
		if c is Label and c.name.begins_with("FloatText"):
			live += 1
	lab.position = Vector2(-20 + (live % 2) * 16.0,
		(-h - 50.0 if above else -h * 0.55) - float(live % 3) * 24.0)
	node.add_child(lab)
	var tw := create_tween()
	tw.tween_property(lab, "position:y", lab.position.y - 46.0, 0.55)
	tw.tween_interval(0.9 if emphasis else 0.7)
	tw.tween_property(lab, "modulate:a", 0.0, 0.55)
	tw.tween_callback(lab.queue_free)

func _show_result(winner: int) -> void:
	if _result_label:
		return
	_battle_over = true
	_result_label = Label.new()
	_result_label.text = "战斗胜利" if winner == 0 else ("战斗失败" if winner == 1 else "脱离战斗")
	_result_label.add_theme_font_size_override("font_size", 36)
	_result_label.add_theme_color_override("font_color", Color("fdf6ea"))
	_result_label.reset_size()
	_result_label.position = Vector2((size.x - _result_label.size.x) / 2.0, 280)
	add_child(_result_label)
	var timer := get_tree().create_timer(2.0)
	timer.timeout.connect(func() -> void:
		visible = false
		if _result_label:
			_result_label.queue_free()
			_result_label = null
		_battle_id = ""
		battle_over.emit())

func _submit_cmd(cmd: String, skill_id: String) -> void:
	_pending_cmd.clear()
	_render_targets()
	var payload := {"cmd": cmd, "skill_id": skill_id}
	if _target_id != 0:
		payload["target_id"] = _target_id
	_last_cmd_payload = payload.duplicate()
	var resp: Dictionary = await Network.send_and_wait(4, Protocol.SUB_BATTLE_CMD, payload)
	if int(resp.get("code", -1)) != 0:
		_reset_cmd()
