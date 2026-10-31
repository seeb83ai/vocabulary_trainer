package handlers

import "testing"

func TestZhMatchKey(t *testing.T) {
	same := [][2]string{
		{"花（动词）", "花 (动词)"},
		{"花（动词）", "花(动词)"},
		{"花（动词）", "花　（动词）"},
		{"花（动词）", "花 ( 动词 )"},
		{"你好", " 你 好 "},
	}
	for _, c := range same {
		if zhMatchKey(c[0]) != zhMatchKey(c[1]) {
			t.Errorf("zhMatchKey(%q) = %q, zhMatchKey(%q) = %q: want equal", c[0], zhMatchKey(c[0]), c[1], zhMatchKey(c[1]))
		}
	}
	different := [][2]string{
		{"花（动词）", "花（名词）"},
		{"花（动词）", "花"},
		{"花（动词）", "花[动词]"},
	}
	for _, c := range different {
		if zhMatchKey(c[0]) == zhMatchKey(c[1]) {
			t.Errorf("zhMatchKey(%q) == zhMatchKey(%q): want different", c[0], c[1])
		}
	}
}
