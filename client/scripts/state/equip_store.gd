extends Node
## 状态层 · 装备域（M2c 切片二：6/1 穿戴、6/2 卸下、6/3 视图）。
## 只缓存服务端视图（槽位明细 + 属性），不本地推演加成；UI 订阅 changed 渲染。

signal changed

var slots: Dictionary = {}   # slot → {id, name, quality, stats}
var attrs: Dictionary = {}   # hp/mp/patk/.../crit(万分制)
var loaded: bool = false

func refresh() -> void:
	var resp: Dictionary = await Network.send_and_wait(6, Protocol.SUB_EQUIP_VIEW, {})
	if int(resp.get("code", -1)) == 0:
		slots = resp.get("slots", {})
		attrs = resp.get("attrs", {})
		loaded = true
		changed.emit()

## 穿戴；返回错误文案（空串=成功）。
func wear(item_id: String) -> String:
	var resp: Dictionary = await Network.send_and_wait(6, Protocol.SUB_EQUIP_WEAR, {"item_id": item_id})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "穿戴失败"))
		QuestStore.notified.emit(msg)
		return msg
	await refresh()
	QuestStore.notified.emit("已穿戴装备")
	return ""

## 卸下；返回错误文案（空串=成功）。
func remove(slot: String) -> String:
	var resp: Dictionary = await Network.send_and_wait(6, Protocol.SUB_EQUIP_REMOVE, {"slot": slot})
	var code := int(resp.get("code", -1))
	if code != 0:
		var msg := str(resp.get("msg", "卸下失败"))
		QuestStore.notified.emit(msg)
		return msg
	await refresh()
	await QuestStore.refresh_bag()
	QuestStore.notified.emit("已卸下装备")
	return ""

func equipped_ids() -> Array:
	var out: Array = []
	for slot in slots:
		out.append(str((slots[slot] as Dictionary).get("id", "")))
	return out

func slot_of_item(item_id: String) -> String:
	for slot in slots:
		if str((slots[slot] as Dictionary).get("id", "")) == item_id:
			return slot
	return ""