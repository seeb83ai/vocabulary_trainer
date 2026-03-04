// cmd/import/main.go — Import vocabulary from a text file into the trainer DB.
//
// File format (3 lines per entry, blank lines ignored):
//   Line 1: pinyin / 汉字   OR   汉字 / pinyin   OR   just pinyin (no Chinese)
//   Line 2: translation(s), comma-separated
//   Line 3: rating string (ignored, e.g. "Schon ziemlich gut")
//
// Usage:
//   go run ./cmd/import -db data/vocab.db -file voc.txt [-dry-run]
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	filePath := flag.String("file", "voc.txt", "path to vocabulary text file")
	dryRun := flag.Bool("dry-run", false, "parse and check duplicates but do not insert")
	flag.Parse()

	entries, err := parseFile(*filePath)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}
	fmt.Printf("Parsed %d entries from %s\n", len(entries), *filePath)

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)", *dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := vocabdb.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	var inserted, skipped, failed int
	for _, e := range entries {
		dup, err := isDuplicate(db, e)
		if err != nil {
			fmt.Printf("  [ERROR] check duplicate %q / %q: %v\n", e.pinyin, e.translation, err)
			failed++
			continue
		}
		if dup {
			fmt.Printf("  [SKIP]  %s / %s — %s\n", e.pinyin, e.hanzi, e.translation)
			skipped++
			continue
		}
		if *dryRun {
			fmt.Printf("  [DRY]   would insert: %s / %s — %s\n", e.pinyin, e.hanzi, e.translation)
			inserted++
			continue
		}
		if err := insert(db, e); err != nil {
			fmt.Printf("  [ERROR] insert %q: %v\n", e.pinyin, err)
			failed++
			continue
		}
		fmt.Printf("  [OK]    %s / %s — %s\n", e.pinyin, e.hanzi, e.translation)
		inserted++
	}

	fmt.Printf("\nDone. inserted=%d  skipped=%d  errors=%d\n", inserted, skipped, failed)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

// entry holds one parsed vocabulary item.
type entry struct {
	pinyin      string
	hanzi       string // may be empty
	translation string // the non-Chinese side (e.g. German/English)
}

// parseFile reads the vocabulary text file and returns all valid entries.
// Format: 3 lines per entry (blank lines and the rating line are skipped).
func parseFile(path string) ([]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" {
			lines = append(lines, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// Entries are in groups of 3: word line, translation, rating
	var entries []entry
	for i := 0; i+2 < len(lines); i += 3 {
		wordLine := lines[i]
		translation := strings.TrimSpace(lines[i+1])
		// lines[i+2] is the rating — ignored

		pinyin, hanzi := splitWordLine(wordLine)
		if pinyin == "" && hanzi == "" {
			fmt.Printf("  [WARN] could not parse line %d: %q\n", i+1, wordLine)
			continue
		}
		entries = append(entries, entry{
			pinyin:      pinyin,
			hanzi:       hanzi,
			translation: translation,
		})
	}
	return entries, nil
}

// splitWordLine splits "pinyin / 汉字" or "汉字 / pinyin" into its parts.
// If no "/" is present the whole string is treated as pinyin-only.
func splitWordLine(line string) (pinyin, hanzi string) {
	parts := strings.SplitN(line, "/", 2)
	if len(parts) == 1 {
		// No slash — the whole thing might be pinyin or a hanzi phrase.
		// Heuristic: if the string contains Chinese characters it's hanzi-only.
		if containsChinese(line) {
			return "", strings.TrimSpace(line)
		}
		return strings.TrimSpace(line), ""
	}
	a := strings.TrimSpace(parts[0])
	b := strings.TrimSpace(parts[1])
	// Determine which side is hanzi
	if containsChinese(a) {
		return b, a
	}
	return a, b
}

// containsChinese reports whether s contains any CJK character.
func containsChinese(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// applySchema creates the tables if they don't exist yet.

// isDuplicate returns true if a word with the same hanzi (or pinyin when no
// hanzi) AND the same translation already exists in the DB.
func isDuplicate(db *sql.DB, e entry) (bool, error) {
	// Determine the zh word text to look for
	zhText := e.hanzi
	if zhText == "" {
		zhText = e.pinyin // pinyin-only entries stored as zh text
	}
	enText := e.translation

	// Check: zh word with this text AND linked to en word with this text
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM words zh
		JOIN translations t ON t.zh_word_id = zh.id
		JOIN words en       ON t.en_word_id = en.id
		WHERE zh.language = 'zh'
		  AND lower(trim(zh.text))  = lower(trim(?))
		  AND lower(trim(en.text))  = lower(trim(?))`,
		zhText, enText).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Also check by pinyin when we have both pinyin and hanzi
	if e.pinyin != "" && e.hanzi != "" {
		err = db.QueryRow(`
			SELECT COUNT(*)
			FROM words zh
			JOIN translations t ON t.zh_word_id = zh.id
			JOIN words en       ON t.en_word_id = en.id
			WHERE zh.language = 'zh'
			  AND lower(trim(zh.pinyin)) = lower(trim(?))
			  AND lower(trim(en.text))   = lower(trim(?))`,
			e.pinyin, enText).Scan(&count)
		if err != nil {
			return false, err
		}
	}
	return count > 0, nil
}

// insert adds one vocabulary entry inside a transaction.
func insert(db *sql.DB, e entry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	zhText := e.hanzi
	if zhText == "" {
		zhText = e.pinyin
	}
	var pinyin *string
	if e.pinyin != "" && e.hanzi != "" {
		pinyin = &e.pinyin
	}

	// Upsert zh word
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO words (text, language, pinyin) VALUES (?, 'zh', ?)`,
		zhText, pinyin); err != nil {
		return fmt.Errorf("insert zh word: %w", err)
	}
	var zhID int64
	if err := tx.QueryRow(
		`SELECT id FROM words WHERE text = ? AND language = 'zh'`, zhText).Scan(&zhID); err != nil {
		return fmt.Errorf("get zh id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, zhID); err != nil {
		return fmt.Errorf("init zh sm2: %w", err)
	}

	// Upsert en word (translation)
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO words (text, language) VALUES (?, 'en')`, e.translation); err != nil {
		return fmt.Errorf("insert en word: %w", err)
	}
	var enID int64
	if err := tx.QueryRow(
		`SELECT id FROM words WHERE text = ? AND language = 'en'`, e.translation).Scan(&enID); err != nil {
		return fmt.Errorf("get en id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, enID); err != nil {
		return fmt.Errorf("init en sm2: %w", err)
	}

	// Link them
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
		enID, zhID); err != nil {
		return fmt.Errorf("link translation: %w", err)
	}

	return tx.Commit()
}
