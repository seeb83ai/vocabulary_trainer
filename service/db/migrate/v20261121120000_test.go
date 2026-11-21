package migrate

import (
	"database/sql"
	"testing"
)

func deFrequencyRank(t *testing.T, db *sql.DB, word string) sql.NullInt64 {
	t.Helper()
	var rank sql.NullInt64
	err := db.QueryRow(`SELECT rank FROM word_frequency_lang WHERE lang = 'de' AND word = ?`, word).Scan(&rank)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("select rank %q: %v", word, err)
	}
	return rank
}

// TestMigrate_FrequencyListCoversLessCommonWords: the bundled lists go past
// the top 8,000, so 借's "verleihen" ranks ahead of the rarer "ausborgen"
// instead of both being unranked (issue #658).
func TestMigrate_FrequencyListCoversLessCommonWords(t *testing.T) {
	db := openRawDB(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	verleihen, ausborgen := deFrequencyRank(t, db, "verleihen"), deFrequencyRank(t, db, "ausborgen")
	if !verleihen.Valid || !ausborgen.Valid {
		t.Fatalf("verleihen = %+v, ausborgen = %+v, want both ranked", verleihen, ausborgen)
	}
	if verleihen.Int64 >= ausborgen.Int64 {
		t.Errorf("verleihen rank %d, ausborgen rank %d, want verleihen more frequent", verleihen.Int64, ausborgen.Int64)
	}
}

// TestMigrate_ReranksTranslationsWithLongerFrequencyList: a database migrated
// with the old 8,000-word lists gets the longer lists, and the stored rank of
// every dictionary gloss and the gloss_rank cache are computed again, without
// the words in parentheses (issue #658). User translations keep rank 0.
func TestMigrate_ReranksTranslationsWithLongerFrequencyList(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261121115959)
	if _, err := db.Exec(`DELETE FROM word_frequency_lang WHERE lang IN ('en', 'de') AND rank > 8000`); err != nil {
		t.Fatalf("shorten frequency list: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO cedict_entries (simplified, lang, pinyin, definition) VALUES
		('借', 'de', 'jiè', 'ausborgen (V); verleihen (V)')`); err != nil {
		t.Fatalf("insert cedict: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '借', 'zh', 2),
		(2, 'ausborgen (V)', 'de', 2),
		(3, 'verleihen (V)', 'de', 2),
		(4, 'ausleihen', 'de', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source, rank) VALUES
		(2, 1, 'cedict', 6418),
		(3, 1, 'cedict', 6418),
		(4, 1, 'user', 0)`); err != nil {
		t.Fatalf("insert translations: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	verleihen, ausborgen := deFrequencyRank(t, db, "verleihen"), deFrequencyRank(t, db, "ausborgen")
	if !verleihen.Valid || !ausborgen.Valid {
		t.Fatalf("verleihen = %+v, ausborgen = %+v, want both ranked", verleihen, ausborgen)
	}
	for _, c := range []struct {
		wordID int64
		want   sql.NullInt64
	}{
		{2, ausborgen},
		{3, verleihen},
		{4, sql.NullInt64{Int64: 0, Valid: true}},
	} {
		var got sql.NullInt64
		if err := db.QueryRow(`SELECT rank FROM translations WHERE zh_word_id = 1 AND translation_word_id = ?`, c.wordID).Scan(&got); err != nil {
			t.Fatalf("select translation rank: %v", err)
		}
		if got != c.want {
			t.Errorf("translation %d rank = %+v, want %+v", c.wordID, got, c.want)
		}
	}
	for gloss, want := range map[string]sql.NullInt64{"ausborgen (V)": ausborgen, "verleihen (V)": verleihen} {
		var got sql.NullInt64
		if err := db.QueryRow(`SELECT rank FROM gloss_rank WHERE lang = 'de' AND gloss = ?`, gloss).Scan(&got); err != nil {
			t.Fatalf("select gloss_rank %q: %v", gloss, err)
		}
		if got != want {
			t.Errorf("gloss_rank %q = %+v, want %+v", gloss, got, want)
		}
	}
}
