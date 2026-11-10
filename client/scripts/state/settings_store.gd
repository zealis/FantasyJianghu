extends Node
## 状态层 · 设置（本地偏好，user://settings.cfg 持久化；不涉服务端权威资产）。
## 每个开关都有真实落点：音频总线/静音、MSAA 画质、帧率与 vsync 省电、
## 战斗倍速/自动/飘字/出招位移演出、消息提醒 toast。震屏未接入故无震动项。

signal changed

const PATH := "user://settings.cfg"
const KEYS := ["bgm", "sfx", "mute", "quality", "vfx", "power_save",
	"auto_battle", "speed_index", "float_text", "notify", "fps_cap"]

var bgm: int = 70
var sfx: int = 85
var mute: bool = false
var quality: int = 2          # 0低 1中 2高 → 画布过滤 nearest/nearest/linear，低档加像素 snap
var vfx: bool = true          # 出招位移演出
var power_save: bool = false  # 帧率压 30 + vsync
var auto_battle: bool = false
var speed_index: int = 0      # ×1/×2/×3
var float_text: bool = true
var notify: bool = true
var fps_cap: int = 60

func _ready() -> void:
	_ensure_buses()
	load_settings()
	apply()

func _ensure_buses() -> void:
	for bus_name in ["Music", "SFX"]:
		if AudioServer.get_bus_index(bus_name) == -1:
			AudioServer.add_bus()
			AudioServer.set_bus_name(AudioServer.bus_count - 1, bus_name)

func set_value(key: String, value: Variant) -> void:
	if key not in KEYS:
		return
	set(key, value)
	save()
	apply()
	changed.emit()

func apply() -> void:
	AudioServer.set_bus_mute(AudioServer.get_bus_index("Master"), mute)
	AudioServer.set_bus_volume_db(AudioServer.get_bus_index("Music"), linear_to_db(maxf(float(bgm), 0.01) / 100.0))
	AudioServer.set_bus_volume_db(AudioServer.get_bus_index("SFX"), linear_to_db(maxf(float(sfx), 0.01) / 100.0))
	var vp := get_viewport()
	if vp:
		# Compatibility 渲染器不支持 2D MSAA；画质落到真实生效的画布过滤与像素_snap。
		vp.canvas_item_default_texture_filter = Viewport.DEFAULT_CANVAS_ITEM_TEXTURE_FILTER_LINEAR \
			if quality >= 2 else Viewport.DEFAULT_CANVAS_ITEM_TEXTURE_FILTER_NEAREST
		vp.snap_2d_transforms_to_pixel = quality == 0
	Engine.max_fps = mini(fps_cap, 30) if power_save else fps_cap
	DisplayServer.window_set_vsync_mode(DisplayServer.VSYNC_ENABLED if power_save else DisplayServer.VSYNC_DISABLED)

func save() -> void:
	var cf := ConfigFile.new()
	for key in KEYS:
		cf.set_value("settings", key, get(key))
	cf.save(PATH)

func load_settings() -> void:
	if not FileAccess.file_exists(PATH):
		return
	var cf := ConfigFile.new()
	if cf.load(PATH) != OK:
		return
	for key in KEYS:
		if cf.has_section_key("settings", key):
			set(key, cf.get_value("settings", key))