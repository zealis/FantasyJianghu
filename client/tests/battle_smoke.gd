extends Node
## 客户端战斗 UI 无头冒烟：覆盖「进战斗 → 帧事件播放 → 指令台复位 → 终局回世界」。
## 不依赖桌面输入注入（CUA 窗口激活在部分环境不可用），可直接 CI 跑。
##
## 运行：
##   Godot_v4.7.2-stable_win64_console.exe --headless --path client \
##       res://tests/battle_smoke.tscn --quit-after 1800
## 退出码 0=通过，1=失败。

const EVENT_INTERVAL := 0.45

func _ready() -> void:
	await get_tree().process_frame
	var layer := preload("res://scenes/battle/battle.tscn").instantiate()
	add_child(layer)

	var snapshot := {
		"battle_id": "smoke-1",
		"turn_version": 0,
		"self": {"id": 1, "name": "冒烟侠", "side": 0, "hp": 100, "max_hp": 100, "mp": 50, "max_mp": 50},
		"enemy": {"id": 2, "name": "木桩人", "side": 1, "hp": 80, "max_hp": 80, "mp": 0, "max_mp": 0},
		"skills": [{"id": "tianlei_po", "name": "天雷破", "category": "单体", "mp_cost": 150, "level_req": 10}],
	}
	layer.enter_battle(snapshot)
	# 布局回归：战斗层必须铺满视口，否则单位站位落在可视区外（实机只显示左上角一小块）。
	var vp: Vector2 = get_viewport().get_visible_rect().size
	print("[battle-smoke] viewport=%s battle.size=%s" % [vp, layer.size])
	if layer.size.x < vp.x - 1.0 or layer.size.y < vp.y - 1.0:
		_fail("战斗层未铺满视口: battle.size=%s viewport=%s" % [layer.size, vp])
		return
	# 复现实机：窗口被缩放后必须仍然铺满（父是普通 Node，锚点不自动传播）。
	var win := get_window()
	win.size = Vector2i(1056, 648)
	await get_tree().process_frame
	await get_tree().process_frame
	var vp_after: Vector2 = get_viewport().get_visible_rect().size
	print("[battle-smoke] after resize viewport=%s battle.size=%s" % [vp_after, layer.size])
	if absf(layer.size.x - vp_after.x) > 1.0 or absf(layer.size.y - vp_after.y) > 1.0:
		_fail("窗口缩放后战斗层未跟随视口: battle.size=%s viewport=%s" % [layer.size, vp_after])
		return
	if layer._units.size() != 2:
		_fail("单位生成数 = %d，期望 2" % layer._units.size())
		return
	# 1v1：双方同一水平线（用户定案，不再斜对角）。
	var y_self: float = float(layer._units[1].position.y)
	var y_enemy: float = float(layer._units[2].position.y)
	if absf(y_self - y_enemy) > 0.5:
		_fail("1v1 双方未对齐: self.y=%f enemy.y=%f" % [y_self, y_enemy])
		return
	# 用户定案：玩家自己在右、敌方/怪物/NPC 在左，且我方立翻面朝左对视。
	if layer._units[1].position.x <= layer._units[2].position.x:
		_fail("1v1 我方应在右、敌方在左")
		return
	var body_1v1 := layer._units[1].get_node_or_null("Body") as Sprite2D
	if body_1v1 == null or not body_1v1.flip_h:
		_fail("我方站右侧应翻面朝左（flip_h）")
		return
	if not layer.visible or layer._cmd_panel._submitted:
		_fail("进战斗后战斗层/指令台状态不正确")
		return

	# 覆盖全部已注册事件分支（含 M2a-2 预留的 dot/reflect），确保播放器不抛运行时错误。
	var events := [
		{"type": "act", "actor": 1, "target": 2, "flags": {"skill": "天雷破"}},
		{"type": "damage", "actor": 1, "target": 2, "value": 37, "flags": {"crit": true}},
		{"type": "miss", "actor": 2, "target": 1},
		{"type": "heal", "actor": 2, "target": 2, "value": 5},
		{"type": "control", "actor": 1, "target": 2, "flags": {"type": "麻痹"}},
		{"type": "control_fail", "actor": 1, "target": 2},
		{"type": "fallback", "actor": 2, "flags": {"reason": "hosted"}},
		{"type": "defend", "actor": 2},
		{"type": "buff", "actor": 1, "target": 1, "flags": {"stat": "mdef_pct"}},
		{"type": "stunned", "actor": 2},
		{"type": "flee_fail", "actor": 2},
		{"type": "dot", "actor": 1, "target": 2, "value": 5},
		{"type": "reflect", "actor": 2, "target": 1, "value": 3},
		{"type": "death", "target": 2},
		{"type": "turn_end", "flags": {"turn": 1, "next_turn": 2}},
	]
	layer._on_packet(4, Protocol.SUB_BATTLE_FRAME, {"turn_version": 1, "events": events})
	# 播完后指令台必须重新开放（旧实现第一回合后永久锁死）。
	await get_tree().create_timer(EVENT_INTERVAL * (events.size() + 2)).timeout
	if layer._queue.size() != 0 or layer._playing:
		_fail("事件队列未播完: queue=%d playing=%s" % [layer._queue.size(), layer._playing])
		return
	if layer._cmd_panel._submitted:
		_fail("新回合指令台未复位")
		return

	# 补发帧去重：已确认回合 1 的旧帧不得再次入队。
	layer._on_packet(4, Protocol.SUB_BATTLE_FRAME, {"turn_version": 1, "events": events, "resend": true})
	if layer._queue.size() != 0:
		_fail("补发帧未按已确认回合去重")
		return

	# --- V1.35 飘字规范（UI 规范 §5 ①②③④）: 符号 / 归属 / 字号 / 延迟 ---
	# 先清掉前面事件分支留下的飘字，避免误判归属。
	_clear_floats(layer._units[1])
	_clear_floats(layer._units[2])

	Settings.vfx = true
	layer._play_event({"type": "damage", "actor": 1, "target": 2, "value": 37,
		"flags": {"crit": true, "combo": true, "lifesteal": 12, "magic": true}})
	# ④ 出现时机：攻击方尚未回位时不得出字。
	if _find_float(layer._units[2]) != null:
		_fail("飘字在回位前就出现了（应等位移走完再出字）")
		return
	await get_tree().create_timer(_dash_wait(layer) + 0.15).timeout
	var num := _find_float(layer._units[2])
	if num == null:
		_fail("回位后未出飘字")
		return
	# ① 正负号：扣血带真减号 U+2212，符号与数字在同一 Label（同色同字号、不可省略）。
	if not num.text.begins_with("−37"):
		_fail("扣血飘字未带负号: %s" % num.text)
		return
	var tag_lab := _find_tag(layer._units[2])
	var tag_txt: String = tag_lab.text if tag_lab != null else ""
	if not (tag_txt.contains("暴击") and tag_txt.contains("吸") and tag_txt.contains("法")):
		_fail("暴击/效果标签缺失: %s" % tag_txt)
		return
	# ② 归属：只在掉血的单位身上出字，攻击方身上不许出。
	if _find_float(layer._units[1]) != null:
		_fail("飘字落到了攻击方身上（应只出在数值实际变化的单位）")
		return
	# ③ 字号：与人物身高挂钩（V1.41 补记：约 0.22 倍字高）。
	var fsz := num.get_theme_font_size("font_size")
	var unit_h: float = float(layer._unit_heights[2])
	var glyph := float(fsz) * 0.72
	if glyph < unit_h * 0.20 or glyph > unit_h * 0.38:
		_fail("飘字字号越界: font_size=%d 字高≈%.0f 身高=%.0f" % [fsz, glyph, unit_h])
		return
	if glyph < 30.0:
		_fail("飘字仍是小字: 字高≈%.0f" % glyph)
		return
	# V1.41 定版色值：暴击与物伤同色 #A62B1A，直接落色、不得二次加深。
	var got_color := num.get_theme_color("font_color")
	if got_color != Color("A62B1A"):
		_fail("暴击飘字未用定版色值 A62B1A: %s" % got_color)
		return
	# 暴击「加大加粗」：字体须为 FontVariation 且合成加粗量 > 0。
	var crit_font := num.get_theme_font("font")
	if not (crit_font is FontVariation) or float((crit_font as FontVariation).variation_embolden) <= 0.0:
		_fail("暴击飘字未加粗")
		return
	# 增血出「+N」：治疗/吸血回复落在被回复的单位身上。
	_clear_floats(layer._units[1])
	layer._play_event({"type": "heal", "actor": 1, "target": 1, "value": 12})
	await get_tree().create_timer(_dash_wait(layer) + 0.15).timeout
	var heal_lab := _find_float(layer._units[1])
	if heal_lab == null or not heal_lab.text.begins_with("+12"):
		_fail("增血飘字未带正号: %s" % (heal_lab.text if heal_lab != null else "<无>"))
		return
	# 普通物伤同样按定版色值落色（不受暴击加粗分支影响）。
	_clear_floats(layer._units[2])
	layer._play_event({"type": "damage", "actor": 1, "target": 2, "value": 9})
	await get_tree().create_timer(_dash_wait(layer) + 0.15).timeout
	var normal_lab := _find_float(layer._units[2])
	if normal_lab == null:
		_fail("普通伤害未出飘字")
		return
	if normal_lab.get_theme_color("font_color") != Color("A62B1A"):
		_fail("普通伤害未用定版色值 A62B1A: %s" % normal_lab.get_theme_color("font_color"))
		return
	_clear_floats(layer._units[1])
	_clear_floats(layer._units[2])

	# 终局 4/5 → 结果横幅 → 战斗层隐藏（main.gd 收到 battle_over 后回主世界）。
	layer._on_packet(4, Protocol.SUB_BATTLE_RESULT, {"winner": 0, "turns": 1, "battle_id": "smoke-1"})
	await get_tree().create_timer(2.5).timeout
	if layer.visible:
		_fail("终局后战斗层未隐藏")
		return
	# --- M2a-2：4v4 站位 / 行动顺序条 / 状态角标 / 默认目标 ---
	var drill := {
		"battle_id": "smoke-4v4", "turn_version": 0, "mode": "drill",
		"units": [],
		"skills": [{"id": "tianlei_po", "name": "天雷破", "category": "单体", "mp_cost": 150, "level_req": 10}],
	}
	for i in range(4):
		drill["units"].append({"id": 100 + i, "name": "我方%d" % (i + 1), "side": 0, "slot": i,
			"hp": 900, "max_hp": 900, "mp": 200, "max_mp": 200, "reflect_pct": 0})
	for i in range(4):
		drill["units"].append({"id": 200 + i, "name": "敌方%d" % (i + 1), "side": 1, "slot": i,
			"hp": 700, "max_hp": 700, "mp": 0, "max_mp": 0, "reflect_pct": 900 if i == 3 else 0})
	layer.enter_battle(drill)
	if layer._units.size() != 8:
		_fail("4v4 应生成 8 个单位, 实得 %d" % layer._units.size())
		return
	if layer._target_id == 0:
		_fail("4v4 应默认锁定首个敌方目标")
		return
	if layer._event_interval >= 0.3:
		_fail("4v4 应加快事件播放间隔: %f" % layer._event_interval)
		return
	# 多人：双方同行同 y，且己方两行围绕战场中心上下对称。
	var self_ys: Array = []
	var enemy_ys: Array = []
	for id in layer._units:
		var py: float = float(layer._units[id].position.y)
		if int(layer._unit_sides.get(id, 0)) == 0:
			self_ys.append(py)
		else:
			enemy_ys.append(py)
	self_ys.sort()
	enemy_ys.sort()
	if self_ys.size() != 4 or enemy_ys.size() != 4:
		_fail("4v4 阵营人数统计异常: %d / %d" % [self_ys.size(), enemy_ys.size()])
		return
	for i in range(4):
		if absf(float(self_ys[i]) - float(enemy_ys[i])) > 0.5:
			_fail("多人战双方同行未对齐: %s / %s" % [str(self_ys), str(enemy_ys)])
			return
	if absf((float(self_ys[0]) + float(self_ys[3])) * 0.5 - 420.0) > 0.5:
		_fail("多人战未围绕战场中心上下居中: %s" % str(self_ys))
		return
	# 同步触发 order（不经过队列），验证顺序条数据。
	layer._play_event({"type": "order", "flags": {
		"ids": [100, 101, 102, 103, 200, 201, 202, 203],
		"names": ["我方1", "我方2", "我方3", "我方4", "敌方1", "敌方2", "敌方3", "敌方4"],
		"slots": [0, 1, 2, 3, 0, 1, 2, 3]}})
	if layer._order_names.size() != 8:
		_fail("行动顺序条应含 8 个名字, 实得 %d" % layer._order_names.size())
		return
	layer._play_event({"type": "shock", "actor": 100, "target": 203, "flags": {"stacks": 2, "max": 3}})
	var state := layer._units[203].get_node_or_null("State") as Label
	if state == null or not state.text.contains("感电"):
		_fail("感电状态角标未更新: %s" % (state.text if state else "null"))
		return
	# --- 两段式出招：点普攻/秘籍只进入待选目标，点敌方单位才发动 ---
	if not layer._cmd_panel.requires_target:
		_fail("4v4 应启用两段式目标选择")
		return
	layer._cmd_panel._submit("attack", "")
	if not layer._cmd_panel.is_pending() or layer._cmd_panel._submitted:
		_fail("点普攻后应进入待选目标且尚未发动")
		return
	if layer._pending_cmd.get("cmd", "") != "attack":
		_fail("待选指令未记录: %s" % str(layer._pending_cmd))
		return
	if layer._cmd_panel._hint_label.text != "请选择攻击目标":
		_fail("待选目标提示未显示: %s" % layer._cmd_panel._hint_label.text)
		return
	var lit := 0
	for id in layer._units:
		if int(layer._unit_sides.get(id, 0)) == 1:
			var r: Node = layer._units[id].get_node_or_null("TargetRing")
			if r is CanvasItem and (r as CanvasItem).visible:
				lit += 1
	if lit != 4:
		_fail("待选目标时应 4 个敌方光圈亮起, 实得 %d" % lit)
		return
	layer._clear_pending()
	if not layer._pending_cmd.is_empty() or layer._cmd_panel.is_pending():
		_fail("取消后待选状态未清空")
		return
	layer._cmd_panel._submit("skill", "s_lei")
	layer._set_target(203)
	if not layer._pending_cmd.is_empty():
		_fail("点目标后待选指令未消费")
		return
	if int(layer._last_cmd_payload.get("target_id", 0)) != 203:
		_fail("点目标后未带 target_id 发动: %s" % str(layer._last_cmd_payload))
		return
	if str(layer._last_cmd_payload.get("skill_id", "")) != "s_lei":
		_fail("点目标后秘籍 id 丢失")
		return
	layer._reset_cmd()
	print("[battle-smoke] 4v4 PASS 两段式出招（选招→选目标→发动 / Esc 取消）")
	# --- 战斗背景按地点切换（映射表驱动） ---
	var bg_q := str(layer._battle_bg_path("清河村", ""))
	var bg_alias := str(layer._battle_bg_path("青禾村", ""))
	var bg_lin := str(layer._battle_bg_path("临渊镇", ""))
	var bg_drill := str(layer._battle_bg_path("清河村", "drill"))
	var bg_unknown := str(layer._battle_bg_path("未登记地点", ""))
	if not bg_q.ends_with("bg_qinghecun.jpg") or bg_alias != bg_q:
		_fail("地点/别名背景映射错误: %s / %s" % [bg_q, bg_alias])
		return
	if not bg_lin.ends_with("bg_linyuanzhen.jpg"):
		_fail("临渊镇背景映射错误: %s" % bg_lin)
		return
	if not bg_drill.ends_with("bg_zangjianyuan.jpg"):
		_fail("演练场背景映射错误: %s" % bg_drill)
		return
	if not bg_unknown.ends_with("bg_common_wild.jpg"):
		_fail("未知地点未回退通用野外: %s" % bg_unknown)
		return
	# 多人战：我方阵营整体在右、敌方阵营整体在左。
	var self_min_x := 100000.0
	var enemy_max_x := -100000.0
	for id in layer._units:
		var ux: float = float(layer._units[id].position.x)
		if int(layer._unit_sides.get(id, 0)) == 0:
			self_min_x = minf(self_min_x, ux)
		else:
			enemy_max_x = maxf(enemy_max_x, ux)
	if self_min_x <= enemy_max_x:
		_fail("4v4 我方阵营应在右、敌方在左: %f / %f" % [self_min_x, enemy_max_x])
		return
	# 倍速：缩短事件播放间隔并更新按钮文字。
	var base_interval: float = float(layer._event_interval)
	layer._on_speed_pressed()
	if layer._event_interval >= base_interval or layer._speed_btn.text != "倍速 ×2":
		_fail("倍速开关未生效: %f / %s" % [layer._event_interval, layer._speed_btn.text])
		return
	# 自动：开启后指令台进入托管倒计时，再关闭还原。
	layer._on_auto_pressed()
	if not bool(layer._cmd_panel.auto_play) or layer._auto_btn.text != "自动·开":
		_fail("自动开关未生效")
		return
	layer._on_auto_pressed()
	if bool(layer._cmd_panel.auto_play):
		_fail("自动开关无法关闭")
		return
	print("[battle-smoke] 4v4 PASS 站位8单位 / 顺序条 / 感电角标 / 目标锁定")

	# --- 怪物立绘朝向：蜃气妖花（荆棘武士）是侧向构图，必须镜像；正面构图怪物保持不翻 ---
	var face_cases := [
		{"name": "荆棘武士", "want": true},
		{"name": "木桩人", "want": false},
	]
	for fc in face_cases:
		layer.enter_battle({
			"battle_id": "smoke-face", "turn_version": 0,
			"units": [
				{"id": 1, "name": "冒烟侠", "side": 0, "slot": 0, "hp": 100, "max_hp": 100, "mp": 50, "max_mp": 50},
				{"id": 2, "name": str(fc["name"]), "side": 1, "slot": 0, "hp": 100, "max_hp": 100, "mp": 0, "max_mp": 0},
			],
			"skills": [],
		})
		var body := layer._units[2].get_node_or_null("Body") as Sprite2D
		if body == null:
			_fail("%s 未加载怪物立绘" % fc["name"])
			return
		if body.flip_h != bool(fc["want"]):
			_fail("%s 朝向镜像不符: flip_h=%s 期望=%s" % [fc["name"], body.flip_h, fc["want"]])
			return
	print("[battle-smoke] PASS 蜃气妖花镜像 / 正面构图怪物不翻")

	# --- V1.39/V1.40：双方各自按入场速度降序填 2×2 方阵 ---
	var f_units: Array = []
	var self_spd := [10, 40, 20, 30]
	var enemy_spd := [50, 5, 35, 15]
	for i in range(4):
		f_units.append({"id": 100 + i, "name": "A%d" % i, "side": 0, "slot": i,
			"hp": 100, "max_hp": 100, "mp": 0, "max_mp": 0, "speed": self_spd[i]})
	for i in range(4):
		f_units.append({"id": 200 + i, "name": "B%d" % i, "side": 1, "slot": i,
			"hp": 100, "max_hp": 100, "mp": 0, "max_mp": 0, "speed": enemy_spd[i]})
	layer.enter_battle({"battle_id": "smoke-form", "turn_version": 0, "mode": "drill",
		"units": f_units, "skills": []})
	# 1 号位上排左 / 2 号位上排右 / 3 号位下排左 / 4 号位下排右（左右按屏幕视角）。
	var expect_form := {
		101: Vector2(865, 320), 103: Vector2(1030, 320),
		102: Vector2(865, 520), 100: Vector2(1030, 520),
		200: Vector2(180, 320), 202: Vector2(330, 320),
		203: Vector2(180, 520), 201: Vector2(330, 520),
	}
	for uid in expect_form:
		var got_pos: Vector2 = layer._units[uid].position
		if got_pos.distance_to(expect_form[uid]) > 0.5:
			_fail("速度布位错误 uid=%d got=%s want=%s" % [uid, str(got_pos), str(expect_form[uid])])
			return
	# 出手事件只做临时冲刺，播完回原位；布位缓存不得被改写。
	layer._play_event({"type": "damage", "actor": 100, "target": 200, "value": 7})
	await get_tree().create_timer(0.7).timeout
	for uid2 in expect_form:
		if layer._units[uid2].position.distance_to(expect_form[uid2]) > 0.5:
			_fail("战斗中布位被改动 uid=%d" % uid2)
			return
	# 同速时按入场顺序（units[] 顺序）稳定填位。
	var tie_units: Array = []
	for i in range(4):
		tie_units.append({"id": 300 + i, "name": "T%d" % i, "side": 0, "slot": i,
			"hp": 100, "max_hp": 100, "mp": 0, "max_mp": 0, "speed": 100})
	for i in range(4):
		tie_units.append({"id": 400 + i, "name": "U%d" % i, "side": 1, "slot": i,
			"hp": 100, "max_hp": 100, "mp": 0, "max_mp": 0, "speed": 100})
	layer.enter_battle({"battle_id": "smoke-tie", "turn_version": 0, "mode": "drill",
		"units": tie_units, "skills": []})
	var tie_expect := [Vector2(865, 320), Vector2(1030, 320), Vector2(865, 520), Vector2(1030, 520)]
	for i in range(4):
		if layer._units[300 + i].position.distance_to(tie_expect[i]) > 0.5:
			_fail("同速未按入场顺序布位 i=%d" % i)
			return
	print("[battle-smoke] PASS 速度 2×2 方阵 / 同速兜底 / 战斗中不变位")

	print("[battle-smoke] PASS 单位生成 / 15 类事件播放 / 指令台复位 / 补发去重 / 终局回世界")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[battle-smoke] FAIL: " + msg)
	get_tree().quit(1)

# --- 飘字检查辅助（V1.35 §5）：飘字挂在单位节点下的 Node2D 容器里，数字与标签是子 Label ---
func _float_holders(unit: Node) -> Array:
	var out := []
	for c in unit.get_children():
		if c is Node2D and str(c.name).begins_with("FloatText"):
			out.append(c)
	return out

func _find_float(unit: Node) -> Label:
	for holder in _float_holders(unit):
		for l in holder.get_children():
			if l is Label:
				return l
	return null

func _find_tag(unit: Node) -> Label:
	for holder in _float_holders(unit):
		var seen := 0
		for l in holder.get_children():
			if l is Label:
				seen += 1
				if seen == 2:
					return l
	return null

func _clear_floats(unit: Node) -> void:
	for holder in _float_holders(unit):
		holder.queue_free()

## 攻击方位移未回位前的剩余等待（秒）：飘字必须压到这段时间之后。
func _dash_wait(layer: Node) -> float:
	return maxf(0.0, float(int(layer._dash_busy_until) - Time.get_ticks_msec()) / 1000.0)
