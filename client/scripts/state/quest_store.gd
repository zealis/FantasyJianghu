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
var exp_next: int = 0        # 当前级升级需求（11/1，M2c 切片三）
var gate_ready: bool = false # 恰处关口且经验满，可手动突破
var next_gate: int = 0       # 下一个突破关口等级（0=已过最后关口或满级）
var unspent_points: int = 0  # 未分配属性点
var points: Dictionary = {"str": 0, "spi": 0, "agi": 0, "con": 0}  # 已分配一级属性
var stats: Dictionary = {}   # 派生属性（11/1 stats，加点界面预览用）
var wallet: Dictionary = {"gold": 0, "xiuwei": 0, "yuanbao": 0}
var daily: Dictionary = {}      # normal/sect/dungeon → {used, limit}
var quests: Array = []          # 服务端 questView 数组（原样保留，UI 按 state 过滤）
var loaded: bool = false

# 5/1 背包快照。
var bag: Dictionary = {"capacity": 0, "used": 0, "items": []}

# 本地 UI 状态：面板点选/前往指定的追踪目标（空串=按主线优先自动选）。
var tracked_override: String = ""

const CATEGORY_NAME := {"normal": "普通日常", "sect": "宗派日常", "dungeon": "副本日常"}
const EFFECT_NAME := {"exp": "经验", "xiuwei": "修为", "gold": "金币"}
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
		# 5/1 同时回钱包：出售/使用后金币要即时刷新（原先只靠 11/1 才更新）。
		wallet = resp.get("wallet", wallet)
		changed.emit()

## 使用物品（5/2，M2c 切片一）；返回错误文案（空串=成功）。
func use_item(item_id: String) -> String:
	var resp: Dictionary = await Network.send_and_wait(5, Protocol.SUB_BAG_USE, {"item_id": item_id})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "使用失败"))
		notified.emit(msg)
		return msg
	var effect: Dictionary = resp.get("effect", {})
	var item_name := item_id
	for it in bag.get("items", []):
		if str(it.get("id", "")) == item_id:
			item_name = str(it.get("name", item_id))
	var kind := str(effect.get("kind", ""))
	notified.emit("使用 %s：%s +%d" % [item_name, str(EFFECT_NAME.get(kind, kind)), int(effect.get("value", 0))])
	await refresh()
	return ""

## 整理背包（5/3）：服务端按品质/类别重排并持久化。
func sort_bag() -> String:
	var resp: Dictionary = await Network.send_and_wait(5, Protocol.SUB_BAG_SORT, {})
	if int(resp.get("code", -1)) != 0:
		var msg := str(resp.get("msg", "整理失败"))
		notified.emit(msg)
		return msg
	notified.emit("背包已整理")
	await refresh_bag()
	return ""

## 出售物品（5/4）：count 默认 1，单价由服务端配置 sell 决定。
func sell_item(item_id: String, count: int = 1) -> String:
	var resp: Dictionary = await Network.send_and_wait(5, Protocol.SUB_BAG_SELL,
		{"item_id": item_id, "count": count})
	if int(resp.get("code", -1)) != 0:
		var msg := str(resp.get("msg", "出售失败"))
		notified.emit(msg)
		return msg
	notified.emit("出售获得 %d 金币" % int(resp.get("gained", 0)))
	await refresh_bag()
	return ""

## 锁定/解锁物品（5/5）：锁定后禁止使用与出售。
func toggle_lock(item_id: String, locked: bool) -> String:
	var resp: Dictionary = await Network.send_and_wait(5, Protocol.SUB_BAG_LOCK,
		{"item_id": item_id, "locked": locked})
	if int(resp.get("code", -1)) != 0:
		var msg := str(resp.get("msg", "锁定失败"))
		notified.emit(msg)
		return msg
	notified.emit("已锁定" if locked else "已解锁")
	await refresh_bag()
	return ""

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

## 追踪目标：优先面板点选/前往指定的任务，其次进行中/可交付主线，再可接主线，最后任一进行中。
func tracked() -> Dictionary:
	if tracked_override != "":
		var picked := by_id(tracked_override)
		if not picked.is_empty() and str(picked.get("state", "")) != "done":
			return picked
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
	exp_next = int(payload.get("exp_next", exp_next))
	gate_ready = bool(payload.get("gate_ready", false))
	next_gate = int(payload.get("next_gate", 0))
	unspent_points = int(payload.get("unspent_points", 0))
	points = payload.get("points", points)
	stats = payload.get("stats", stats)
	wallet = payload.get("wallet", wallet)
	daily = payload.get("daily", daily)
	quests = payload.get("quests", quests)
	loaded = true
	# 等级/经验条转真：回填 session 域供 HUD 等级徽与经验条（服务端权威）。
	SessionStore.level = level
	SessionStore.exp_ratio = (float(exp) / float(exp_next)) if exp_next > 0 else 0.0
	SessionStore.notify_changed()
	changed.emit()

## 属性加点（2/5）：delta 为本次预加的增量 {str,spi,agi,con}；返回错误文案（空串=成功）。
func allocate(delta: Dictionary) -> String:
	var resp: Dictionary = await Network.send_and_wait(2, Protocol.SUB_CHAR_ALLOCATE, delta)
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "加点失败"))
		notified.emit(msg)
		return msg
	if resp.has("points"):
		points = resp.get("points", points)
		unspent_points = int(resp.get("unspent", unspent_points))
	notified.emit("加点成功")
	await refresh()
	return ""

## 洗点（2/6）：返还全部已分配点（本轮免费）；返回错误文案（空串=成功）。
func reset_points() -> String:
	var resp: Dictionary = await Network.send_and_wait(2, Protocol.SUB_CHAR_RESET_POINTS, {})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "洗点失败"))
		notified.emit(msg)
		return msg
	if resp.has("points"):
		points = resp.get("points", points)
		unspent_points = int(resp.get("unspent", unspent_points))
	notified.emit("洗点完成，已返还全部属性点")
	await refresh()
	return ""

## 手动突破关口（2/4）；返回错误文案（空串=成功）。
func breakthrough() -> String:
	var resp: Dictionary = await Network.send_and_wait(2, Protocol.SUB_CHAR_BREAKTHROUGH, {})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "突破失败"))
		notified.emit(msg)
		return msg
	notified.emit("突破成功，等级 %d" % int(resp.get("level", level)))
	await refresh()
	return ""

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
