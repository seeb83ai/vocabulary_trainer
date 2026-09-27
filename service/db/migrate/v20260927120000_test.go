package migrate

import (
	"database/sql"
	"sort"
	"testing"
)

// tagsOfWord returns the sorted tag names linked to wordID.
func tagsOfWord(t *testing.T, db *sql.DB, wordID int64) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT tg.name FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id
		WHERE wt.word_id = ?`, wordID)
	if err != nil {
		t.Fatalf("query tags: %v", err)
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRenameHSKTags(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20260925190000)

	if _, err := db.Exec(`INSERT OR IGNORE INTO users (id, email, password_hash) VALUES (1, 'lib@x', ''), (2, 'u@x', '')`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, text, language, user_id) VALUES
		(1, '爱', 'zh', 1),
		(2, '爱', 'zh', 2),
		(3, '我爱你', 'zh', 2),
		(4, '书', 'zh', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tags (id, name, description, importable) VALUES
		(10, 'hsk1', 'HSK 1', 1),
		(11, 's_hsk1', '', 0),
		(12, 'HSK1', '', 0),
		(13, 'hsk6', '', 1),
		(14, 'hsk2-6', '', 0)`); err != nil {
		t.Fatalf("insert tags: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO word_tags (word_id, tag_id) VALUES
		(1, 10), (2, 10), (3, 11), (4, 12), (4, 13), (2, 14), (2, 13)`); err != nil {
		t.Fatalf("insert word_tags: %v", err)
	}
	if _, err := db.Exec(`UPDATE user_settings SET train_tags = '["hsk1","food","HSK1","s_hsk1","hsk6","hsk2-6"]' WHERE user_id = 2`); err != nil {
		t.Fatalf("insert settings: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	cases := []struct {
		wordID int64
		want   []string
	}{
		{1, []string{"hsk2-1"}},
		{2, []string{"hsk2-1", "hsk2-6"}},
		{3, []string{"s_hsk2-1"}},
		{4, []string{"HSK1", "hsk2-6"}},
	}
	for _, c := range cases {
		if got := tagsOfWord(t, db, c.wordID); !equalStrings(got, c.want) {
			t.Errorf("tags of word %d = %q, want %q", c.wordID, got, c.want)
		}
	}

	// Renamed tag keeps its metadata.
	var desc string
	var imp int
	if err := db.QueryRow(`SELECT description, importable FROM tags WHERE name = 'hsk2-1'`).Scan(&desc, &imp); err != nil {
		t.Fatalf("read hsk2-1: %v", err)
	}
	if desc != "HSK 1" || imp != 1 {
		t.Errorf("hsk2-1 meta = (%q, %d), want (\"HSK 1\", 1)", desc, imp)
	}

	// Merged tags leave exactly one row, importable if any source was.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*), MAX(importable) FROM tags WHERE name = 'hsk2-6'`).Scan(&n, &imp); err != nil {
		t.Fatalf("count hsk2-6: %v", err)
	}
	if n != 1 || imp != 1 {
		t.Errorf("hsk2-6 rows = %d importable = %d, want 1 row importable", n, imp)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name IN ('hsk1','hsk6','s_hsk1')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("old tag rows left: %d", n)
	}

	var trainTags string
	if err := db.QueryRow(`SELECT train_tags FROM user_settings WHERE user_id = 2`).Scan(&trainTags); err != nil {
		t.Fatalf("read train_tags: %v", err)
	}
	if want := `["hsk2-1","food","HSK1","s_hsk2-1","hsk2-6"]`; trainTags != want {
		t.Errorf("train_tags = %s, want %s", trainTags, want)
	}
}
