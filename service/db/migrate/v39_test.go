package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openV39TestDB creates an in-memory SQLite DB with the minimum schema
// required to exercise the v39 migration: users, words, sm2_progress,
// hanzi_decomposition (without the new pinyin column yet) and
// component_progress.
func openV39TestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	schema := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`,
		`CREATE TABLE words (
			id INTEGER PRIMARY KEY,
			user_id INTEGER,
			text TEXT,
			language TEXT
		)`,
		`CREATE TABLE sm2_progress (
			word_id INTEGER,
			due_date TEXT,
			first_seen_date TEXT
		)`,
		`CREATE TABLE hanzi_decomposition (
			character TEXT PRIMARY KEY,
			definition TEXT,
			radical TEXT,
			decomposition TEXT,
			etymology TEXT
		)`,
		`CREATE TABLE component_progress (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			character TEXT NOT NULL,
			due_date TEXT NOT NULL,
			repetitions INTEGER NOT NULL DEFAULT 0,
			easiness REAL NOT NULL DEFAULT 2.5,
			interval_days INTEGER NOT NULL DEFAULT 1,
			total_correct INTEGER NOT NULL DEFAULT 0,
			total_attempts INTEGER NOT NULL DEFAULT 0,
			first_seen_date TEXT,
			UNIQUE(user_id, character)
		)`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

func pinyinColumnExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(hanzi_decomposition)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "pinyin" {
			return true
		}
	}
	return false
}

func TestV39_AddsPinyinColumn(t *testing.T) {
	db := openV39TestDB(t)
	if pinyinColumnExists(t, db) {
		t.Fatalf("precondition: pinyin column should not exist yet")
	}
	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39: %v", err)
	}
	if !pinyinColumnExists(t, db) {
		t.Errorf("want pinyin column after v39 run")
	}
}

func TestV39_AddPinyinColumn_Idempotent(t *testing.T) {
	db := openV39TestDB(t)
	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39 first run: %v", err)
	}
	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39 second run should be idempotent: %v", err)
	}
}

func TestV39_WipesAndRebackfills_DropsPhoneticOnly(t *testing.T) {
	db := openV39TestDB(t)

	// Seed parent 妈 (pictophonetic, phonetic=马, semantic=女, radical=女).
	ety := `{"type":"pictophonetic","phonetic":"马","semantic":"女","hint":"mother"}`
	if _, err := db.Exec(`INSERT INTO hanzi_decomposition (character, definition, radical, decomposition, etymology)
		VALUES ('妈', 'mother', '女', '⿰女马', ?)`, ety); err != nil {
		t.Fatalf("seed 妈: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO hanzi_decomposition (character, definition) VALUES ('女', 'woman')`); err != nil {
		t.Fatalf("seed 女: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO hanzi_decomposition (character, definition) VALUES ('马', 'horse')`); err != nil {
		t.Fatalf("seed 马: %v", err)
	}

	// Seed a user learning 妈.
	if _, err := db.Exec(`INSERT INTO users (id, email) VALUES (1, 'a@b.c')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO words (id, user_id, text, language) VALUES (10, 1, '妈', 'zh')`); err != nil {
		t.Fatalf("seed word: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, due_date, first_seen_date) VALUES (10, '2026-04-20 00:00:00', '2026-04-15')`); err != nil {
		t.Fatalf("seed sm2: %v", err)
	}

	// Pre-populate component_progress with the stale (pre-filter) rows.
	if _, err := db.Exec(`INSERT INTO component_progress (user_id, character, due_date, repetitions)
		VALUES (1, '女', '2026-04-20 00:00:00', 7), (1, '马', '2026-04-20 00:00:00', 3)`); err != nil {
		t.Fatalf("seed component_progress: %v", err)
	}

	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39: %v", err)
	}

	var chars []string
	rows, err := db.Query(`SELECT character FROM component_progress WHERE user_id = 1 ORDER BY character`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		chars = append(chars, c)
	}
	rows.Close()

	if len(chars) != 1 || chars[0] != "女" {
		t.Errorf("want [女] only (马 is phonetic-only), got %v", chars)
	}

	// Pre-existing SM-2 repetitions are wiped by the "wipe and re-backfill" strategy.
	var reps int
	if err := db.QueryRow(`SELECT repetitions FROM component_progress WHERE user_id = 1 AND character = '女'`).Scan(&reps); err != nil {
		t.Fatalf("query repetitions: %v", err)
	}
	if reps != 0 {
		t.Errorf("want repetitions reset to 0 after wipe, got %d", reps)
	}
}

func TestV39_SpreadsDueDates_Max5PerDay(t *testing.T) {
	db := openV39TestDB(t)

	// Seed a user.
	if _, err := db.Exec(`INSERT INTO users (id, email) VALUES (1, 'a@b.c')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Seed 7 ideographic parent characters (each yields 1 semantic component)
	// all due on the same day. After backfill spreadComponentDueDates should
	// push the overflow (2) to the next day.
	ety := `{"type":"ideographic"}`
	for i := 0; i < 7; i++ {
		parent := string(rune('A' + i))       // placeholder non-Han; skipped below
		_ = parent
	}
	// Actually we need Han characters so the backfill iterates them. Use CJK
	// runes that round-trip through unicode.Is(unicode.Han, r).
	parents := []rune{'一', '二', '三', '四', '五', '六', '七'}
	for i, r := range parents {
		// Parent's decomposition contains a single component 日 (which has a
		// definition so the filter keeps it), and an unrelated second rune
		// which has no definition so it is filtered out — ensures each parent
		// contributes exactly one stored component, but they all share the
		// same character "日" so we actually need distinct components.
		// Use distinct dummy Han chars for components.
		comp := rune('甲' + i) // 甲乙丙丁戊己庚 — all Han, simple
		decomp := string([]rune{0x2FF0, r, comp})
		if _, err := db.Exec(`INSERT INTO hanzi_decomposition (character, decomposition, etymology) VALUES (?, ?, ?)`,
			string(r), decomp, ety); err != nil {
			t.Fatalf("seed parent %q: %v", string(r), err)
		}
		if _, err := db.Exec(`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)`,
			string(comp), "comp def"); err != nil {
			t.Fatalf("seed comp %q: %v", string(comp), err)
		}
		wordID := int64(100 + i)
		if _, err := db.Exec(`INSERT INTO words (id, user_id, text, language) VALUES (?, 1, ?, 'zh')`, wordID, string(r)); err != nil {
			t.Fatalf("seed word: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO sm2_progress (word_id, due_date, first_seen_date) VALUES (?, '2026-04-20 00:00:00', '2026-04-15')`, wordID); err != nil {
			t.Fatalf("seed sm2: %v", err)
		}
	}

	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39: %v", err)
	}

	counts := map[string]int{}
	rows, err := db.Query(`SELECT due_date FROM component_progress WHERE user_id = 1`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(d) >= 10 {
			counts[d[:10]]++
		}
	}
	rows.Close()

	if counts["2026-04-20"] != 5 {
		t.Errorf("want 5 on 2026-04-20, got %d (counts=%v)", counts["2026-04-20"], counts)
	}
	if counts["2026-04-21"] != 2 {
		t.Errorf("want 2 on 2026-04-21, got %d (counts=%v)", counts["2026-04-21"], counts)
	}
}

func TestV39_EmptyComponentProgress_NoOp(t *testing.T) {
	db := openV39TestDB(t)
	if err := rebackfillComponentsV39(db); err != nil {
		t.Fatalf("v39 on empty: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM component_progress`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 rows, got %d", count)
	}
}
