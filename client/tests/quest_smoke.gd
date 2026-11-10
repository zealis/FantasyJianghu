extends Node
## 客户端任务 UI 无头冒烟：用假数据驱动 QuestStore → 校验追踪条/任务面板/背包面板渲染。
## 运行：Godot --headless --path client res://tests/quest_smoke.tscn --quit-after 600

var _layer: Control
var _frames := 0

func _ready() -> void:
	# 造一份服务端形状的数据（字段与 11/1 回包一致）。
	QuestStore.level = 1
	QuestStore.exp = 120
	QuestStore.wallet = {"gold": 500, "xiuwei": 20, "yuanbao": 0}
	QuestStore.daily = {
		"normal": {"used": 1, "limit": 50},
		"sect": {"used": 0, "limit": 20},
		"dungeon": {"used": 0, "limit": 10},
	}
	QuestStore.quests = [
		{
			"id": "main_001", "name": "归村", "type": "main", "chapter": 1,
			"state": "active", "desc": "闭关归来，先向何伯问清村中近况。",
			"giver": {"map_id": "清河村", "npc_id": "hebo"},
			"objectives": [{"kind": "talk", "target": "hebo", "cur": 0, "need": 1}],
			"rewards": {"exp": 120, "xiuwei": 20, "gold": 500,
				"items": [{"id": "pill_xiaohuan", "name": "小还丹", "count": 2}]},
		},
		{
			"id": "main_002", "name": "试手木桩", "type": "main", "chapter": 1,
			"state": "locked", "desc": "教头要先看你手上功夫。", "reason": "前置任务未完成",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "kill", "target": "muzhuang", "cur": 0, "need": 1}],
			"rewards": {"exp": 300, "gold": 1200, "items": []},
		},
		{
			"id": "daily_n_001", "name": "村口巡逻", "type": "daily_normal",
			"state": "completable", "desc": "在清河村走上一圈。",
			"giver": {"map_id": "清河村", "npc_id": "tieshan"},
			"objectives": [{"kind": "visit", "target": "清河村", "cur": 1, "need": 1}],
			"rewards": {"exp": 10, "gold": 20, "items": []},
		},
	]
	QuestStore.bag = {
		"capacity": 10, "used": 3,
		"items": [
			{"id": "pill_xiaohuan", "name": "小还丹", "category": "丹药", "count": 3, "stack": 5,
				"quality": "凡品", "desc": "回复少量气血的入门丹药。", "use": null, "sell": 5, "locked": false},
			{"id": "exp_dan", "name": "经验丹", "category": "丹药", "count": 1, "stack": 99,
				"quality": "良品", "desc": "使用得经验 100。", "use": {"kind": "exp", "value": 100},
				"sell": 20, "locked": false},
			{"id": "robe_yunjin", "name": "云锦袍", "category": "装备", "count": 1, "stack": 1,
				"quality": "良品", "desc": "云纹织锦的长袍。", "use": null, "slot": "clothes",
				"stats": {"pdef": 12, "hp": 60}, "sell": 35, "locked": false},
		],
	}
	EquipStore.slots = {"weapon": {"id": "sword_wood", "name": "粗木剑", "quality": "良品",
		"stats": {"patk": 18}, "slot": "weapon"}}
	EquipStore.attrs = {"hp": 226, "mp": 100, "patk": 89, "matk": 71, "pdef": 50, "mdef": 60,
		"speed": 100, "hit": 10000, "dodge": 0, "crit": 500, "combo": 0, "resist": 1000}
	_layer = preload("res://scripts/ui/quest_ui.gd").new()
	add_child(_layer)
	QuestStore.changed.emit()

func _process(_delta: float) -> void:
	_frames += 1
	if _frames < 3:
		return
	set_process(false)
	_run()

func _fail(msg: String) -> void:
	printerr("[quest-smoke] FAIL ", msg)
	get_tree().quit(1)

func _run() -> void:
	# 1. 追踪条：应显示进行中主线的名字与目标进度。
	var title: String = _layer._tracker_title.text
	if not title.contains("归村") or not title.contains("进行中"):
		_fail("追踪条未显示当前主线: %s" % title)
		return
	var body: String = _layer._tracker_body.text
	if not body.contains("对话 0/1"):
		_fail("追踪条目标进度不符: %s" % body)
		return

	# 2. 任务面板：四页签 + 主线行（章节徽/前往/未开放原因）。
	_layer._panel.visible = true
	_layer._refresh_panel()
	await get_tree().process_frame
	await get_tree().process_frame
	if _layer._tab_buttons.size() != 4:
		_fail("任务页签数 = %d, 期望 4" % _layer._tab_buttons.size())
		return
	var rows: int = _layer._panel_body.get_child_count()
	if rows != 2:
		_fail("主线页签行数 = %d, 期望 2" % rows)
		return
	var texts: String = _collect_text(_layer._panel_body)
	if not texts.contains("第一章") or not texts.contains("前往"):
		_fail("主线行缺章节徽/前往按钮: %s" % texts)
		return
	if not texts.contains("前置任务未完成"):
		_fail("未开放任务缺原因文案")
		return
	# 日常页签：可交付按钮；点选追踪后行内出「追踪中」徽。
	_layer._on_tab("daily")
	await get_tree().process_frame
	if not _collect_text(_layer._panel_body).contains("交付"):
		_fail("可交付任务缺「交付」按钮")
		return
	QuestStore.tracked_override = "daily_n_001"
	_layer._refresh_panel()
	await get_tree().process_frame
	if not _collect_text(_layer._panel_body).contains("追踪中"):
		_fail("点选追踪后行内缺「追踪中」徽")
		return
	if str(QuestStore.tracked().get("id", "")) != "daily_n_001":
		_fail("追踪覆写未生效")
		return
	QuestStore.tracked_override = ""
	_layer._on_tab("side")
	await get_tree().process_frame
	if not _collect_text(_layer._panel_body).contains("暂无支线任务"):
		_fail("支线空态文案缺失")
		return
	_layer._on_tab("main")
	await get_tree().process_frame

	# 2b. 前往：追踪覆写 + 关面板 + 自动走到 giver NPC 交互距离内。
	var npc: Node2D = preload("res://scripts/game/npc.gd").new()
	npc.npc_id = "hebo"
	add_child(npc)
	npc.position.x = 900.0
	var player: Node2D = preload("res://scripts/game/player.gd").new()
	add_child(player)
	player.position.x = 300.0
	_layer._on_go(QuestStore.by_id("main_001"))
	if QuestStore.tracked_override != "main_001" or _layer._panel.visible:
		_fail("前往未追踪/未关面板")
		return
	if absf(player._target_x - 830.0) > 1.0:
		_fail("前往未自动走到 NPC 交互距离: target_x=%s" % str(player._target_x))
		return
	npc.queue_free()
	player.queue_free()
	QuestStore.tracked_override = ""

	# 3. 日常计数行。
	if not _layer._daily_label.text.contains("普通日常 1/50"):
		_fail("日常计数不符: %s" % _layer._daily_label.text)
		return

	# 4. 背包面板：格阵 + 容量 + 详情卡 + 货币栏 + 未开放位置灰。
	_layer._bag_panel.visible = true
	_layer._refresh_bag()
	await get_tree().process_frame
	await get_tree().process_frame
	if _layer._bag_grid.get_child_count() != 10:
		_fail("背包格数 = %d, 期望容量 10" % _layer._bag_grid.get_child_count())
		return
	if not _layer._bag_cap_label.text.contains("容量 3/10"):
		_fail("背包容量文案不符: %s" % _layer._bag_cap_label.text)
		return
	if not _layer._bag_gold_label.text.contains("金币：500"):
		_fail("货币栏金币不符: %s" % _layer._bag_gold_label.text)
		return
	_layer._selected_item = "pill_xiaohuan"
	_layer._refresh_bag()
	await get_tree().process_frame
	var bag_texts: String = _collect_text(_layer._bag_detail)
	if not bag_texts.contains("小还丹") or not bag_texts.contains("×3"):
		_fail("详情卡未渲染选中物品: %s" % bag_texts)
		return
	if bag_texts.contains("使用"):
		_fail("无 use 定义的物品不应渲染使用按钮")
		return
	# 整理/出售/锁定已开放：按钮可用，详情含出售价。
	for op in ["sort", "sell", "lock"]:
		if (_layer._bag_ops[op] as Button).disabled:
			_fail("%s 按钮应可用" % op)
			return
	if not bag_texts.contains("售价    5 金币"):
		_fail("详情卡缺出售价: %s" % bag_texts)
		return
	# 锁定态：格角标 + 详情提示。
	QuestStore.bag["items"][0]["locked"] = true
	_layer._refresh_bag()
	await get_tree().process_frame
	var locked_texts: String = _collect_text(_layer._bag_detail)
	if not locked_texts.contains("已锁定"):
		_fail("锁定态详情未提示: %s" % locked_texts)
		return
	QuestStore.bag["items"][0]["locked"] = false
	_layer._refresh_bag()
	await get_tree().process_frame
	# 品质色：选中格 = 朱橙；取消选中后边框 = 品质色；详情名称 = 品质色。
	var cell0 := _layer._bag_grid.get_child(0) as Panel
	if cell0.get_theme_stylebox("panel").border_color != Color("C66E34"):
		_fail("选中格边框非朱橙")
		return
	_layer._selected_item = ""
	_layer._refresh_bag()
	await get_tree().process_frame
	cell0 = _layer._bag_grid.get_child(0) as Panel
	if cell0.get_theme_stylebox("panel").border_color != Color("8E8B83"):
		_fail("凡品格边框非品质色: %s" % str(cell0.get_theme_stylebox("panel").border_color))
		return
	if cell0.get_theme_stylebox("panel").bg_color != Color("8E8B83"):
		_fail("凡品格底未按品质色填充: %s" % str(cell0.get_theme_stylebox("panel").bg_color))
		return
	var empty_cell := _layer._bag_grid.get_child(3) as Panel
	if empty_cell.get_theme_stylebox("panel").bg_color != Color("EEE4CB"):
		_fail("空格应保持纸底")
		return
	_layer._selected_item = "exp_dan"
	_layer._refresh_bag()
	await get_tree().process_frame
	var cell1 := _layer._bag_grid.get_child(1) as Panel
	if cell1.get_theme_stylebox("panel").border_color != Color("C66E34"):
		_fail("选中格边框非朱橙")
		return
	var exp_texts: String = _collect_text(_layer._bag_detail)
	if not exp_texts.contains("使用") or not exp_texts.contains("使用得经验 100。"):
		_fail("有 use 定义的物品缺使用按钮/描述: %s" % exp_texts)
		return
	var name_lab := _find_label(_layer._bag_detail, "经验丹")
	if name_lab == null or name_lab.get_theme_color("font_color") != Color("5E9C5F"):
		_fail("详情名称未按良品品质色着色")
		return
	# 装备：属性转真 / 万分制换算 / 站台槽图标 / 穿戴与卸下按钮。
	if _layer._bag_stats["patk"].text != "89":
		_fail("属性物攻未转真: %s" % _layer._bag_stats["patk"].text)
		return
	if _layer._bag_stats["crit"].text != "5.0%":
		_fail("暴击万分制未换算: %s" % _layer._bag_stats["crit"].text)
		return
	var weapon_slot: PanelContainer = _layer._slot_panels["weapon"]
	if weapon_slot.get_child_count() == 0:
		_fail("武器槽未渲染装备图标")
		return
	if weapon_slot.get_theme_stylebox("panel").bg_color != Color("5E9C5F"):
		_fail("已穿戴槽底色未按品质色（与背包格一致）")
		return
	var clothes_slot: PanelContainer = _layer._slot_panels["clothes"]
	if clothes_slot.get_theme_stylebox("panel").bg_color != Color("EEE4CB"):
		_fail("空槽应保持纸底")
		return
	_layer._selected_item = "sword_wood"
	_layer._refresh_bag()
	await get_tree().process_frame
	if not _collect_text(_layer._bag_detail).contains("卸下"):
		_fail("已穿戴装备详情缺卸下按钮")
		return
	_layer._selected_item = "robe_yunjin"
	_layer._refresh_bag()
	await get_tree().process_frame
	if not _collect_text(_layer._bag_detail).contains("穿戴"):
		_fail("背包装备详情缺穿戴按钮")
		return
	_layer._selected_item = "pill_xiaohuan"
	_layer._refresh_bag()
	await get_tree().process_frame
	if not _placeholders_disabled(_layer._bag_panel):
		_fail("仓库按钮未置灰（服务端未开放）")
		return
	# 站台皮肤跟随会话回填（面板构建早于登录回包）。
	SessionStore.faction = "断岳剑宗"
	SessionStore.gender = "男"
	SessionStore.notify_changed()
	await get_tree().process_frame
	if not _layer._bag_figure.texture.resource_path.contains("断岳剑宗男"):
		_fail("站台皮肤未跟随门派/性别: %s" % _layer._bag_figure.texture.resource_path)
		return

	# 5. NPC 对话注入：铁山身上有可交付日常；未开放的主线不得出现。
	var for_tieshan: Array = QuestStore.quests_for_npc("tieshan")
	if for_tieshan.is_empty():
		_fail("NPC 任务注入为空（11/1 数据未参与过滤）")
		return
	if for_tieshan.size() != 1 or str(for_tieshan[0].get("id", "")) != "daily_n_001":
		_fail("NPC 任务注入未按状态过滤: %s" % str(for_tieshan))
		return

	# 6. 奖励提示。
	_layer._on_reward({"exp": 120, "xiuwei": 20, "gold": 500,
		"items": [{"id": "pill_xiaohuan", "name": "小还丹", "count": 2}]})
	if not _layer._toast.text.contains("小还丹 ×2"):
		_fail("奖励提示未包含物品: %s" % _layer._toast.text)
		return

	print("[quest-smoke] PASS 追踪条 / 四页签 / 追踪中 / 日常计数 / 背包格阵与详情 / 品质色 / 装备槽与穿卸 / 置灰位 / NPC 注入 / 奖励提示")
	get_tree().quit(0)

## 未开放能力（仓库/整理/出售/锁定）必须置灰，承战斗轮「不做假功能」口径。
## 仅仓库等服务端未开放的能力保持置灰；整理/出售/锁定已接入 5/3~5/5。
func _placeholders_disabled(root: Node) -> bool:
	var ok := true
	for c in root.get_children():
		if c is Button:
			var t := (c as Button).text
			if t.begins_with("仓库"):
				ok = ok and (c as Button).disabled
		ok = _placeholders_disabled(c) and ok
	return ok

## 递归收集控件文本（按钮/标签），用于断言渲染内容。
func _collect_text(root: Node) -> String:
	var out := ""
	for c in root.get_children():
		if c is Label:
			out += (c as Label).text + "\n"
		elif c is Button:
			out += (c as Button).text + "\n"
		out += _collect_text(c)
	return out

func _find_label(root: Node, text: String) -> Label:
	for c in root.get_children():
		if c is Label and (c as Label).text == text:
			return c as Label
		var found := _find_label(c, text)
		if found != null:
			return found
	return null
