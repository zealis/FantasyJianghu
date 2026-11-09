extends Node
## 状态层 · session 域（前端规范 §3.3）。
## 只存服务端下发的权威状态与本地 UI 状态，不在前端推演资产变化。
## 当前为离线骨架占位：初始值仅用于表现，资产操作待服务端回包。

signal session_changed

# 联机会话（服务端下发）。
var token: String = ""
var account_id: int = 0
var chars: Array = []  # 服务端 2/1 回包的角色列表
var factions: Array = []  # 八派明细（2/1 下发，宗门卡片来源）

var player_name: String = "侠客"
var level: int = 1
var faction: String = "惊雷殿"
var gender: String = "男"
var title: String = ""
var hp_ratio: float = 1.0
var mp_ratio: float = 1.0
var exp_ratio: float = 0.0
var vip: int = 0

func display_name() -> String:
	if title.is_empty():
		return player_name
	return "【%s】%s" % [title, player_name]

## 本地回填字段后统一通知（HUD 刷新）。
func notify_changed() -> void:
	session_changed.emit()

func apply_server_state(state: Dictionary) -> void:
	# 权威状态来自服务端回包，客户端不自行结算。
	player_name = state.get("name", player_name)
	level = state.get("level", level)
	faction = state.get("faction", faction)
	hp_ratio = state.get("hp_ratio", hp_ratio)
	mp_ratio = state.get("mp_ratio", mp_ratio)
	exp_ratio = state.get("exp_ratio", exp_ratio)
	session_changed.emit()