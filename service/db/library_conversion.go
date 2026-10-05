package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"reflect"
	"sort"
	"vocabulary_trainer/models"
)

// libraryConversion names the one-time conversion in data_conversions.
const libraryConversion = "library_references"

// LibraryConversionPending reports whether copied words still have to be
// converted to library references (see ConvertToLibraryReferences).
func (s *Store) LibraryConversionPending(ctx context.Context) (bool, error) {
	var done bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM data_conversions WHERE name = ?)`, libraryConversion).Scan(&done)
	if err != nil {
		return false, fmt.Errorf("check library conversion: %w", err)
	}
	return !done, nil
}

// BackupTo writes a consistent copy of the database to path (VACUUM INTO,
// safe while the database is in WAL mode).
func (s *Store) BackupTo(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("backup database: %w", err)
	}
	return nil
}

type convertWord struct {
	id, userID int64
	text       string
}

// ConvertToLibraryReferences converts every learner's own zh word whose text
// the dictionaries have into a library reference (ADR-0005), once. Learners
// active in the last 7 days keep exactly the glosses (and sources) they see;
// for all others the word shows the library glosses plus their own extra
// glosses. Copied gloss links and words are deleted. A learner's languages
// follow the glosses they had (e.g. EN only: no secondary language). Before
// committing, every converted word's glosses are compared with the state
// before; any difference rolls everything back.
func (s *Store) ConvertToLibraryReferences(ctx context.Context) (models.LibraryConversionReport, error) {
	var report models.LibraryConversionReport

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, text FROM words
		 WHERE user_id != ? AND language = 'zh' AND library_word_id IS NULL ORDER BY user_id, id`, LibraryUserID)
	if err != nil {
		return report, fmt.Errorf("load own words: %w", err)
	}
	var words []convertWord
	textSet := map[string]bool{}
	for rows.Next() {
		var w convertWord
		if err := rows.Scan(&w.id, &w.userID, &w.text); err != nil {
			rows.Close()
			return report, err
		}
		words = append(words, w)
		textSet[w.text] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	active := map[int64]bool{}
	rows, err = s.db.QueryContext(ctx, `
		SELECT user_id FROM daily_stats WHERE date >= date('now', '-7 days')
		UNION
		SELECT user_id FROM usage_events WHERE substr(last_seen, 1, 10) >= date('now', '-7 days')`)
	if err != nil {
		return report, fmt.Errorf("load active users: %w", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return report, err
		}
		active[id] = true
	}
	rows.Close()

	// The library words must exist before the transaction (one connection).
	texts := make([]string, 0, len(textSet))
	for t := range textSet {
		texts = append(texts, t)
	}
	sort.Strings(texts)
	libraryIDs, err := s.ensureLibraryWords(ctx, texts)
	if err != nil {
		return report, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	byUser := map[int64][]convertWord{}
	var userIDs []int64
	for _, w := range words {
		if _, ok := libraryIDs[w.text]; !ok {
			continue
		}
		if byUser[w.userID] == nil {
			userIDs = append(userIDs, w.userID)
		}
		byUser[w.userID] = append(byUser[w.userID], w)
	}

	for _, userID := range userIDs {
		userWords := byUser[userID]
		faithful := active[userID]
		ids := make([]int64, len(userWords))
		for i, w := range userWords {
			ids[i] = w.id
		}
		before, err := snapshotGlosses(ctx, tx, ids)
		if err != nil {
			return report, err
		}
		if err := setLanguagesFromGlosses(ctx, tx, userID); err != nil {
			return report, err
		}
		for _, w := range userWords {
			if err := convertWordToReference(ctx, tx, w, libraryIDs[w.text], faithful); err != nil {
				return report, fmt.Errorf("convert word %d (%s) of user %d: %w", w.id, w.text, userID, err)
			}
		}
		after, err := snapshotGlosses(ctx, tx, ids)
		if err != nil {
			return report, err
		}
		if err := compareConvertedGlosses(before, after, faithful); err != nil {
			return report, fmt.Errorf("user %d: %w", userID, err)
		}
		report.Users++
		report.Words += len(userWords)
		if faithful {
			report.FaithfulUsers++
		}
		log.Printf("Library conversion: user %d, %d words (faithful=%v)", userID, len(userWords), faithful)
	}

	res, err := tx.ExecContext(ctx,
		`DELETE FROM words WHERE user_id != ? AND language != 'zh'
		   AND NOT EXISTS (SELECT 1 FROM translations t WHERE t.translation_word_id = words.id)`, LibraryUserID)
	if err != nil {
		return report, fmt.Errorf("delete copied gloss words: %w", err)
	}
	n, _ := res.RowsAffected()
	report.GlossWordsDeleted = int(n)
	if _, err := tx.ExecContext(ctx, `INSERT INTO data_conversions (name) VALUES (?)`, libraryConversion); err != nil {
		return report, fmt.Errorf("record library conversion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	for _, userID := range userIDs {
		s.invalidateSettingsCache(userID)
	}
	return report, nil
}

// setLanguagesFromGlosses sets a learner's primary and secondary language to
// the languages of their glosses: a learner who only had EN glosses gets no
// secondary language, so the library's DE glosses stay hidden.
func setLanguagesFromGlosses(ctx context.Context, tx *sql.Tx, userID int64) error {
	have := map[string]bool{}
	rows, err := tx.QueryContext(ctx,
		`SELECT DISTINCT tw.language FROM translations t
		 JOIN words z ON z.id = t.zh_word_id
		 JOIN words tw ON tw.id = t.translation_word_id
		 WHERE z.user_id = ? AND z.language = 'zh'`, userID)
	if err != nil {
		return fmt.Errorf("load learner gloss languages: %w", err)
	}
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			rows.Close()
			return err
		}
		have[lang] = true
	}
	rows.Close()
	if len(have) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_settings (user_id) VALUES (?)`, userID); err != nil {
		return fmt.Errorf("ensure settings: %w", err)
	}
	var primary string
	if err := tx.QueryRowContext(ctx, `SELECT primary_lang FROM user_settings WHERE user_id = ?`, userID).Scan(&primary); err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if !have[primary] {
		primary = "en"
		if !have["en"] {
			primary = "de"
		}
	}
	secondary := ""
	for _, lang := range libraryLangs {
		if have[lang] && lang != primary {
			secondary = lang
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_settings SET primary_lang = ?, secondary_lang = ? WHERE user_id = ?`, primary, secondary, userID); err != nil {
		return fmt.Errorf("update languages: %w", err)
	}
	return nil
}

// convertWordToReference turns a learner's copied word into a reference to
// libraryID. faithful keeps exactly the copied glosses and their sources;
// otherwise the word shows the library glosses plus the copied glosses the
// library lacks.
func convertWordToReference(ctx context.Context, tx *sql.Tx, w convertWord, libraryID int64, faithful bool) error {
	texts, sources, err := referenceGlosses(ctx, tx, w.id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE words SET library_word_id = ? WHERE id = ?`, libraryID, w.id); err != nil {
		return err
	}
	if !faithful {
		library, err := visibleLibraryGlosses(ctx, tx, w.userID, libraryID)
		if err != nil {
			return err
		}
		for lang := range texts {
			for i, text := range texts[lang] {
				if _, ok := library[wordKey{text, lang}]; ok {
					sources[lang][i] = "cedict"
				}
			}
		}
		for key := range library {
			texts[key.lang] = append(texts[key.lang], key.text)
			sources[key.lang] = append(sources[key.lang], "cedict")
		}
	}
	return setReferenceGlosses(ctx, tx, w.userID, w.id, libraryID, texts, sources)
}

// snapshotGlosses returns, per word, the sorted "text|source" glosses per
// language that the learner sees.
func snapshotGlosses(ctx context.Context, q querier, ids []int64) (map[int64]map[string][]string, error) {
	out := map[int64]map[string][]string{}
	for _, id := range ids {
		texts, sources, err := referenceGlosses(ctx, q, id)
		if err != nil {
			return nil, err
		}
		m := map[string][]string{}
		for lang := range texts {
			for i, text := range texts[lang] {
				m[lang] = append(m[lang], text+"|"+sources[lang][i])
			}
			sort.Strings(m[lang])
		}
		out[id] = m
	}
	return out, nil
}

// compareConvertedGlosses checks a conversion: with faithful, every word must
// show exactly the glosses it showed before; otherwise every gloss text it
// showed before must still be there (the library may add more).
func compareConvertedGlosses(before, after map[int64]map[string][]string, faithful bool) error {
	for id, b := range before {
		a := after[id]
		if faithful {
			if !reflect.DeepEqual(a, b) {
				return fmt.Errorf("word %d: glosses changed from %v to %v", id, b, a)
			}
			continue
		}
		for lang, glosses := range b {
			have := map[string]bool{}
			for _, g := range a[lang] {
				have[glossText(g)] = true
			}
			for _, g := range glosses {
				if !have[glossText(g)] {
					return fmt.Errorf("word %d: gloss %q (%s) lost", id, glossText(g), lang)
				}
			}
		}
	}
	return nil
}

// glossText strips the "|source" suffix of a snapshot gloss.
func glossText(g string) string {
	for i := len(g) - 1; i >= 0; i-- {
		if g[i] == '|' {
			return g[:i]
		}
	}
	return g
}
