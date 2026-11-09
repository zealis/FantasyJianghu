extends RefCounted
## 头像素材统一处理：assets/avatars/*.png 自带一圈装裱画框，部分门派顶部还多一条
## 深色横杆（惊雷殿 6.3%、断岳剑宗 2.7%、破军枪府 2.2%；人物主体最早 11.8%）。
## 缩到 64~68px 后横杆就变成「角色头顶一根横线」，需在显示前裁掉四周画框。
## 用法（preload 调用静态方法，避免依赖 class_name 缓存）：
##   const AvatarFrames := preload("res://scripts/ui/avatar_frames.gd")
##   tex = AvatarFrames.trim(tex)

const FRAME_RATIO := 0.11

static func trim(tex: Texture2D, ratio: float = FRAME_RATIO) -> Texture2D:
	if tex == null:
		return null
	var w: int = tex.get_width()
	var h: int = tex.get_height()
	var cx: int = int(float(w) * ratio)
	var cy: int = int(float(h) * ratio)
	if w - 2 * cx < 16 or h - 2 * cy < 16:
		return tex
	var atlas := AtlasTexture.new()
	atlas.atlas = tex
	atlas.region = Rect2(cx, cy, w - 2 * cx, h - 2 * cy)
	return atlas
