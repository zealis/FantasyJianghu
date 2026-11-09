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

static func is_registered(main_code: int) -> bool:
	return MAIN_CODES.has(main_code)

static func frame(main_code: int, sub_code: int, seq: int, payload: Dictionary) -> Dictionary:
	return {"main": main_code, "sub": sub_code, "seq": seq, "payload": payload}