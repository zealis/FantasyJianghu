class_name SceneLogic
extends RefCounted
## 场景与逻辑分离（前端规范 §4）：可行走路径段/NPC 点位/热区独立于场景图。
## 换图不得改动逻辑文件结构；本加载器只读 JSON。

static var _cache: Dictionary = {}
## 当前主流程使用的逻辑数据路径（横版默认；切到俯视场景时由 main.gd 覆盖）。
static var current_path: String = "res://data/qinghecun_logic.json"

static func get_logic(path: String = "") -> Dictionary:
	var p := path if path != "" else current_path
	if _cache.has(p):
		return _cache[p]
	var text := FileAccess.get_file_as_string(p)
	var data: Dictionary = JSON.parse_string(text)
	_cache[p] = data if data is Dictionary else {}
	return _cache[p]

static func ground_y() -> float:
	return float(get_logic().get("ground_y", 1050))

static func walk_bounds() -> Vector2:
	var w: Dictionary = get_logic().get("walk", {})
	return Vector2(float(w.get("x_min", 80)), float(w.get("x_max", 1840)))