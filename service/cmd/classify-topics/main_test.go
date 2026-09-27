package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"vocabulary_trainer/models"
)

func TestParseResponse(t *testing.T) {
	batch := []models.TopicCandidate{{Zh: "护照"}, {Zh: "的"}, {Zh: "医生"}, {Zh: "漏掉"}}
	text := "Here you go:\n" +
		"护照\ttravel, unknown-topic\n" +
		"的\t-\n" +
		"医生\thealth,work\n" +
		"不在批次\tfood\n"

	got := parseResponse(text, batch)
	want := map[string][]string{
		"护照": {"travel"},
		"的":  {},
		"医生": {"health", "work"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWriteResults_AndLoadDone(t *testing.T) {
	dir := t.TempDir()
	batch := []models.TopicCandidate{{Zh: "护照", En: "passport"}, {Zh: "的", En: "of, possessive"}}
	res := map[string][]string{"护照": {"travel", "shopping"}, "的": {}}

	if err := writeResults(dir, batch, res); err != nil {
		t.Fatal(err)
	}
	if err := writeResults(dir, []models.TopicCandidate{{Zh: "签证", En: "visa"}}, map[string][]string{"签证": {"travel"}}); err != nil {
		t.Fatal(err)
	}

	travel, err := os.ReadFile(filepath.Join(dir, "travel.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if string(travel) != "zh,en\n护照,passport\n签证,visa\n" {
		t.Errorf("travel.csv = %q", travel)
	}
	none, err := os.ReadFile(filepath.Join(dir, "_none.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if string(none) != "zh,en\n的,\"of, possessive\"\n" {
		t.Errorf("_none.csv = %q", none)
	}

	done, err := loadDone(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"护照": true, "的": true, "签证": true}
	if !reflect.DeepEqual(done, want) {
		t.Errorf("done = %v, want %v", done, want)
	}
}
