package db

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"vocabulary_trainer/models"
)

// TestMain sets migration credential env vars once for the entire test binary.
func TestMain(m *testing.M) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Setenv("BCRYPT_COST", "min") // speed up bcrypt in tests
	os.Exit(m.Run())
}

var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// buildTemplateDB runs all migrations once into a scratch file so individual
// tests can clone it instead of re-running migrations for every test.
func buildTemplateDB(tb testing.TB) string {
	tb.Helper()
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "vocab-test-template-*")
		if err != nil {
			templateErr = err
			return
		}
		path := filepath.Join(dir, "template.db")
		if err := OpenMigratedTemplate(path); err != nil {
			templateErr = err
			return
		}
		templatePath = path
	})
	if templateErr != nil {
		tb.Fatalf("build template db: %v", templateErr)
	}
	return templatePath
}

// openTestDB creates a SQLite store for tests by cloning the pre-migrated
// template database rather than running all migrations from scratch.
func openTestDB(t *testing.T) *Store {
	t.Helper()
	tmpl := buildTemplateDB(t)
	data, err := os.ReadFile(tmpl)
	if err != nil {
		t.Fatalf("read template db: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write test db copy: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("openTestDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedWord inserts one full vocabulary entry and returns the zh word ID.
func seedWord(t *testing.T, s *Store, zhText, pinyin string, enTexts []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       zhText,
		Pinyin:       pinyin,
		Translations: map[string][]string{"en": enTexts},
	})
	if err != nil {
		t.Fatalf("seedWord %q: %v", zhText, err)
	}
	return id
}

func seedWordWithTags(t *testing.T, s *Store, zhText, pinyin string, enTexts, tags []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(2), models.CreateWordRequest{
		ZhText:       zhText,
		Pinyin:       pinyin,
		Translations: map[string][]string{"en": enTexts},
		Tags:         tags,
	})
	if err != nil {
		t.Fatalf("seedWordWithTags %q: %v", zhText, err)
	}
	return id
}

func seedTemplateWord(t *testing.T, s *Store, zhText, pinyin string, enTexts []string, tags []string) int64 {
	t.Helper()
	id, err := s.CreateWord(context.Background(), int64(1), models.CreateWordRequest{
		ZhText:       zhText,
		Pinyin:       pinyin,
		Translations: map[string][]string{"en": enTexts},
		Tags:         tags,
	})
	if err != nil {
		t.Fatalf("seedTemplateWord %q: %v", zhText, err)
	}
	return id
}

// makeDifficult marks a seeded word as graduated (learning_new_word=0) with the
// given accuracy/easiness so it is a candidate for the difficult-words drill.
func makeDifficult(t *testing.T, s *Store, id int64, totalCorrect, totalAttempts int, easiness float64, dueOffset time.Duration) {
	t.Helper()
	due := time.Now().UTC().Add(dueOffset).Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE sm2_progress SET learning_new_word = 0, first_seen_at = datetime('now'),
		 total_correct = ?, total_attempts = ?, easiness = ?, due_date = ?, drill_flag = 0
		 WHERE word_id = ?`,
		totalCorrect, totalAttempts, easiness, due, id)
	if err != nil {
		t.Fatalf("makeDifficult(%d): %v", id, err)
	}
}

func TestParseDateTime_RFC3339(t *testing.T) {
	s := "2026-02-21T15:04:05Z"
	got := parseDateTime(s)
	if got.IsZero() {
		t.Errorf("parseDateTime(%q) returned zero time", s)
	}
}

func TestParseDateTime_SQLiteFormat(t *testing.T) {
	s := "2026-02-21 15:04:05"
	got := parseDateTime(s)
	if got.IsZero() {
		t.Errorf("parseDateTime(%q) returned zero time", s)
	}
	if got.Year() != 2026 || got.Month() != 2 || got.Day() != 21 {
		t.Errorf("wrong date parsed: %v", got)
	}
}

func TestParseDateTime_InvalidReturnsZero(t *testing.T) {
	got := parseDateTime("not-a-date")
	if !got.IsZero() {
		t.Errorf("invalid input should return zero time, got %v", got)
	}
}

// TestBackupRestore_RoundTrip verifies the SQLite online-backup primitive used by
// the scheduled backup (VACUUM INTO, the same mechanism as `sqlite3 .backup`)
// produces a restorable copy with data intact. Production uses `sqlite3 .backup`;
// this exercises the equivalent via the Go driver so it runs without the CLI.
func TestBackupRestore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src, err := Open(filepath.Join(dir, "vocab.db"))
	if err != nil {
		t.Fatalf("open source: %v", err)
	}
	id := seedWord(t, src, "備份測試詞", "bèi fèn cí", []string{"backup marker"})

	backupPath := filepath.Join(dir, "vocab_backup.sq3")
	if _, err := src.db.Exec("VACUUM INTO ?", backupPath); err != nil {
		t.Fatalf("backup (VACUUM INTO): %v", err)
	}
	src.Close()

	// Restore = open the backup file as a normal DB.
	restored, err := Open(backupPath)
	if err != nil {
		t.Fatalf("open restored backup: %v", err)
	}
	defer restored.Close()

	wd, err := restored.GetWordByID(context.Background(), 2, id)
	if err != nil {
		t.Fatalf("read restored word: %v", err)
	}
	if wd == nil || wd.ZhText != "備份測試詞" {
		t.Fatalf("seeded word did not survive backup/restore: %+v", wd)
	}
}
