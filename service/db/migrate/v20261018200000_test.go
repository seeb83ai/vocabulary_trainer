package migrate

import (
	"database/sql"
	"sort"
	"strings"
	"testing"
)

// glossesOf returns the "lang:text" glosses linked to a zh word, sorted.
func glossesOf(t *testing.T, db *sql.DB, wordID int64) []string {
	t.Helper()
	rows, err := db.Query(`
		SELECT tw.language || ':' || tw.text FROM translations tr
		JOIN words tw ON tw.id = tr.translation_word_id
		WHERE tr.zh_word_id = ? ORDER BY 1`, wordID)
	if err != nil {
		t.Fatalf("query glosses: %v", err)
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
	return out
}

func libraryWordID(t *testing.T, db *sql.DB, text string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM words WHERE text = ? AND language = 'zh' AND user_id = 1`, text).Scan(&id); err != nil {
		t.Fatalf("library word %q: %v", text, err)
	}
	return id
}

func sortedTags(t *testing.T, db *sql.DB, wordID int64) []string {
	t.Helper()
	tags := tagsOfWord(t, db, wordID)
	sort.Strings(tags)
	return tags
}

func TestAddGrammarExamples(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261014190000)

	// 我们 is already in the library with a dictionary gloss; 我吃。 belongs
	// to a learner only.
	if _, err := db.Exec(`INSERT INTO words (id, text, language, pinyin, user_id) VALUES
		(100, '我们', 'zh', 'wǒmen', 1),
		(101, 'we (dict)', 'en', NULL, 1),
		(102, '我吃。', 'zh', 'Wǒ chī.', 2)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source) VALUES (101, 100, 'cedict')`); err != nil {
		t.Fatalf("insert translation: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// A new sentence: page pinyin, EN + DE glosses, both tags.
	id := libraryWordID(t, db, "我吃。")
	var pinyin string
	if err := db.QueryRow(`SELECT pinyin FROM words WHERE id = ?`, id).Scan(&pinyin); err != nil {
		t.Fatal(err)
	}
	if pinyin != "Wǒ chī." {
		t.Errorf("pinyin of 我吃。 = %q, want %q", pinyin, "Wǒ chī.")
	}
	if got, want := glossesOf(t, db, id), []string{"de:Ich esse.", "en:I eat."}; !equalStrings(got, want) {
		t.Errorf("glosses of 我吃。 = %q, want %q", got, want)
	}
	if got, want := sortedTags(t, db, id), []string{"grammar", "grammar_s"}; !equalStrings(got, want) {
		t.Errorf("tags of 我吃。 = %q, want %q", got, want)
	}
	var source string
	var rank sql.NullInt64
	if err := db.QueryRow(`SELECT source, rank FROM translations WHERE zh_word_id = ? LIMIT 1`, id).Scan(&source, &rank); err != nil {
		t.Fatal(err)
	}
	if source != "user" || rank.Valid {
		t.Errorf("gloss source/rank = %q/%v, want user/NULL", source, rank)
	}

	// A new word: pinyin without spaces, only the grammar tag, split glosses.
	id = libraryWordID(t, db, "我的")
	if err := db.QueryRow(`SELECT pinyin FROM words WHERE id = ?`, id).Scan(&pinyin); err != nil {
		t.Fatal(err)
	}
	if pinyin != "wǒde" {
		t.Errorf("pinyin of 我的 = %q, want %q", pinyin, "wǒde")
	}
	if got, want := glossesOf(t, db, id), []string{"de:mein", "de:meins", "en:mine", "en:my"}; !equalStrings(got, want) {
		t.Errorf("glosses of 我的 = %q, want %q", got, want)
	}
	if got, want := sortedTags(t, db, id), []string{"grammar"}; !equalStrings(got, want) {
		t.Errorf("tags of 我的 = %q, want %q", got, want)
	}

	// An existing library word only gets the tag.
	if got, want := sortedTags(t, db, 100), []string{"grammar"}; !equalStrings(got, want) {
		t.Errorf("tags of 我们 = %q, want %q", got, want)
	}
	if got, want := glossesOf(t, db, 100), []string{"en:we (dict)"}; !equalStrings(got, want) {
		t.Errorf("glosses of 我们 = %q, want %q", got, want)
	}

	// The learner's own word stays untouched.
	if got := tagsOfWord(t, db, 102); len(got) != 0 {
		t.Errorf("tags of learner word = %q, want none", got)
	}

	// Both tags are importable lists.
	for _, name := range []string{"grammar", "grammar_s"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = ? AND importable = 1 AND description != ''`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("importable %s tags with description = %d, want 1", name, n)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id WHERE tg.name = 'grammar'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(grammarExamples) {
		t.Errorf("words tagged grammar = %d, want %d", n, len(grammarExamples))
	}
}

// seedLibraryGloss gives the library one word with a dictionary gloss, so the
// migration sees a filled library.
func seedLibraryGloss(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO words (id, text, language, pinyin, user_id) VALUES
		(100, '书', 'zh', 'shū', 1),
		(101, 'book', 'en', NULL, 1)`); err != nil {
		t.Fatalf("insert words: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO translations (translation_word_id, zh_word_id, source) VALUES (101, 100, 'cedict')`); err != nil {
		t.Fatalf("insert translation: %v", err)
	}
}

func TestAddGrammarExamplesRunsTwice(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261014190000)
	seedLibraryGloss(t, db)
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	count := func() (words, links, tags int) {
		t.Helper()
		if err := db.QueryRow(`SELECT
			(SELECT COUNT(*) FROM words),
			(SELECT COUNT(*) FROM translations),
			(SELECT COUNT(*) FROM word_tags)`).Scan(&words, &links, &tags); err != nil {
			t.Fatal(err)
		}
		return
	}
	w1, l1, t1 := count()
	if t1 != len(grammarExamples)+35 {
		t.Fatalf("word tags after first run = %d, want %d", t1, len(grammarExamples)+35)
	}
	if err := addGrammarExamples(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if w2, l2, t2 := count(); w1 != w2 || l1 != l2 || t1 != t2 {
		t.Errorf("second run changed counts: words %d→%d, links %d→%d, tags %d→%d", w1, w2, l1, l2, t1, t2)
	}
}

// Test, fresh and not yet filled databases skip the migration: their library
// has no glosses.
func TestAddGrammarExamplesSkipsLibraryWithoutGlosses(t *testing.T) {
	db := openRawDB(t)
	migrateUpTo(t, db, 20261014190000)
	if _, err := db.Exec(`INSERT INTO words (text, language, pinyin, user_id) VALUES ('书', 'zh', 'shū', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var words, tags int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM words), (SELECT COUNT(*) FROM tags WHERE name LIKE 'grammar%')`).Scan(&words, &tags); err != nil {
		t.Fatal(err)
	}
	if words != 1 || tags != 0 {
		t.Errorf("words = %d, grammar tags = %d, want 1 and 0", words, tags)
	}
}

func TestGrammarExamplesData(t *testing.T) {
	seen := map[string]bool{}
	sentences := 0
	for _, e := range grammarExamples {
		if seen[e.zh] {
			t.Errorf("%s occurs twice", e.zh)
		}
		seen[e.zh] = true
		if e.pinyin == "" || len(e.en) == 0 || len(e.de) == 0 {
			t.Errorf("%s: needs pinyin, en and de", e.zh)
		}
		for _, g := range append(append([]string{}, e.en...), e.de...) {
			if g == "" {
				t.Errorf("%s: empty gloss", e.zh)
			}
			// CheckAnswer splits a gloss at "," and "/" and accepts each part.
			if strings.ContainsAny(g, ",/") {
				t.Errorf("%s: gloss %q contains , or /", e.zh, g)
			}
		}
		if e.sentence {
			sentences++
		}
	}
	if sentences == 0 || sentences == len(grammarExamples) {
		t.Errorf("sentences = %d of %d, want some but not all", sentences, len(grammarExamples))
	}
}
