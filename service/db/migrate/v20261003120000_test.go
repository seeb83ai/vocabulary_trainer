package migrate

import "testing"

func TestRejoinExampleSentenceTranslations(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261001220000)

	whole := "Bsp.: 我帮助你学习汉语。 -- Ich helfe dir, Chinesisch zu lernen."
	if _, err := db.Exec(`INSERT INTO cedict_entries (simplified, lang, pinyin, definition) VALUES
		('帮助', 'de', 'bāng zhù', 'helfen; ` + whole + `; Hilfe, Unterstützung'),
		('例', 'de', 'lì', 'Bsp.: 例 -- eins, zwei; zwei')`); err != nil {
		t.Fatalf("insert cedict: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '帮助', 'zh', 2),
		(2, 'helfen', 'de', 2),
		(3, 'Bsp.: 我帮助你学习汉语。 -- Ich helfe dir', 'de', 2),
		(4, 'Chinesisch zu lernen.', 'de', 2),
		(5, 'Hilfe', 'de', 2),
		(6, 'Unterstützung', 'de', 2),
		(7, '学', 'zh', 2),
		(8, '例', 'zh', 2),
		(9, 'Bsp.: 例 -- eins', 'de', 2),
		(10, 'zwei', 'de', 2),
		(11, '帮助', 'zh', 1),
		(12, 'Bsp.: 我帮助你学习汉语。 -- Ich helfe dir', 'de', 1),
		(13, 'Chinesisch zu lernen.', 'de', 1)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id) VALUES (1), (2), (3), (4), (5), (6), (7), (8), (9), (10), (11), (12), (13)`); err != nil {
		t.Fatalf("insert progress: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source, rank) VALUES
		(2, 1, 'cedict', NULL),
		(3, 1, 'cedict', NULL),
		(4, 1, 'cedict', NULL),
		(4, 7, 'user', 0),
		(5, 1, 'cedict', NULL),
		(6, 1, 'cedict', NULL),
		(9, 8, 'cedict', NULL),
		(10, 8, 'cedict', NULL),
		(12, 11, 'cedict', NULL),
		(13, 11, 'cedict', NULL)`); err != nil {
		t.Fatalf("insert translations: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cases := []struct {
		name string
		zhID int64
		want []string
	}{
		{"example rejoined", 1, []string{whole + "|cedict", "Hilfe|cedict", "Unterstützung|cedict", "helfen|cedict"}},
		{"other user's example rejoined", 11, []string{whole + "|cedict"}},
		{"fragment that is also its own sense stays", 8, []string{"Bsp.: 例 -- eins, zwei|cedict", "zwei|cedict"}},
		{"user translation untouched", 7, []string{"Chinesisch zu lernen.|user"}},
	}
	for _, c := range cases {
		got := deLinksFor(t, db, c.zhID)
		if len(got) != len(c.want) {
			t.Errorf("%s: links = %q, want %q", c.name, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: links[%d] = %q, want %q", c.name, i, got[i], c.want[i])
			}
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE id IN (3, 12, 13)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("want orphaned fragment words 3, 12, 13 deleted, %d left", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE id = 4`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Error("want fragment word 4 kept, it is still linked as a user translation")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM gloss_rank WHERE gloss = ?`, whole).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want the whole example sentence in gloss_rank, got %d rows", n)
	}

	// A second run changes nothing.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 20261003120000`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	got := deLinksFor(t, db, 1)
	if len(got) != 4 {
		t.Errorf("links after second run = %q, want 4", got)
	}
}
