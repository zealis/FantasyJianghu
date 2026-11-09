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

	# --- V1.28 飘字：特殊效果前缀 + 深褐描边（合成事件，不伪造服务端数据） ---
	layer._play_event({"type": "damage", "actor": 1, "target": 2, "value": 37,
		"flags": {"crit": true, "combo": true, "lifesteal": 12, "magic": true}})
	var ftxt := ""
	var outline := 0
	for c in layer._units[2].get_children():
		if c is Label and c.name.begins_with("FloatText"):
			ftxt = c.text
			outline = c.get_theme_constant("outline_size")
	if not ftxt.begins_with("爆连吸法"):
		_fail("特殊效果前缀未生效: %s" % ftxt)
		return
	if outline < 3:
		_fail("飘字描边缺失: %d" % outline)
		return

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

	print("[battle-smoke] PASS 单位生成 / 15 类事件播放 / 指令台复位 / 补发去重 / 终局回世界")
	get_tree().quit(0)

func _fail(msg: String) -> void:
	push_error("[battle-smoke] FAIL: " + msg)
	get_tree().quit(1)
