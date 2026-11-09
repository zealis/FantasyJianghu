// Package validate 为跨模块的输入校验。
package validate

import "unicode"

// Name 校验名称：中文/字母/数字/下划线，长度按 rune 计（min..max 含边界）。
func Name(name string, min, max int) bool {
	n := 0
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		case unicode.Is(unicode.Han, r):
		default:
			return false
		}
		n++
	}
	return n >= min && n <= max
}

// HanName 校验角色名：2–6 个汉字（§9 命名界面定稿口径）。
func HanName(name string) bool {
	n := 0
	for _, r := range name {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
		n++
	}
	return n >= 2 && n <= 6
}
