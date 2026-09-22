package db

import (
	"context"
	"reflect"
	"testing"
)

func TestGetLookalikes_SeededPairsBothDirections(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	cases := map[string][]string{
		"囗": {"口"}, "口": {"囗"},
		"土": {"士"}, "士": {"土"},
		"未": {"末"}, "末": {"未"},
		"已": {"巳"}, "巳": {"已"},
		"日": {"曰"}, "曰": {"日"},
	}
	for char, want := range cases {
		got, err := s.GetLookalikes(ctx, char)
		if err != nil {
			t.Fatalf("GetLookalikes(%q): %v", char, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetLookalikes(%q) = %v, want %v", char, got, want)
		}
	}
}

func TestGetLookalikes_NoMatchReturnsEmpty(t *testing.T) {
	s := openTestDB(t)
	for _, text := range []string{"人", "口口", ""} {
		got, err := s.GetLookalikes(context.Background(), text)
		if err != nil {
			t.Fatalf("GetLookalikes(%q): %v", text, err)
		}
		if len(got) != 0 {
			t.Errorf("GetLookalikes(%q) = %v, want empty", text, got)
		}
	}
}

func TestGetLookalikes_MultipleSortedByCharacter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO lookalike_chars (character, lookalike) VALUES ('己', '已'), ('己', '巳')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := s.GetLookalikes(ctx, "己")
	if err != nil {
		t.Fatalf("GetLookalikes: %v", err)
	}
	want := []string{"已", "巳"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
