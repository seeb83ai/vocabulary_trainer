package db

import (
	"context"
	"testing"
	"vocabulary_trainer/models"
)

func TestZhMatchKey(t *testing.T) {
	same := [][2]string{
		{"花（动词）", "花 (动词)"},
		{"花（动词）", "花(动词)"},
		{"花（动词）", "花　（动词）"},
		{"花（动词）", "花 ( 动词 )"},
		{"你好", " 你 好 "},
	}
	for _, c := range same {
		if ZhMatchKey(c[0]) != ZhMatchKey(c[1]) {
			t.Errorf("ZhMatchKey(%q) = %q, ZhMatchKey(%q) = %q: want equal", c[0], ZhMatchKey(c[0]), c[1], ZhMatchKey(c[1]))
		}
	}
	different := [][2]string{
		{"花（动词）", "花（名词）"},
		{"花（动词）", "花"},
		{"花（动词）", "花[动词]"},
	}
	for _, c := range different {
		if ZhMatchKey(c[0]) == ZhMatchKey(c[1]) {
			t.Errorf("ZhMatchKey(%q) == ZhMatchKey(%q): want different", c[0], c[1])
		}
	}
}

func TestZhWordTextsByMatchKey(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	seedWord(t, s, "花 (动词)", "huā (dòng cí)", []string{"spend"})
	if _, err := s.CreateWord(ctx, LibraryUserID, models.CreateWordRequest{
		ZhText: "你好", Translations: map[string][]string{"en": {"hello"}},
	}); err != nil {
		t.Fatalf("CreateWord library user: %v", err)
	}

	got, err := s.ZhWordTextsByMatchKey(ctx, 2)
	if err != nil {
		t.Fatalf("ZhWordTextsByMatchKey: %v", err)
	}
	// Only zh words of user 2: not the en word "spend", not the library word 你好.
	if len(got) != 1 || got[ZhMatchKey("花（动词）")] != "花 (动词)" {
		t.Errorf("want {%q: 花 (动词)}, got %v", ZhMatchKey("花（动词）"), got)
	}
}
