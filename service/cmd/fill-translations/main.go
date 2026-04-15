// cmd/fill-translations/main.go — Fill missing EN or DE translations for zh words.
//
// For every zh word that is missing an EN or DE translation, this tool queries
// the database, selects the best available source text, calls the DeepL API,
// and inserts the result.
//
// Translation strategy per missing language:
//   - zh exists, en missing  → translate zh text (ZH → EN)
//   - zh exists, de missing  → translate zh text (ZH → DE)
//   - en exists, de missing  → translate en text (EN → DE)  [used instead of zh→de when en is available]
//   - de exists, en missing  → translate de text (DE → EN)  [used instead of zh→en when de is available]
//
// Note: a single zh word may produce two DeepL calls if both en and de are absent.
//
// Usage:
//
//	DEEPL_API_KEY=... go run ./cmd/fill-translations [-db data/vocab.db] [-dry-run] [-batch 50]
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
	dryRun := flag.Bool("dry-run", false, "show what would be inserted without writing")
	batchSize := flag.Int("batch", 50, "DeepL batch size")
	flag.Parse()

	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		log.Fatal("DEEPL_API_KEY environment variable is required")
	}

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

	words, err := loadMissingTranslations(db)
	if err != nil {
		log.Fatalf("load words: %v", err)
	}
	fmt.Printf("Found %d zh word(s) with missing EN or DE translations.\n", len(words))
	if len(words) == 0 {
		return
	}

	// Build two independent translation jobs: one per target language.
	// Each job collects (zhID, sourceText, sourceLang, targetLang).
	type job struct {
		zhID       int64
		zhText     string
		sourceText string
		sourceLang string
		targetLang string
	}

	var enJobs, deJobs []job
	for _, w := range words {
		if !w.hasEN {
			src, srcLang := w.zhText, "ZH"
			if w.hasDE {
				src, srcLang = w.deText, "DE"
			}
			enJobs = append(enJobs, job{w.zhID, w.zhText, src, srcLang, "EN"})
		}
		if !w.hasDE {
			src, srcLang := w.zhText, "ZH"
			if w.hasEN {
				src, srcLang = w.enText, "EN"
			}
			deJobs = append(deJobs, job{w.zhID, w.zhText, src, srcLang, "DE"})
		}
	}

	var totalInserted, totalFailed int

	for _, jobs := range [][]job{enJobs, deJobs} {
		if len(jobs) == 0 {
			continue
		}
		targetLang := jobs[0].targetLang
		fmt.Printf("\nTranslating %d word(s) → %s…\n", len(jobs), targetLang)

		// Translate in batches.
		for i := 0; i < len(jobs); i += *batchSize {
			end := i + *batchSize
			if end > len(jobs) {
				end = len(jobs)
			}
			batch := jobs[i:end]

			// Group by source language (all items in a batch may have the same source lang
			// when they come from a uniform strategy, but guard for mixed batches).
			bySource := map[string][]int{} // sourceLang → indices within batch
			for idx, j := range batch {
				bySource[j.sourceLang] = append(bySource[j.sourceLang], idx)
			}

			results := make([]string, len(batch))
			for srcLang, indices := range bySource {
				texts := make([]string, len(indices))
				for k, idx := range indices {
					texts[k] = batch[idx].sourceText
				}
				translated, err := deeplTranslate(texts, srcLang, targetLang, apiKey)
				if err != nil {
					fmt.Printf("  [ERROR] DeepL batch (src=%s target=%s): %v\n", srcLang, targetLang, err)
					totalFailed += len(indices)
					// Mark failed slots with empty string so we skip them below.
					continue
				}
				for k, idx := range indices {
					results[idx] = translated[k]
				}
			}

			for idx, j := range batch {
				translatedText := results[idx]
				if translatedText == "" {
					continue // translation failed for this item
				}
				lang := strings.ToLower(targetLang)
				if *dryRun {
					fmt.Printf("  [DRY]   %s → [%s] %s\n", j.zhText, lang, translatedText)
					totalInserted++
					continue
				}
				if err := insertTranslation(db, j.zhID, translatedText, lang); err != nil {
					fmt.Printf("  [ERROR] insert %q (%s): %v\n", j.zhText, lang, err)
					totalFailed++
					continue
				}
				fmt.Printf("  [OK]    %s → [%s] %s\n", j.zhText, lang, translatedText)
				totalInserted++
			}
		}
	}

	fmt.Printf("\nDone. inserted=%d  errors=%d\n", totalInserted, totalFailed)
	if *dryRun {
		fmt.Println("(dry-run: no changes were written)")
	}
}

type zhWord struct {
	zhID   int64
	zhText string
	hasEN  bool
	enText string
	hasDE  bool
	deText string
}

// loadMissingTranslations returns all zh words that are missing at least one of EN or DE.
func loadMissingTranslations(db *sql.DB) ([]zhWord, error) {
	rows, err := db.Query(`
		SELECT
			w.id,
			w.text,
			(SELECT MIN(ew.text) FROM words ew JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id WHERE ew.language = 'en') AS en_text,
			(SELECT MIN(ew.text) FROM words ew JOIN translations t ON t.en_word_id = ew.id AND t.zh_word_id = w.id WHERE ew.language = 'de') AS de_text
		FROM words w
		WHERE w.language = 'zh'
		  AND (
		    NOT EXISTS (SELECT 1 FROM translations t JOIN words ew ON t.en_word_id = ew.id WHERE t.zh_word_id = w.id AND ew.language = 'en')
		    OR
		    NOT EXISTS (SELECT 1 FROM translations t JOIN words ew ON t.en_word_id = ew.id WHERE t.zh_word_id = w.id AND ew.language = 'de')
		  )
		ORDER BY w.id
	`)
	if err != nil {
		return nil, err
	}

	var result []zhWord
	for rows.Next() {
		var w zhWord
		var enText, deText sql.NullString
		if err := rows.Scan(&w.zhID, &w.zhText, &enText, &deText); err != nil {
			rows.Close()
			return nil, err
		}
		w.hasEN = enText.Valid && enText.String != ""
		w.enText = enText.String
		w.hasDE = deText.Valid && deText.String != ""
		w.deText = deText.String
		result = append(result, w)
	}
	rows.Close()
	return result, rows.Err()
}

// insertTranslation inserts a translation word for the given zh word and links them.
func insertTranslation(db *sql.DB, zhID int64, text, lang string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO words (text, language) VALUES (?, ?)`, text, lang,
	); err != nil {
		return fmt.Errorf("insert word: %w", err)
	}
	var transID int64
	if err := tx.QueryRow(
		`SELECT id FROM words WHERE text = ? AND language = ?`, text, lang,
	).Scan(&transID); err != nil {
		return fmt.Errorf("get word id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO sm2_progress (word_id) VALUES (?)`, transID,
	); err != nil {
		return fmt.Errorf("init sm2: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO translations (en_word_id, zh_word_id) VALUES (?, ?)`, transID, zhID,
	); err != nil {
		return fmt.Errorf("link translation: %w", err)
	}

	return tx.Commit()
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
