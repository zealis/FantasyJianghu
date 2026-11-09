class_name Protocol
## 协议主码登记制（开发技术规范 §5.1）。新增系统先登记后开发。
## 帧结构：主码 uint16 + 子码 uint16 + 序号 uint32 + 负载。
## 本表为客户端侧登记副本，与服务端同源生成（packages/protocol）。

const MAIN_CODES := {
	1: "系统/心跳/踢下线",
	2: "账号/角色",
	3: "世界/地图/移动",
	4: "战斗",
	5: "背包/物品",
	6: "装备/强化",
	7: "法宝",
	8: "命魂/神魂",
	9: "队伍",
	10: "NPC/商店",
	11: "任务",
	12: "活动",
	13: "宗派",
	14: "寄售行",
	15: "邮件",
	16: "聊天",
	17: "排行榜/竞技",
	18: "商城/VIP",
	19: "坐骑",
	20: "抽奖",
}

const HEARTBEAT_SECONDS := 15.0

## 子码常量（登记单源 sub_codes，与服务端同源生成）。
## 主码 1 系统/心跳/踢下线
const SUB_KICK := 1
const SUB_PING := 0
## 主码 2 账号/角色
const SUB_AUTH := 1
const SUB_CHAR_CREATE := 2
const SUB_CHAR_SELECT := 3
## 主码 3 世界/地图/移动
const SUB_ENTER_SCENE := 1
const SUB_LEAVE := 4
const SUB_LEAVE_BROADCAST := 5
const SUB_LINE_LIST := 7
const SUB_MOVE := 2
const SUB_MOVE_BROADCAST := 3
const SUB_PLAYER_JOIN := 6
const SUB_SWITCH_LINE := 8
## 主码 4 战斗
const SUB_BATTLE_ACK := 4
const SUB_BATTLE_CMD := 2
const SUB_BATTLE_ENTER := 1
const SUB_BATTLE_FRAME := 3
const SUB_BATTLE_LEAVE := 6
const SUB_BATTLE_RESTORE := 7
const SUB_BATTLE_RESULT := 5
## 主码 5 背包/物品
const SUB_BAG_LIST := 1
## 主码 10 NPC/商店
const SUB_NPC_TALK := 1
## 主码 11 任务
const SUB_QUEST_ACCEPT := 2
const SUB_QUEST_DAILY := 5
const SUB_QUEST_LIST := 1
const SUB_QUEST_PROGRESS := 4
const SUB_QUEST_SUBMIT := 3

static func is_registered(main_code: int) -> bool:
	return MAIN_CODES.has(main_code)

static func frame(main_code: int, sub_code: int, seq: int, payload: Dictionary) -> Dictionary:
	return {"main": main_code, "sub": sub_code, "seq": seq, "payload": payload}
