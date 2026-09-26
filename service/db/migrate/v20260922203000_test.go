package migrate

import (
	"database/sql"
	"sort"
	"testing"
)

// deLinksFor returns "text|source" for every translation linked to zhID.
func deLinksFor(t *testing.T, db *sql.DB, zhID int64) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT w.text || '|' || t.source FROM translations t
		JOIN words w ON w.id = t.translation_word_id
		WHERE t.zh_word_id = ?`, zhID)
	if err != nil {
		t.Fatalf("query links: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestSplitCedictTranslationsAtComma(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20260922183500)

	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '为', 'zh', 2),
		(2, 'wegen, um (P), im Bestreben (S)', 'de', 2),
		(3, 'fungieren (in, als)', 'de', 2),
		(4, 'a, b', 'de', 2),
		(5, 'wegen', 'de', 2),
		(6, '因为', 'zh', 2),
		(7, 'as, for', 'en', 2),
		(8, '对', 'zh', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id) VALUES (1), (2), (3), (4), (5), (6), (7), (8)`); err != nil {
		t.Fatalf("insert progress: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source, rank) VALUES
		(2, 1, 'cedict', NULL),
		(3, 1, 'cedict', NULL),
		(4, 1, 'user', 0),
		(5, 6, 'cedict', NULL),
		(7, 1, 'cedict', NULL),
		(7, 8, 'user', 0)`); err != nil {
		t.Fatalf("insert translations: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	got := deLinksFor(t, db, 1)
	want := []string{"a, b|user", "as|cedict", "for|cedict", "fungieren (in, als)|cedict", "im Bestreben (S)|cedict", "um (P)|cedict", "wegen|cedict"}
	if len(got) != len(want) {
		t.Fatalf("links for 为 = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("links[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// The existing "wegen" word is reused, not duplicated.
	var wegenID int64
	if err := db.QueryRow(`SELECT id FROM words WHERE text = 'wegen' AND language = 'de' AND user_id = 2`).Scan(&wegenID); err != nil {
		t.Fatalf("find wegen: %v", err)
	}
	if wegenID != 5 {
		t.Errorf("want existing wegen word 5 reused, got id %d", wegenID)
	}

	// The split-up word has no links left and is removed with its progress.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE id = 2`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("want orphaned comma-joined word 2 deleted")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = 2`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("want progress of deleted word 2 removed")
	}

	// "as, for" is still linked as a user translation of 对, so it stays.
	if err := db.QueryRow(`SELECT COUNT(*) FROM translations WHERE translation_word_id = 7 AND zh_word_id = 8`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("want user-sourced link of word 7 kept")
	}

	// New sense words get progress rows and a frequency rank.
	var umID int64
	if err := db.QueryRow(`SELECT id FROM words WHERE text = 'um (P)' AND language = 'de' AND user_id = 2`).Scan(&umID); err != nil {
		t.Fatalf("find um (P): %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sm2_progress WHERE word_id = ?`, umID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("want sm2_progress row for new word 'um (P)'")
	}
	var wantRank, gotRank sql.NullInt64
	if err := db.QueryRow(`SELECT rank FROM word_frequency_lang WHERE word = 'wegen' AND lang = 'de'`).Scan(&wantRank); err != nil {
		t.Fatalf("frequency rank for wegen: %v", err)
	}
	if err := db.QueryRow(`SELECT rank FROM translations WHERE translation_word_id = 5 AND zh_word_id = 1`).Scan(&gotRank); err != nil {
		t.Fatal(err)
	}
	if gotRank != wantRank {
		t.Errorf("rank for wegen = %v, want %v", gotRank, wantRank)
	}
}
