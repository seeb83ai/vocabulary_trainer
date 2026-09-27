package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestClaudeCLI(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	// The fake prints its arguments, one per line, then echoes stdin.
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"arg:$a\"; done\ncat\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := claudeCLI(context.Background(), bin, "claude-opus-5", "low", "SYS", "护照\tpassport\n")
	if err != nil {
		t.Fatalf("claudeCLI: %v", err)
	}
	for _, want := range []string{"arg:-p", "arg:--model\narg:claude-opus-5", "arg:--effort\narg:low",
		"arg:--system-prompt\narg:SYS", "arg:--tools\narg:\n", "护照\tpassport"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestClaudeCLI_Error(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'usage limit reached' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := claudeCLI(context.Background(), bin, "m", "low", "s", "u")
	if err == nil || !strings.Contains(err.Error(), "usage limit reached") {
		t.Errorf("err = %v, want stderr in error", err)
	}
}

// answerAll replies "-" (no topic) for every word in the prompt, and counts
// how many calls run at the same time.
func answerAll(inFlight, peak *atomic.Int32) func(string) (string, error) {
	return func(user string) (string, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		var b strings.Builder
		for _, line := range strings.Split(strings.TrimSpace(user), "\n") {
			zh, _, _ := strings.Cut(line, "\t")
			b.WriteString(zh + "\t-\n")
		}
		return b.String(), nil
	}
}

func TestClassifyAll_Parallel(t *testing.T) {
	dir := t.TempDir()
	var todo []models.TopicCandidate
	for i := range 20 {
		todo = append(todo, models.TopicCandidate{Zh: fmt.Sprintf("词%02d", i)})
	}
	var inFlight, peak atomic.Int32

	classified, missing, err := classifyAll(context.Background(), todo, 2, 4, dir, answerAll(&inFlight, &peak))
	if err != nil {
		t.Fatal(err)
	}
	if classified != 20 || missing != 0 {
		t.Errorf("classified=%d missing=%d, want 20/0", classified, missing)
	}
	if p := peak.Load(); p < 2 || p > 4 {
		t.Errorf("peak concurrency = %d, want 2..4", p)
	}
	done, err := loadDone(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 20 {
		t.Errorf("%d words written, want 20", len(done))
	}
}

func TestClassifyAll_StopsOnErrorAndKeepsFinishedBatches(t *testing.T) {
	dir := t.TempDir()
	var todo []models.TopicCandidate
	for i := range 10 {
		todo = append(todo, models.TopicCandidate{Zh: fmt.Sprintf("词%02d", i)})
	}
	var inFlight, peak atomic.Int32
	ok := answerAll(&inFlight, &peak)
	var calls atomic.Int32
	gen := func(user string) (string, error) {
		if calls.Add(1) == 3 {
			return "", errors.New("usage limit reached")
		}
		return ok(user)
	}

	_, _, err := classifyAll(context.Background(), todo, 1, 1, dir, gen)
	if err == nil || !strings.Contains(err.Error(), "usage limit reached") {
		t.Fatalf("err = %v, want usage limit error", err)
	}
	done, _ := loadDone(dir)
	if len(done) != 2 {
		t.Errorf("%d words written, want the 2 finished before the error", len(done))
	}
}

func TestClassifyAll_CancelFinishesInFlight(t *testing.T) {
	dir := t.TempDir()
	var todo []models.TopicCandidate
	for i := range 10 {
		todo = append(todo, models.TopicCandidate{Zh: fmt.Sprintf("词%02d", i)})
	}
	ctx, cancel := context.WithCancel(context.Background())
	var inFlight, peak atomic.Int32
	ok := answerAll(&inFlight, &peak)
	gen := func(user string) (string, error) {
		cancel() // Ctrl-C while the first batch runs
		return ok(user)
	}

	_, _, err := classifyAll(ctx, todo, 1, 1, dir, gen)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	done, _ := loadDone(dir)
	if len(done) != 1 {
		t.Errorf("%d words written, want only the in-flight batch (1)", len(done))
	}
}
