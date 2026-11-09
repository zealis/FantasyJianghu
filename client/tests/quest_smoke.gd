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
		"capacity": 10, "used": 1,
		"items": [{"id": "pill_xiaohuan", "name": "小还丹", "category": "丹药", "count": 3, "stack": 5}],
	}
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

	# 2. 任务面板：主线/日常分组 + 可接取/可交付按钮。
	_layer._panel.visible = true
	_layer._refresh_panel()
	await get_tree().process_frame
	await get_tree().process_frame
	var rows: int = _layer._panel_body.get_child_count()
	if rows < 5:  # 主线标题 + 2 条主线 + 日常标题 + 1 条日常
		_fail("任务面板行数 = %d, 期望 >= 5" % rows)
		return
	var texts: String = _collect_text(_layer._panel_body)
	if not texts.contains("交付"):
		_fail("可交付任务缺「交付」按钮")
		return
	if not texts.contains("前置任务未完成"):
		_fail("未开放任务缺原因文案")
		return

	# 3. 日常计数行。
	if not _layer._daily_label.text.contains("普通日常 1/50"):
		_fail("日常计数不符: %s" % _layer._daily_label.text)
		return

	# 4. 背包面板。
	_layer._bag_panel.visible = true
	_layer._refresh_bag()
	await get_tree().process_frame
	await get_tree().process_frame
	var bag_texts: String = _collect_text(_layer._bag_body)
	if not bag_texts.contains("小还丹") or not bag_texts.contains("×3"):
		_fail("背包未渲染物品: %s" % bag_texts)
		return
	if not bag_texts.contains("格子 1/10"):
		_fail("背包格数文案不符: %s" % bag_texts)
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

	print("[quest-smoke] PASS 追踪条 / 任务面板 / 日常计数 / 背包 / NPC 注入 / 奖励提示")
	get_tree().quit(0)

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
