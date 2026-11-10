extends Node
## 主流程：登录面板 → 进场快照 → 显现世界/HUD（前端规范 §3.1 场景组织）。

func _ready() -> void:
	# 主世界已切到 2.5D 俯视扩张版：对话/任务/热区数据改读 v8 逻辑文件。
	# 横版 qinghecun.tscn 与其逻辑数据保留存档，改回 World 的 ext_resource 即可回退。
	SceneLogic.current_path = "res://data/qinghecun_v8_logic.json"
	$World.visible = false
	$UILayer.visible = false
	$DialogueLayer.visible = false
	WorldStore.snapshot_applied.connect(_on_snapshot_applied)
	Network.disconnected.connect(_on_disconnected)
	$BattleCanvas/BattleLayer.battle_over.connect(_on_battle_over)
	$DialogueLayer.get_node("DialoguePanel").dialogue_action.connect(_on_dialogue_action)

func _on_snapshot_applied() -> void:
	$LoginLayer.visible = false
	$World.visible = true
	$UILayer.visible = true
	$DialogueLayer.visible = true
	# 进场/战斗回世界后拉任务与背包全量（服务端权威，登录恢复也走这条）。
	QuestStore.refresh()
	EquipStore.refresh()

## 断线/被踢：回登录层（登录页负责展示原因与重登入口，M10）。
func _on_disconnected(_msg: String) -> void:
	$LoginLayer.visible = true
	$World.visible = false
	$UILayer.visible = false
	$DialogueLayer.visible = false
	# 清理世界状态，避免重登后残留旧远端玩家与旧任务快照。
	WorldStore.remote_players.clear()
	QuestStore.quests = []
	QuestStore.loaded = false
	QuestStore.changed.emit()

## 设置底栏：切换账号（保留账号名）/退出登录（连账号名一起清）回登录层。
func return_to_login(clear_account: bool) -> void:
	Network.disconnect_ws()
	_on_disconnected("")
	$BattleCanvas/BattleLayer.visible = false
	SessionStore.token = ""
	var login := $LoginLayer.get_node_or_null("Login")
	if login and login.has_method("reset_for_logout"):
		login.call("reset_for_logout", clear_account)

func _on_dialogue_action(action: String) -> void:
	print("[main] dialogue action: ", action)
	if action.begins_with("battle:"):
		var arg := action.trim_prefix("battle:")
		# battle:drill 走 M2a-2 4v4 演练，其余为 1v1 切磋（monster_id）。
		if arg == "drill":
			enter_battle({"mode": "drill"})
		else:
			enter_battle({"monster_id": arg})

## 战斗入口（NPC 对话）：4/1 进战斗，参数由对话 action 数据驱动。
func enter_battle(params: Dictionary) -> void:
	print("[main] enter_battle: ", params)
	var resp: Dictionary = await Network.send_and_wait(4, Protocol.SUB_BATTLE_ENTER, params)
	print("[main] enter_battle resp: ", resp)
	if int(resp.get("code", -1)) != 0 or not resp.has("battle_id"):
		# 旧实现静默 return，玩家只看到「点了没反应」；改为给出可见反馈。
		_notify(str(resp.get("msg", "进入战斗失败，请稍后再试")))
		return
	$World.visible = false
	$UILayer.visible = false
	$DialogueLayer.visible = false
	$BattleCanvas/BattleLayer.visible = true
	$BattleCanvas/BattleLayer.enter_battle(resp)

func _notify(msg: String) -> void:
	var hud := $UILayer.get_node_or_null("HUD")
	if hud and hud.has_method("_toast"):
		hud.call("_toast", msg)
	else:
		push_warning("[main] " + msg)

## 战斗终局：回主世界并重新拉场景快照（刷新位置/分线/远端玩家）。
func _on_battle_over() -> void:
	$BattleCanvas/BattleLayer.visible = false
	$World.visible = true
	$UILayer.visible = true
	$DialogueLayer.visible = true
	var resp: Dictionary = await Network.send_and_wait(3, Protocol.SUB_ENTER_SCENE, {"map_id": WorldStore.current_map})
	if int(resp.get("code", -1)) == 0:
		WorldStore.apply_snapshot(resp)
