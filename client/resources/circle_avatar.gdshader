shader_type canvas_item;
// 定稿行动序列为圆形头像：把方形纹理裁成圆。
void fragment() {
	vec2 p = UV - vec2(0.5);
	if (dot(p, p) > 0.25) discard;
	COLOR = texture(TEXTURE, UV);
}
