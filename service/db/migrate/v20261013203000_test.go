package migrate

import "testing"

func TestRenameSentenceTagsToSuffix(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261012120000)

	if _, err := db.Exec(`INSERT OR IGNORE INTO users (id, email, password_hash) VALUES (1, 'lib@x', ''), (2, 'u@x', '')`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '我爱你', 'zh', 2),
		(2, '我买牛奶', 'zh', 2),
		(3, '你好吗', 'zh', 2),
		(4, '书', 'zh', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tags (id, name, description, importable) VALUES
		(10, 's_hsk2-1', 'HSK 1 sentences', 0),
		(11, 's_hsk2-2', '', 0),
		(12, 'hsk2-2_s', '', 1),
		(13, 'S_food', '', 0),
		(14, 'class_s1', '', 0),
		(15, 's_', '', 0)`); err != nil {
		t.Fatalf("insert tags: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO word_tags (word_id, tag_id) VALUES
		(1, 10), (2, 11), (2, 12), (3, 13), (4, 14), (4, 15)`); err != nil {
		t.Fatalf("insert word_tags: %v", err)
	}
	if _, err := db.Exec(`UPDATE user_settings SET train_tags = '["s_hsk2-1","hsk2-1","s_hsk2-2","hsk2-2_s","S_food"]' WHERE user_id = 2`); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cases := []struct {
		wordID int64
		want   []string
	}{
		{1, []string{"hsk2-1_s"}},
		{2, []string{"hsk2-2_s"}},
		{3, []string{"food_s"}},
		{4, []string{"class_s1", "s_"}},
	}
	for _, c := range cases {
		if got := tagsOfWord(t, db, c.wordID); !equalStrings(got, c.want) {
			t.Errorf("tags of word %d = %q, want %q", c.wordID, got, c.want)
		}
	}

	var desc string
	if err := db.QueryRow(`SELECT description FROM tags WHERE name = 'hsk2-1_s'`).Scan(&desc); err != nil {
		t.Fatalf("read hsk2-1_s: %v", err)
	}
	if desc != "HSK 1 sentences" {
		t.Errorf("hsk2-1_s description = %q, want %q", desc, "HSK 1 sentences")
	}

	var n, imp int
	if err := db.QueryRow(`SELECT COUNT(*), MAX(importable) FROM tags WHERE name = 'hsk2-2_s'`).Scan(&n, &imp); err != nil {
		t.Fatalf("count hsk2-2_s: %v", err)
	}
	if n != 1 || imp != 1 {
		t.Errorf("hsk2-2_s rows = %d importable = %d, want 1 row importable", n, imp)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name IN ('s_hsk2-1','s_hsk2-2','S_food')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("old tag rows left: %d", n)
	}

	var trainTags string
	if err := db.QueryRow(`SELECT train_tags FROM user_settings WHERE user_id = 2`).Scan(&trainTags); err != nil {
		t.Fatalf("read train_tags: %v", err)
	}
	if want := `["hsk2-1_s","hsk2-1","hsk2-2_s","food_s"]`; trainTags != want {
		t.Errorf("train_tags = %s, want %s", trainTags, want)
	}
}
