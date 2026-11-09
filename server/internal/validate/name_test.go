package validate

import "testing"

func TestName(t *testing.T) {
	cases := []struct {
		name, input string
		min, max    int
		want        bool
	}{
		{"中文两字", "侠客", 2, 8, true},
		{"中文八字", "断岳剑宗弟子甲乙", 2, 8, true},
		{"含下划线数字", "hero_01", 2, 16, true},
		{"过短", "甲", 2, 8, false},
		{"过长", "一二三四五六七八九", 2, 8, false},
		{"含空格", "bad name", 2, 16, false},
		{"含符号", "侠客!", 2, 8, false},
		{"空串", "", 2, 8, false},
	}
	for _, c := range cases {
		if got := Name(c.input, c.min, c.max); got != c.want {
			t.Errorf("%s: Name(%q) = %v, 期望 %v", c.name, c.input, got, c.want)
		}
	}
}

func TestHanName(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"云溪", true},
		{"清河少侠", true},
		{"六个汉字的名", true},
		{"甲", false},                    // 过短
		{"七个汉字的 角色 名", false}, // 过长（含空格亦非法）
		{"abc", false},                  // 非汉字
		{"云溪1", false},                 // 含数字
		{"", false},
	}
	for _, c := range cases {
		if got := HanName(c.input); got != c.want {
			t.Errorf("HanName(%q) = %v, 期望 %v", c.input, got, c.want)
		}
	}
}
