// cmd/fill-hanzi-translations/main.go — Translate hanzi_decomposition definitions via DeepL.
//
// For every row in hanzi_decomposition that has a non-empty English definition and
// does not yet have a translation for the target language, this tool calls the DeepL
// API and stores the result in hanzi_decomposition_translation.
//
// Usage:
//
//	DEEPL_API_KEY=... go run ./cmd/fill-hanzi-translations [-db data/vocab.db] [-lang DE] [-batch 50] [-limit 0] [-dry-run]
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	vocabdb "vocabulary_trainer/db"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "data/vocab.db", "path to SQLite database")
	lang := flag.String("lang", "DE", "DeepL target language code (e.g. DE, EN, FR)")
	batchSize := flag.Int("batch", 50, "number of texts per DeepL API call")
	limit := flag.Int("limit", 0, "stop after inserting N rows (0 = no limit)")
	dryRun := flag.Bool("dry-run", false, "print what would be inserted without writing")
	flag.Parse()

	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		log.Fatal("DEEPL_API_KEY environment variable is required")
	}

	targetLang := strings.ToUpper(*lang)

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

	entries, err := loadPending(db, targetLang)
	if err != nil {
		log.Fatalf("load pending: %v", err)
	}

	fmt.Printf("Found %d character(s) without a %s definition.\n", len(entries), targetLang)
	if len(entries) == 0 {
		return
	}

	if *limit > 0 && len(entries) > *limit {
		entries = entries[:*limit]
		fmt.Printf("Limiting to %d character(s).\n", *limit)
	}

	var totalInserted, totalFailed int

	for i := 0; i < len(entries); i += *batchSize {
		end := i + *batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		texts := make([]string, len(batch))
		for k, e := range batch {
			texts[k] = e.definition
		}

		translated, err := deeplTranslate(texts, "EN", targetLang, apiKey)
		if err != nil {
			fmt.Printf("  [ERROR] DeepL batch %d–%d: %v\n", i+1, end, err)
			totalFailed += len(batch)
			continue
		}

		for k, e := range batch {
			def := translated[k]
			if def == "" {
				totalFailed++
				continue
			}
			langLower := strings.ToLower(targetLang)
			if *dryRun {
				fmt.Printf("  [DRY]   %s → [%s] %s\n", e.character, langLower, def)
				totalInserted++
				continue
			}
			if err := insertTranslation(db, e.character, targetLang, def); err != nil {
				fmt.Printf("  [ERROR] insert %q (%s): %v\n", e.character, langLower, err)
				totalFailed++
				continue
			}
			fmt.Printf("  [OK]    %s → [%s] %s\n", e.character, langLower, def)
			totalInserted++
		}
	}

	fmt.Printf("\nDone. inserted=%d  errors=%d\n", totalInserted, totalFailed)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

type entry struct {
	character  string
	definition string
}

// loadPending returns all characters that have a non-empty English definition
// but no existing translation for the given target language.
func loadPending(db *sql.DB, lang string) ([]entry, error) {
	rows, err := db.Query(`
		SELECT character, definition
		FROM hanzi_decomposition
		WHERE definition IS NOT NULL AND definition != ''
		  AND character NOT IN (
		      SELECT character FROM hanzi_decomposition_translation WHERE lang = ?
		  )
		ORDER BY character`,
		lang,
	)
	if err != nil {
		return nil, err
	}
	var result []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.character, &e.definition); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, e)
	}
	rows.Close()
	return result, rows.Err()
}

// insertTranslation writes a single translated definition to hanzi_decomposition_translation.
func insertTranslation(db *sql.DB, character, lang, definition string) error {
	_, err := db.Exec(
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition)
		 VALUES (?, ?, ?)
		 ON CONFLICT(character, lang) DO NOTHING`,
		character, lang, definition,
	)
	return err
}

// deeplTranslate calls the DeepL v2 API and returns one translated string per input text.
// Free-tier keys (ending in ":fx") are routed to api-free.deepl.com.
func deeplTranslate(texts []string, sourceLang, targetLang, apiKey string) ([]string, error) {
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
		SourceLang: sourceLang,
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
