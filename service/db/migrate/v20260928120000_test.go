package migrate

import "testing"

func TestDeleteTagA1(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20260927120000)

	if _, err := db.Exec(`INSERT OR IGNORE INTO users (id, email, password_hash) VALUES (1, 'lib@x', ''), (2, 'u@x', '')`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '爱', 'zh', 1),
		(2, '书', 'zh', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tags (id, name) VALUES (10, 'a1'), (11, 'A1'), (12, 'hsk2-1'), (13, 'a10')`); err != nil {
		t.Fatalf("insert tags: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO word_tags (word_id, tag_id) VALUES (1, 10), (1, 12), (2, 10), (2, 11), (2, 13)`); err != nil {
		t.Fatalf("insert word_tags: %v", err)
	}
	if _, err := db.Exec(`UPDATE user_settings SET train_tags = '["a1","food","A1","a1","a10"]' WHERE user_id = 2`); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'a1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("tag a1 rows left: %d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM word_tags WHERE tag_id = 10`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("word_tags links to a1 left: %d", n)
	}

	// Words stay; other tags (also similar names) stay.
	if got := tagsOfWord(t, db, 1); !equalStrings(got, []string{"hsk2-1"}) {
		t.Errorf("tags of word 1 = %q, want [hsk2-1]", got)
	}
	if got := tagsOfWord(t, db, 2); !equalStrings(got, []string{"A1", "a10"}) {
		t.Errorf("tags of word 2 = %q, want [A1 a10]", got)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM words`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("words = %d, want 2", n)
	}

	var trainTags string
	if err := db.QueryRow(`SELECT train_tags FROM user_settings WHERE user_id = 2`).Scan(&trainTags); err != nil {
		t.Fatal(err)
	}
	if want := `["food","A1","a10"]`; trainTags != want {
		t.Errorf("train_tags = %s, want %s", trainTags, want)
	}
}
