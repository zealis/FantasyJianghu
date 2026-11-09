extends Node
## 状态层 · 任务域（GDD §9 / 技术规范 §4 quest）。
## 服务端权威：11/1 全量状态、11/2 接取、11/3 交付、11/4 进度推送、11/5 日常计数。
## 本单例只缓存与转换，不做任何结算；UI 层订阅 changed 信号渲染。

signal changed
signal reward_granted(rewards: Dictionary)
signal notified(msg: String)

# 11/1 全量快照。
var level: int = 1
var exp: int = 0
var wallet: Dictionary = {"gold": 0, "xiuwei": 0, "yuanbao": 0}
var daily: Dictionary = {}      # normal/sect/dungeon → {used, limit}
var quests: Array = []          # 服务端 questView 数组（原样保留，UI 按 state 过滤）
var loaded: bool = false

# 5/1 背包快照。
var bag: Dictionary = {"capacity": 0, "used": 0, "items": []}

const CATEGORY_NAME := {"normal": "普通日常", "sect": "宗派日常", "dungeon": "副本日常"}
const STATE_TEXT := {
	"available": "可接取", "active": "进行中", "completable": "可交付",
	"done": "已完成", "locked": "未开放",
}

func _ready() -> void:
	Network.packet_received.connect(_on_packet)

# ---- 请求 ----

## 拉全量任务状态（进场/交付后调用）。
func refresh() -> void:
	var resp: Dictionary = await Network.send_and_wait(11, 1, {})
	if int(resp.get("code", -1)) == 0:
		_apply(resp)
		await refresh_bag()

## 拉背包与钱包（5/1）。
func refresh_bag() -> void:
	var resp: Dictionary = await Network.send_and_wait(5, 1, {})
	if int(resp.get("code", -1)) == 0:
		bag = resp
		changed.emit()

## 接取任务；返回错误文案（空串=成功）。
func accept(quest_id: String) -> String:
	var resp: Dictionary = await Network.send_and_wait(11, 2, {"quest_id": quest_id})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "接取失败"))
		notified.emit(msg)
		return msg
	if resp.has("quests"):
		quests = resp.get("quests", [])
		daily = resp.get("daily", daily)
		changed.emit()
	notified.emit("已接取任务")
	return ""

## 交付任务；返回错误文案（空串=成功）。
func submit(quest_id: String) -> String:
	var resp: Dictionary = await Network.send_and_wait(11, 3, {"quest_id": quest_id})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "交付失败"))
		notified.emit(msg)
		return msg
	reward_granted.emit(resp.get("rewards", {}))
	if resp.has("daily_chest"):
		notified.emit("该类日常已达上限，宝箱奖励将于邮件补发（M2c）")
	await refresh()
	return ""

# ---- 查询（UI 用） ----

func by_id(quest_id: String) -> Dictionary:
	for q in quests:
		if str(q.get("id", "")) == quest_id:
			return q
	return {}

## 某 NPC 身上可接/可交的任务（对话面板注入用）。
func quests_for_npc(npc_id: String) -> Array:
	var out: Array = []
	for q in quests:
		var giver: Dictionary = q.get("giver", {})
		if str(giver.get("npc_id", "")) != npc_id:
			continue
		var st := str(q.get("state", ""))
		if st == "available" or st == "completable":
			out.append(q)
	return out

## 追踪目标：优先进行中/可交付主线，其次可接主线，最后任一进行中任务。
func tracked() -> Dictionary:
	var fallback: Dictionary = {}
	for q in quests:
		var st := str(q.get("state", ""))
		if st == "completable" and str(q.get("type", "")) == "main":
			return q
		if st == "active":
			if str(q.get("type", "")) == "main":
				return q
			if fallback.is_empty():
				fallback = q
	for q in quests:
		if str(q.get("state", "")) == "available" and str(q.get("type", "")) == "main":
			return q
	return fallback

func main_quests() -> Array:
	var out: Array = []
	for q in quests:
		if str(q.get("type", "")) == "main":
			out.append(q)
	return out

func daily_quests() -> Array:
	var out: Array = []
	for q in quests:
		if str(q.get("type", "")) != "main":
			out.append(q)
	return out

## 目标进度文案（客户端只做展示格式化）。
func progress_text(q: Dictionary) -> String:
	var parts: Array = []
	for o in q.get("objectives", []):
		var obj: Dictionary = o
		parts.append("%s %d/%d" % [_objective_name(str(obj.get("kind", "")), str(obj.get("target", ""))),
			int(obj.get("cur", 0)), int(obj.get("need", 0))])
	return "  ".join(parts)

func _objective_name(kind: String, target: String) -> String:
	match kind:
		"talk":
			return "对话"
		"kill":
			return "击杀 %s" % _monster_name(target)
		"visit":
			return "前往 %s" % target
	return kind

func _monster_name(id: String) -> String:
	const NAMES := {"muzhuang": "木桩人", "yelinang": "野狼", "shanzei": "山贼", "jingji_wushi": "荆棘武士"}
	return str(NAMES.get(id, id))

# ---- 服务端推送 ----

func _apply(payload: Dictionary) -> void:
	level = int(payload.get("level", level))
	exp = int(payload.get("exp", exp))
	wallet = payload.get("wallet", wallet)
	daily = payload.get("daily", daily)
	quests = payload.get("quests", quests)
	loaded = true
	changed.emit()

func _on_packet(main_code: int, sub_code: int, payload: Dictionary) -> void:
	# 11/4 进度推送：合并到本地任务快照（推送只带 quest_id/state/objectives）。
	if main_code == 11 and sub_code == 4:
		var qid := str(payload.get("quest_id", ""))
		for i in range(quests.size()):
			if str(quests[i].get("id", "")) == qid:
				quests[i]["state"] = payload.get("state", quests[i].get("state", ""))
				quests[i]["objectives"] = payload.get("objectives", quests[i].get("objectives", []))
				changed.emit()
				return
