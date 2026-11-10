extends Node
## 设置面板无头冒烟：控件/置灰位/音频/画质/帧率真实生效/持久化往返/提醒与飘字门禁/退出登录兜底。
## 运行：Godot --headless --path client res://tests/settings_smoke.tscn --quit-after 900

var _panel: Control
var _frames := 0

func _ready() -> void:
	_panel = preload("res://scripts/ui/settings_panel.gd").new()
	add_child(_panel)

func _process(_delta: float) -> void:
	_frames += 1
	if _frames < 3:
		return
	set_process(false)
	_run()

func _fail(msg: String) -> void:
	printerr("[settings-smoke] FAIL ", msg)
	get_tree().quit(1)

func _run() -> void:
	_panel.open()
	await get_tree().process_frame
	if not _panel._panel.visible:
		_fail("open() 未显示面板")
		return
	var shake := _panel.find_child("Toggle_shake", true, false) as CheckButton
	if shake == null or not shake.disabled:
		_fail("震动提示未留位置灰（震屏未接入）")
		return

	# 音频真实生效：Music 总线增益 + Master 静音。
	Settings.set_value("bgm", 42)
	var db := AudioServer.get_bus_volume_db(AudioServer.get_bus_index("Music"))
	if absf(db - linear_to_db(0.42)) > 0.01:
		_fail("背景音乐滑杆未落到 Music 总线: %f" % db)
		return
	Settings.set_value("mute", true)
	if not AudioServer.is_bus_mute(AudioServer.get_bus_index("Master")):
		_fail("一键静音未静音 Master")
		return
	Settings.set_value("mute", false)

	# 画质 → 画布过滤/像素 snap（Compatibility 无 2D MSAA）；省电/帧率上限 → Engine.max_fps。
	Settings.set_value("quality", 0)
	if get_viewport().canvas_item_default_texture_filter != Viewport.DEFAULT_CANVAS_ITEM_TEXTURE_FILTER_NEAREST \
			or not get_viewport().snap_2d_transforms_to_pixel:
		_fail("画质低未落 nearest+像素 snap")
		return
	Settings.set_value("quality", 2)
	if get_viewport().canvas_item_default_texture_filter != Viewport.DEFAULT_CANVAS_ITEM_TEXTURE_FILTER_LINEAR \
			or get_viewport().snap_2d_transforms_to_pixel:
		_fail("画质高未落 linear 过滤")
		return
	Settings.set_value("power_save", true)
	if Engine.max_fps != 30:
		_fail("省电模式未压 30 帧: %d" % Engine.max_fps)
		return
	Settings.set_value("power_save", false)
	Settings.set_value("fps_cap", 60)
	if Engine.max_fps != 60:
		_fail("帧率上限未生效: %d" % Engine.max_fps)
		return

	# 持久化往返：写盘后重新加载仍为所设值。
	Settings.set_value("bgm", 42)
	Settings.load_settings()
	if Settings.bgm != 42:
		_fail("持久化往返丢失: %d" % Settings.bgm)
		return
	Settings.set_value("bgm", 70)

	# 消息提醒门禁：关闭时 quest toast 不落。
	var qu: Control = preload("res://scripts/ui/quest_ui.gd").new()
	add_child(qu)
	Settings.set_value("notify", false)
	qu._show_toast("测试提醒")
	if qu._toast.visible:
		_fail("消息提醒关闭后 toast 仍落")
		return
	Settings.set_value("notify", true)
	qu._show_toast("测试提醒")
	if not qu._toast.visible:
		_fail("消息提醒开启后 toast 未落")
		return

	# 伤害飘字门禁：关闭时不生成飘字节点。
	var b: Node = preload("res://scripts/battle/battle.gd").new()
	add_child(b)
	var unit := Node2D.new()
	add_child(unit)
	Settings.set_value("float_text", false)
	b._float_text(unit, "12", Color.WHITE)
	if unit.get_child_count() != 0:
		_fail("伤害飘字关闭后仍生成节点")
		return
	Settings.set_value("float_text", true)
	b._float_text(unit, "12", Color.WHITE)
	if unit.get_child_count() != 1:
		_fail("伤害飘字开启后未生成节点")
		return
	Settings.set_value("vfx", false)
	b._dash_attack(unit, unit)
	Settings.set_value("vfx", true)

	# 战斗偏好存储（进战斗时由 battle.gd 消费）。
	Settings.set_value("auto_battle", true)
	Settings.set_value("speed_index", 2)
	if not Settings.auto_battle or Settings.speed_index != 2:
		_fail("战斗偏好未存储")
		return
	Settings.set_value("auto_battle", false)
	Settings.set_value("speed_index", 0)

	# 等级/经验回填 session（HUD 转真）与突破按钮门禁（M2c 切片三）。
	QuestStore._apply({"level": 2, "exp": 50, "exp_next": 283, "gate_ready": false,
		"unspent_points": 4, "wallet": QuestStore.wallet, "daily": QuestStore.daily, "quests": QuestStore.quests})
	if SessionStore.level != 2 or absf(SessionStore.exp_ratio - 50.0 / 283.0) > 0.001:
		_fail("等级/经验条未回填 session")
		return
	if not _panel._break_btn.disabled:
		_fail("非关口突破按钮应置灰")
		return
	QuestStore.gate_ready = true
	QuestStore.changed.emit()
	if _panel._break_btn.disabled:
		_fail("关口经验满突破按钮应可用")
		return
	QuestStore.gate_ready = false
	QuestStore.changed.emit()

	# 无 Main 场景时退出登录只关面板（兜底不崩）。
	_panel._logout(false)
	if _panel._panel.visible:
		_fail("退出登录兜底未关面板")
		return
	print("[settings-smoke] PASS 控件/置灰位/音频/画质/帧率/持久化/提醒门禁/飘字门禁/战斗偏好/退出兜底")
	get_tree().quit(0)