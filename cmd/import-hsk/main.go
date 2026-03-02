// cmd/import-hsk/main.go — Import HSK vocabulary from mandarinbean.com into the trainer DB.
//
// Fetches https://mandarinbean.com/hsk-N-vocabulary-list/ for each requested level,
// parses the vocabulary table, and inserts new zh/en word pairs. Duplicate pairs
// (same zh text + same translation) are skipped; the HSK tag is still attached to
// already-existing words. Each zh word is tagged with "hsk-N".
//
// Optionally translates the English source text via the DeepL API before inserting.
// Set DEEPL_API_KEY in the environment and pass -lang <code> (e.g. -lang de).
// If the key is absent, translation is silently skipped and the original English is used.
//
// Usage:
//
//	go run ./cmd/import-hsk [-db data/vocab.db] [-levels 1,2,3,4,5,6] [-lang en] [-dry-run]
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

const urlPattern = "https://mandarinbean.com/hsk-%d-vocabulary-list/"

type entry struct {
	hanzi       string
	pinyin      string
	translation string
}

func main() {
	dbPath    := flag.String("db", "data/vocab.db", "path to SQLite database")
	levelsStr := flag.String("levels", "1,2,3,4,5,6", "comma-separated HSK levels to import (1-6)")
	lang      := flag.String("lang", "en", "DeepL target language code (e.g. de, fr, es); requires DEEPL_API_KEY env var")
	dryRun    := flag.Bool("dry-run", false, "fetch and check duplicates but do not insert")
	flag.Parse()

	var levels []int
	for _, s := range strings.Split(*levelsStr, ",") {
		s = strings.TrimSpace(s)
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 6 {
			log.Fatalf("invalid level %q: must be an integer 1-6", s)
		}
		levels = append(levels, n)
	}

	targetLang := strings.ToUpper(strings.TrimSpace(*lang))
	apiKey     := os.Getenv("DEEPL_API_KEY")
	translate  := targetLang != "EN" && apiKey != ""

	if targetLang != "EN" && apiKey == "" {
		fmt.Println("[WARN]  DEEPL_API_KEY not set — importing with original English translations")
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)", *dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := applySchema(db); err != nil {
		log.Fatalf("schema: %v", err)
	}

	var totalInserted, totalSkipped, totalFailed int

	for _, level := range levels {
		tag := fmt.Sprintf("hsk-%d", level)
		url := fmt.Sprintf(urlPattern, level)
		fmt.Printf("\n=== HSK %d  %s ===\n", level, url)

		body, err := fetchPage(url)
		if err != nil {
			fmt.Printf("  [ERROR] fetch: %v\n", err)
			totalFailed++
			continue
		}

		entries := parseTable(body)
		fmt.Printf("  Parsed %d entries\n", len(entries))
		if len(entries) == 0 {
			fmt.Println("  [WARN]  No entries found — check that the page structure hasn't changed.")
			continue
		}

		if translate {
			fmt.Printf("  Translating to %s via DeepL…\n", targetLang)
			entries, err = translateEntries(entries, targetLang, apiKey)
			if err != nil {
				fmt.Printf("  [ERROR] translation failed: %v\n", err)
				totalFailed++
				continue
			}
		}

		ins, skip, fail := importEntries(db, entries, tag, *dryRun)
		totalInserted += ins
		totalSkipped += skip
		totalFailed += fail
	}

	fmt.Printf("\nDone. inserted=%d  skipped=%d  errors=%d\n", totalInserted, totalSkipped, totalFailed)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

// importEntries inserts all entries for one HSK level and returns counts.
func importEntries(db *sql.DB, entries []entry, tag string, dryRun bool) (inserted, skipped, failed int) {
	for _, e := range entries {
		zhID, dup, err := isDuplicate(db, e)
		if err != nil {
			fmt.Printf("  [ERROR] duplicate check %q: %v\n", e.hanzi, err)
			failed++
			continue
		}

		if dup {
			fmt.Printf("  [SKIP]  %s — %s\n", e.hanzi, e.translation)
			skipped++
			// Still attach the tag to the existing word even though we skip the insert.
			if !dryRun && zhID != 0 {
				if err := assignTag(db, zhID, tag); err != nil {
					fmt.Printf("  [WARN]  tag %q for existing %q: %v\n", tag, e.hanzi, err)
				}
			}
			continue
		}

		if dryRun {
			fmt.Printf("  [DRY]   would insert: %s (%s) — %s\n", e.hanzi, e.pinyin, e.translation)
			inserted++
			continue
		}

		zhID, err = insert(db, e)
		if err != nil {
			fmt.Printf("  [ERROR] insert %q: %v\n", e.hanzi, err)
			failed++
			continue
		}
		if err := assignTag(db, zhID, tag); err != nil {
			fmt.Printf("  [WARN]  tag %q for %q: %v\n", tag, e.hanzi, err)
		}
		fmt.Printf("  [OK]    %s (%s) — %s\n", e.hanzi, e.pinyin, e.translation)
		inserted++
	}
	return
}

// translateEntries replaces the translation field of every entry with the DeepL
// result for the given target language. All texts for the level are sent in
// batches of 50 to minimise round-trips. Returns a new slice; the originals are
// unchanged. Fails fast if any batch returns an error.
func translateEntries(entries []entry, targetLang, apiKey string) ([]entry, error) {
	// Collect all source texts in order.
	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = e.translation
	}

	// Translate in batches of 50 (DeepL's recommended batch size).
	const batchSize = 50
	translated := make([]string, 0, len(texts))
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := deeplTranslate(texts[i:end], targetLang, apiKey)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end-1, err)
		}
		translated = append(translated, batch...)
	}

	// Build result with translated texts.
	result := make([]entry, len(entries))
	for i, e := range entries {
		result[i] = e
		result[i].translation = translated[i]
	}
	return result, nil
}

// deeplTranslate calls the DeepL v2 API and returns one translated string per
// input text, preserving order. Free-tier keys (ending in ":fx") are routed to
// api-free.deepl.com; all others go to api.deepl.com.
func deeplTranslate(texts []string, targetLang, apiKey string) ([]string, error) {
	base := "https://api.deepl.com/v2/translate"
	if strings.HasSuffix(apiKey, ":fx") {
		base = "https://api-free.deepl.com/v2/translate"
	}

	reqBody, err := json.Marshal(struct {
		Text       []string `json:"text"`
		TargetLang string   `json:"target_lang"`
		SourceLang string   `json:"source_lang"`
	}{
		Text:       texts,
		TargetLang: targetLang,
		SourceLang: "EN",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, base, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DeepL returned HTTP %d: %s", resp.StatusCode, respBytes)
	}

	var result struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(result.Translations) != len(texts) {
		return nil, fmt.Errorf("DeepL returned %d translations for %d texts", len(result.Translations), len(texts))
	}

	out := make([]string, len(result.Translations))
	for i, t := range result.Translations {
		out[i] = t.Text
	}
	return out, nil
}

// fetchPage downloads the given URL and returns the response body as a string.
func fetchPage(url string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec — URL is built from a compile-time pattern + integer
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// parseTable finds the <figure><table> in the page body and extracts vocabulary entries.
func parseTable(body string) []entry {
	// The page contains exactly one <figure> element with the vocabulary table.
	figStart := strings.Index(body, "<figure")
	figEnd   := strings.Index(body, "</figure>")
	if figStart == -1 || figEnd == -1 {
		return nil
	}
	fig := body[figStart : figEnd+len("</figure>")]

	tbStart := strings.Index(fig, "<tbody>")
	tbEnd   := strings.Index(fig, "</tbody>")
	if tbStart == -1 || tbEnd == -1 {
		return nil
	}
	tbody := fig[tbStart+len("<tbody>") : tbEnd]

	var entries []entry
	for _, row := range splitRows(tbody) {
		cells := extractCells(row)
		if len(cells) < 4 {
			continue
		}

		// First cell is the entry number; skip category-header rows (empty first cell).
		noText := strings.TrimSpace(stripHTML(cells[0]))
		if noText == "" {
			continue
		}
		if _, err := strconv.Atoi(noText); err != nil {
			continue // not a numeric entry
		}

		hanzi       := strings.TrimSpace(stripHTML(cells[1]))
		pinyin      := normalizeSpace(stripHTML(cells[2]))
		translation := strings.TrimSpace(stripHTML(cells[3]))
		if hanzi == "" || translation == "" {
			continue
		}

		entries = append(entries, entry{hanzi: hanzi, pinyin: pinyin, translation: translation})
	}
	return entries
}

// splitRows splits an HTML tbody into individual <tr>…</tr> substrings.
func splitRows(tbody string) []string {
	var rows []string
	rest := tbody
	for {
		start := strings.Index(rest, "<tr")
		if start == -1 {
			break
		}
		end := strings.Index(rest[start:], "</tr>")
		if end == -1 {
			break
		}
		rows = append(rows, rest[start:start+end+len("</tr>")])
		rest = rest[start+end+len("</tr>"):]
	}
	return rows
}

// extractCells returns the inner HTML of each <td>…</td> in a table row.
func extractCells(row string) []string {
	var cells []string
	rest := row
	for {
		start := strings.Index(rest, "<td")
		if start == -1 {
			break
		}
		gtIdx := strings.Index(rest[start:], ">")
		if gtIdx == -1 {
			break
		}
		contentStart := start + gtIdx + 1
		closeIdx := strings.Index(rest[contentStart:], "</td>")
		if closeIdx == -1 {
			break
		}
		cells = append(cells, rest[contentStart:contentStart+closeIdx])
		rest = rest[contentStart+closeIdx+len("</td>"):]
	}
	return cells
}

// stripHTML removes all HTML tags and decodes HTML entities (including numeric
// references such as &#8217;). Non-breaking spaces are normalised to regular spaces.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '<':
			inTag = true
		case c == '>':
			inTag = false
		case !inTag:
			b.WriteByte(c)
		}
	}
	r := html.UnescapeString(b.String())
	return strings.ReplaceAll(r, "\u00a0", " ") // &nbsp; → regular space
}

// normalizeSpace collapses runs of whitespace into a single space and trims.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// isDuplicate checks whether the zh word exists and whether the specific
// zh+translation pair is already stored. Returns (zhWordID, isDup, err).
// zhWordID is non-zero whenever the zh word exists (even if the pair is new).
func isDuplicate(db *sql.DB, e entry) (zhID int64, dup bool, err error) {
	err = db.QueryRow(
		`SELECT id FROM words WHERE language = 'zh' AND lower(trim(text)) = lower(trim(?))`,
		e.hanzi,
	).Scan(&zhID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	// zh word exists — check if this exact translation is already linked.
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM translations t
		JOIN words en ON t.en_word_id = en.id
		WHERE t.zh_word_id = ?
		  AND lower(trim(en.text)) = lower(trim(?))`,
		zhID, e.translation,
	).Scan(&count)
	if err != nil {
		return zhID, false, err
	}
	return zhID, count > 0, nil
}

// insert adds a vocabulary entry in a transaction and returns the zh word ID.
func insert(db *sql.DB, e entry) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var pinyin *string
	if e.pinyin != "" {
		pinyin = &e.pinyin
	}

	// Upsert zh word.
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO words (text, language, pinyin) VALUES (?, 'zh', ?)`,
		e.hanzi, pinyin); err != nil {
		return 0, fmt.Errorf("insert zh: %w", err)
	}
	var zhID int64
	if err := tx.QueryRow(
		`SELECT id FROM words WHERE text = ? AND language = 'zh'`, e.hanzi).Scan(&zhID); err != nil {
		return 0, fmt.Errorf("get zh id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, zhID); err != nil {
		return 0, fmt.Errorf("init zh sm2: %w", err)
	}

	// Upsert translation word (stored as language='en' regardless of actual language).
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO words (text, language) VALUES (?, 'en')`, e.translation); err != nil {
		return 0, fmt.Errorf("insert translation: %w", err)
	}
	var enID int64
	if err := tx.QueryRow(
		`SELECT id FROM words WHERE text = ? AND language = 'en'`, e.translation).Scan(&enID); err != nil {
		return 0, fmt.Errorf("get translation id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, enID); err != nil {
		return 0, fmt.Errorf("init translation sm2: %w", err)
	}

	// Link zh ↔ translation.
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`,
		enID, zhID); err != nil {
		return 0, fmt.Errorf("link translation: %w", err)
	}

	return zhID, tx.Commit()
}

// assignTag upserts the named tag and links it to the zh word.
func assignTag(db *sql.DB, zhID int64, tagName string) error {
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO tags (name) VALUES (?)`, tagName); err != nil {
		return fmt.Errorf("upsert tag: %w", err)
	}
	var tagID int64
	if err := db.QueryRow(
		`SELECT id FROM tags WHERE name = ?`, tagName).Scan(&tagID); err != nil {
		return fmt.Errorf("get tag id: %w", err)
	}
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO word_tags (word_id, tag_id) VALUES (?, ?)`, zhID, tagID); err != nil {
		return fmt.Errorf("link tag: %w", err)
	}
	return nil
}

// applySchema creates all required tables if they do not yet exist.
func applySchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS words (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			text       TEXT    NOT NULL,
			language   TEXT    NOT NULL CHECK(language IN ('en', 'zh')),
			pinyin     TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(text, language)
		);
		CREATE TABLE IF NOT EXISTS translations (
			en_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
			zh_word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
			PRIMARY KEY (en_word_id, zh_word_id)
		);
		CREATE TABLE IF NOT EXISTS sm2_progress (
			word_id        INTEGER PRIMARY KEY REFERENCES words(id) ON DELETE CASCADE,
			repetitions    INTEGER NOT NULL DEFAULT 0,
			easiness       REAL    NOT NULL DEFAULT 2.5,
			interval_days  INTEGER NOT NULL DEFAULT 1,
			due_date       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			total_correct  INTEGER NOT NULL DEFAULT 0,
			total_attempts INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_sm2_due        ON sm2_progress(due_date);
		CREATE INDEX IF NOT EXISTS idx_words_text_lang ON words(text, language);
		CREATE TABLE IF NOT EXISTS tags (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);
		CREATE TABLE IF NOT EXISTS word_tags (
			word_id INTEGER NOT NULL REFERENCES words(id) ON DELETE CASCADE,
			tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
			PRIMARY KEY (word_id, tag_id)
		);
	`)
	return err
}
