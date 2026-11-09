class_name SceneLogic
extends RefCounted
## 场景与逻辑分离（前端规范 §4）：可行走路径段/NPC 点位/热区独立于场景图。
## 换图不得改动逻辑文件结构；本加载器只读 JSON。

static var _cache: Dictionary = {}

static func get_logic(path: String = "res://data/qinghecun_logic.json") -> Dictionary:
	if _cache.has(path):
		return _cache[path]
	var text := FileAccess.get_file_as_string(path)
	var data: Dictionary = JSON.parse_string(text)
	_cache[path] = data if data is Dictionary else {}
	return _cache[path]

static func ground_y() -> float:
	return float(get_logic().get("ground_y", 1050))

static func walk_bounds() -> Vector2:
	var w: Dictionary = get_logic().get("walk", {})
	return Vector2(float(w.get("x_min", 80)), float(w.get("x_max", 1840)))