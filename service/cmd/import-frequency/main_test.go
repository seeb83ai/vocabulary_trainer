package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

func TestParse_SkipsCommentsAndBlankLines(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "freq-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("# comment line\n\n你好\t1\n谢谢\t2\nmalformed\n")
	f.Seek(0, 0)

	entries, skipped, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped line, got %d", skipped)
	}
	if entries[0].word != "你好" || entries[0].rank != 1 {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].word != "谢谢" || entries[1].rank != 2 {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
}

func TestParse_BundledDataFile(t *testing.T) {
	f, err := os.Open("frequency_data.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	entries, skipped, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("bundled data file should have no malformed lines, got %d skipped", skipped)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry in the bundled data file")
	}
	if !strings.Contains(entries[0].word, "的") && entries[0].rank != 1 {
		// Not a strict requirement on content, just sanity-check ordering starts at 1.
		t.Errorf("expected first entry rank 1, got %+v", entries[0])
	}
}

func TestImportEntries_WritesWordFrequencyTable(t *testing.T) {
	os.Setenv("ADMIN_EMAIL", "admin@example.de")
	os.Setenv("ADMIN_PASSWORD", "I am the admin")
	os.Setenv("USER_EMAIL", "me@example.de")
	os.Setenv("USER_PASSWORD", "I learn zh")
	os.Setenv("BCRYPT_COST", "min")

	path := t.TempDir() + "/test.db"
	if err := vocabdb.OpenMigratedTemplate(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	n, err := importEntries(db, []freqEntry{{word: "你好", rank: 1}, {word: "谢谢", rank: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expected 2 rows written, got %d", n)
	}

	var rank int
	if err := db.QueryRow(`SELECT rank FROM word_frequency WHERE word = ?`, "你好").Scan(&rank); err != nil {
		t.Fatal(err)
	}
	if rank != 1 {
		t.Errorf("expected rank 1, got %d", rank)
	}

	// Re-importing with a different rank should update, not duplicate.
	if _, err := importEntries(db, []freqEntry{{word: "你好", rank: 99}}); err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM word_frequency WHERE word = ?`, "你好").Scan(&count)
	if count != 1 {
		t.Errorf("expected upsert to keep a single row, got %d", count)
	}
	db.QueryRow(`SELECT rank FROM word_frequency WHERE word = ?`, "你好").Scan(&rank)
	if rank != 99 {
		t.Errorf("expected updated rank 99, got %d", rank)
	}
}
