package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"vocabulary_trainer/models"
)

// LibraryUserID owns the shared library (see ADR-0005).
const LibraryUserID int64 = 1

// libraryLangs are the languages the library takes glosses for: CC-CEDICT
// for en, HanDeDict for de.
var libraryLangs = []string{"en", "de"}

type libraryWord struct {
	id      int64
	text    string
	removed bool
}

// RefreshLibrary brings every library zh word's glosses in line with the
// dictionaries (each definition split into senses, as the import does).
// Rows are upserted by text, so unchanged glosses keep their ids and learner
// references and deletions stay valid. A word whose existing gloss set changes
// gets library_updated_at; a word with no dictionary entry any more keeps its
// glosses and is flagged library_removed. Gloss words left without any link
// are deleted.
func (s *Store) RefreshLibrary(ctx context.Context) (models.LibraryRefreshReport, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, text, library_removed FROM words WHERE user_id = ? AND language = 'zh' ORDER BY id`, LibraryUserID)
	if err != nil {
		return models.LibraryRefreshReport{}, fmt.Errorf("load library words: %w", err)
	}
	var words []libraryWord
	for rows.Next() {
		var w libraryWord
		if err := rows.Scan(&w.id, &w.text, &w.removed); err != nil {
			rows.Close()
			return models.LibraryRefreshReport{}, err
		}
		words = append(words, w)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.LibraryRefreshReport{}, err
	}
	rows.Close()
	return s.refreshLibraryWords(ctx, words)
}

// refreshLibraryWords is RefreshLibrary for the given library words.
func (s *Store) refreshLibraryWords(ctx context.Context, words []libraryWord) (models.LibraryRefreshReport, error) {
	var report models.LibraryRefreshReport
	ids := make([]int64, len(words))
	for i, w := range words {
		ids[i] = w.id
	}
	current, err := s.libraryLinks(ctx, ids)
	if err != nil {
		return report, err
	}

	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.text
	}
	dict, err := s.LookupDictionaryBatch(ctx, texts, libraryLangs)
	if err != nil {
		return report, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	bw, err := newBatchWriter(ctx, tx, LibraryUserID)
	if err != nil {
		return report, err
	}
	defer bw.close()
	bw.noProgress = true

	for _, w := range words {
		report.Words++
		want := map[wordKey]bool{}
		for lang, senses := range dict[w.text] {
			for _, sense := range senses {
				if sense = strings.TrimSpace(sense); sense != "" {
					want[wordKey{sense, lang}] = true
				}
			}
		}
		if len(want) == 0 {
			if !w.removed {
				if _, err := tx.ExecContext(ctx, `UPDATE words SET library_removed = 1 WHERE id = ?`, w.id); err != nil {
					return report, fmt.Errorf("flag removed library word: %w", err)
				}
			}
			report.Missing++
			continue
		}
		if w.removed {
			if _, err := tx.ExecContext(ctx, `UPDATE words SET library_removed = 0 WHERE id = ?`, w.id); err != nil {
				return report, fmt.Errorf("unflag library word: %w", err)
			}
		}

		have := current[w.id]
		changed := false
		for key := range want {
			if _, ok := have[key]; ok {
				continue
			}
			transID, err := bw.word(ctx, key.text, key.lang, nil)
			if err != nil {
				return report, err
			}
			rank, err := bw.rank(ctx, key.lang, key.text, "cedict")
			if err != nil {
				return report, err
			}
			if _, err := bw.insLink.ExecContext(ctx, transID, w.id, "cedict", rank); err != nil {
				return report, fmt.Errorf("link library gloss: %w", err)
			}
			report.Added++
			changed = true
		}
		for key, transID := range have {
			if want[key] {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM translations WHERE translation_word_id = ? AND zh_word_id = ?`, transID, w.id); err != nil {
				return report, fmt.Errorf("drop library gloss: %w", err)
			}
			report.Dropped++
			changed = true
		}
		if changed && len(have) > 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE words SET library_updated_at = CURRENT_TIMESTAMP WHERE id = ?`, w.id); err != nil {
				return report, fmt.Errorf("mark library word updated: %w", err)
			}
			report.Changed++
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM words WHERE user_id = ? AND language != 'zh'
		   AND NOT EXISTS (SELECT 1 FROM translations t WHERE t.translation_word_id = words.id)`, LibraryUserID); err != nil {
		return report, fmt.Errorf("delete orphan library glosses: %w", err)
	}
	return report, tx.Commit()
}

// libraryLinks returns the gloss links of the given library zh words: zh id
// → (text, lang) → gloss word id.
func (s *Store) libraryLinks(ctx context.Context, ids []int64) (map[int64]map[wordKey]int64, error) {
	out := map[int64]map[wordKey]int64{}
	for start := 0; start < len(ids); start += lookupBatchChunk {
		chunk := ids[start:min(start+lookupBatchChunk, len(ids))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT t.zh_word_id, tw.text, tw.language, tw.id
			 FROM translations t
			 JOIN words tw ON tw.id = t.translation_word_id
			 WHERE t.zh_word_id IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("load library links: %w", err)
		}
		for rows.Next() {
			var zhID, transID int64
			var key wordKey
			if err := rows.Scan(&zhID, &key.text, &key.lang, &transID); err != nil {
				rows.Close()
				return nil, err
			}
			if out[zhID] == nil {
				out[zhID] = map[wordKey]int64{}
			}
			out[zhID][key] = transID
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// LibraryNeedsPrefill reports whether the library has zh words but no gloss
// at all while the dictionaries are loaded — the state right after upgrading
// to library references, or after the first dictionary import.
func (s *Store) LibraryNeedsPrefill(ctx context.Context) (bool, error) {
	var need bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM words WHERE user_id = ? AND language = 'zh')
		   AND NOT EXISTS (SELECT 1 FROM translations t JOIN words z ON z.id = t.zh_word_id
		                   WHERE z.user_id = ? AND z.language = 'zh')
		   AND EXISTS (SELECT 1 FROM cedict_entries)`, LibraryUserID, LibraryUserID).Scan(&need)
	if err != nil {
		return false, fmt.Errorf("check library prefill: %w", err)
	}
	return need, nil
}

// CreateReferences gives userID a library reference to each library zh word
// in libraryIDs (in order) and returns the learner's word ids. A reference
// is a slim zh word row with the library pinyin and library_word_id; it has
// no gloss links of its own and starts unseen, like a new word. tags are
// added to every reference. A word the learner already has (same text) is
// returned as is. Creating a reference clears its tombstone.
func (s *Store) CreateReferences(ctx context.Context, userID int64, libraryIDs []int64, tags []string) ([]int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	bw, err := newBatchWriter(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	defer bw.close()

	ins, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO words (text, language, pinyin, user_id, library_word_id)
		 SELECT text, 'zh', pinyin, ?, id FROM words WHERE id = ? AND user_id = ? AND language = 'zh'`)
	if err != nil {
		return nil, fmt.Errorf("prepare reference insert: %w", err)
	}
	defer ins.Close()
	sel, err := tx.PrepareContext(ctx,
		`SELECT w.id FROM words w JOIN words l ON l.text = w.text
		 WHERE l.id = ? AND w.user_id = ? AND w.language = 'zh'`)
	if err != nil {
		return nil, fmt.Errorf("prepare reference lookup: %w", err)
	}
	defer sel.Close()

	ids := make([]int64, 0, len(libraryIDs))
	for _, libID := range libraryIDs {
		if _, err := ins.ExecContext(ctx, userID, libID, LibraryUserID); err != nil {
			return nil, fmt.Errorf("insert reference: %w", err)
		}
		var id int64
		if err := sel.QueryRowContext(ctx, libID, userID).Scan(&id); err != nil {
			return nil, fmt.Errorf("get reference id for library word %d: %w", libID, err)
		}
		if _, err := bw.insSM2.ExecContext(ctx, id); err != nil {
			return nil, fmt.Errorf("init sm2: %w", err)
		}
		for _, name := range tags {
			if name = strings.TrimSpace(name); name == "" {
				continue
			}
			tagID, err := bw.tag(ctx, name)
			if err != nil {
				return nil, err
			}
			if _, err := bw.insWordTag.ExecContext(ctx, id, tagID); err != nil {
				return nil, fmt.Errorf("link tag: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM word_tombstones WHERE user_id = ? AND library_word_id = ?`, userID, libID); err != nil {
			return nil, fmt.Errorf("clear tombstone: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, tx.Commit()
}

// learnerLangs returns the native languages a learner sees library glosses in
// (primary, secondary), with the same defaults as the user_translations view.
func learnerLangs(ctx context.Context, q querier, userID int64) (map[string]bool, error) {
	primary, secondary := "en", "de"
	err := q.QueryRowContext(ctx,
		`SELECT primary_lang, secondary_lang FROM user_settings WHERE user_id = ?`, userID).Scan(&primary, &secondary)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("load learner languages: %w", err)
	}
	return map[string]bool{primary: true, secondary: true}, nil
}

// visibleLibraryGlosses returns the library word's glosses in the learner's
// languages: (text, lang) → gloss word id.
func visibleLibraryGlosses(ctx context.Context, tx querier, userID, libraryID int64) (map[wordKey]int64, error) {
	langs, err := learnerLangs(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT tw.text, tw.language, tw.id FROM translations t
		 JOIN words tw ON tw.id = t.translation_word_id
		 WHERE t.zh_word_id = ?`, libraryID)
	if err != nil {
		return nil, fmt.Errorf("load library glosses: %w", err)
	}
	defer rows.Close()
	out := map[wordKey]int64{}
	for rows.Next() {
		var key wordKey
		var id int64
		if err := rows.Scan(&key.text, &key.lang, &id); err != nil {
			return nil, err
		}
		if langs[key.lang] {
			out[key] = id
		}
	}
	return out, rows.Err()
}

// setReferenceGlosses makes the learner see exactly the requested glosses on
// a library reference: requested library glosses stay shared, the other
// visible library glosses become deletions, and requested glosses the
// library lacks become the learner's own links. overrides_updated_at is set
// when the overrides change, and cleared when none remain.
func setReferenceGlosses(ctx context.Context, tx *sql.Tx, userID, refID, libraryID int64, translations, sources map[string][]string) error {
	library, err := visibleLibraryGlosses(ctx, tx, userID, libraryID)
	if err != nil {
		return err
	}
	type ownGloss struct {
		key    wordKey
		source string
	}
	// A requested gloss stays shared only when it came from the dictionary
	// (source cedict) and the library has it. A gloss the learner typed
	// (source user) is their own, even when the library has the same text:
	// it keeps its source and order, and hides the library copy.
	shared := map[wordKey]bool{}
	seen := map[wordKey]bool{}
	var own []ownGloss
	for lang, texts := range translations {
		for i, text := range texts {
			key := wordKey{strings.TrimSpace(text), lang}
			if key.text == "" || seen[key] {
				continue
			}
			seen[key] = true
			source := sourceAt(sources[lang], i)
			if _, ok := library[key]; ok && source == "cedict" {
				shared[key] = true
				continue
			}
			own = append(own, ownGloss{key, source})
		}
	}
	deletions := map[int64]bool{}
	for key, id := range library {
		if !shared[key] {
			deletions[id] = true
		}
	}

	// Compare with the current overrides, so that saving an unchanged form
	// does not mark the reference as edited.
	curDeletions := map[int64]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT translation_word_id FROM translation_deletions WHERE user_word_id = ?`, refID)
	if err != nil {
		return fmt.Errorf("load deletions: %w", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		curDeletions[id] = true
	}
	rows.Close()
	curOwn := map[wordKey]bool{}
	rows, err = tx.QueryContext(ctx,
		`SELECT tw.text, tw.language FROM translations t JOIN words tw ON tw.id = t.translation_word_id
		 WHERE t.zh_word_id = ?`, refID)
	if err != nil {
		return fmt.Errorf("load own glosses: %w", err)
	}
	for rows.Next() {
		var key wordKey
		if err := rows.Scan(&key.text, &key.lang); err != nil {
			rows.Close()
			return err
		}
		curOwn[key] = true
	}
	rows.Close()
	changed := len(curDeletions) != len(deletions) || len(curOwn) != len(own)
	for id := range deletions {
		if !curDeletions[id] {
			changed = true
		}
	}
	for _, g := range own {
		if !curOwn[g.key] {
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM translation_deletions WHERE user_word_id = ?`, refID); err != nil {
		return fmt.Errorf("clear deletions: %w", err)
	}
	for id := range deletions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO translation_deletions (user_word_id, translation_word_id) VALUES (?, ?)`, refID, id); err != nil {
			return fmt.Errorf("add deletion: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM translations WHERE zh_word_id = ?`, refID); err != nil {
		return fmt.Errorf("clear own glosses: %w", err)
	}
	for _, g := range own {
		transID, err := upsertWord(ctx, tx, g.key.text, g.key.lang, nil, userID)
		if err != nil {
			return err
		}
		if err := linkTranslation(ctx, tx, transID, refID, g.key.lang, g.key.text, g.source); err != nil {
			return fmt.Errorf("link own gloss: %w", err)
		}
	}
	stamp := "CURRENT_TIMESTAMP"
	if len(deletions) == 0 && len(own) == 0 {
		stamp = "NULL"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE words SET overrides_updated_at = `+stamp+` WHERE id = ?`, refID); err != nil {
		return fmt.Errorf("stamp overrides: %w", err)
	}
	return nil
}

// referenceGlosses returns the glosses a learner currently sees on a word,
// with their sources, in the shape of a word request.
func referenceGlosses(ctx context.Context, tx querier, wordID int64) (map[string][]string, map[string][]string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT tw.language, tw.text, t.source FROM user_translations t
		 JOIN words tw ON tw.id = t.translation_word_id
		 WHERE t.zh_word_id = ?`, wordID)
	if err != nil {
		return nil, nil, fmt.Errorf("load glosses: %w", err)
	}
	defer rows.Close()
	texts, sources := map[string][]string{}, map[string][]string{}
	for rows.Next() {
		var lang, text, source string
		if err := rows.Scan(&lang, &text, &source); err != nil {
			return nil, nil, err
		}
		texts[lang] = append(texts[lang], text)
		sources[lang] = append(sources[lang], source)
	}
	return texts, sources, rows.Err()
}

// TombstonedLibraryWords returns the library words the learner deleted after
// having a reference to them.
func (s *Store) TombstonedLibraryWords(ctx context.Context, userID int64) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT library_word_id FROM word_tombstones WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("load tombstones: %w", err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// LibraryWordsWithGlosses returns which of the library words have at least
// one gloss in the learner's languages. A reference to any other word would
// have no translation, so the import skips it.
func (s *Store) LibraryWordsWithGlosses(ctx context.Context, userID int64, libraryIDs []int64) (map[int64]bool, error) {
	langs, err := learnerLangs(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for start := 0; start < len(libraryIDs); start += lookupBatchChunk {
		chunk := libraryIDs[start:min(start+lookupBatchChunk, len(libraryIDs))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT DISTINCT t.zh_word_id, tw.language FROM translations t
			 JOIN words tw ON tw.id = t.translation_word_id
			 WHERE t.zh_word_id IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("check library glosses: %w", err)
		}
		for rows.Next() {
			var id int64
			var lang string
			if err := rows.Scan(&id, &lang); err != nil {
				rows.Close()
				return nil, err
			}
			if langs[lang] {
				out[id] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// listLibraryWords returns the library zh words tagged tag and every one of
// andTags, in list order.
func (s *Store) listLibraryWords(ctx context.Context, tag string, andTags []string) ([]libraryWord, error) {
	args := []any{tag, LibraryUserID}
	andFilter := ""
	if len(andTags) > 0 {
		andFilter = ` AND (SELECT COUNT(DISTINCT tg2.name) FROM word_tags wt2 JOIN tags tg2 ON tg2.id = wt2.tag_id
		                  WHERE wt2.word_id = w.id AND tg2.name IN (?` + strings.Repeat(",?", len(andTags)-1) + `)) = ?`
		for _, tg := range andTags {
			args = append(args, tg)
		}
		args = append(args, len(andTags))
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.text FROM words w
		 JOIN word_tags wt ON wt.word_id = w.id
		 JOIN tags tg ON tg.id = wt.tag_id AND tg.name = ?
		 WHERE w.user_id = ? AND w.language = 'zh'`+andFilter+`
		 ORDER BY w.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list library words: %w", err)
	}
	defer rows.Close()
	var out []libraryWord
	for rows.Next() {
		var w libraryWord
		if err := rows.Scan(&w.id, &w.text); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ImportedLists returns the library lists the learner imported (one entry
// per tag + and_tags, with the tags of the last finished import), each with
// the number of new and of removed words (see models.ImportedList).
func (s *Store) ImportedLists(ctx context.Context, userID int64) ([]models.ImportedList, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag, and_tags, apply_tags FROM import_jobs
		 WHERE user_id = ? AND status = 'done' ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("load imported lists: %w", err)
	}
	var lists []models.ImportedList
	index := map[string]int{}
	for rows.Next() {
		var l models.ImportedList
		var andJSON, applyJSON string
		if err := rows.Scan(&l.Tag, &andJSON, &applyJSON); err != nil {
			rows.Close()
			return nil, err
		}
		l.AndTags, l.ApplyTags = []string{}, []string{}
		_ = json.Unmarshal([]byte(andJSON), &l.AndTags)
		_ = json.Unmarshal([]byte(applyJSON), &l.ApplyTags)
		key := l.Tag + "\x00" + andJSON
		if i, ok := index[key]; ok {
			lists[i].ApplyTags = l.ApplyTags
			continue
		}
		index[key] = len(lists)
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	sort.Slice(lists, func(i, j int) bool { return lists[i].Tag < lists[j].Tag })

	have := map[string]bool{}
	textRows, err := s.db.QueryContext(ctx, `SELECT text FROM words WHERE user_id = ? AND language = 'zh'`, userID)
	if err != nil {
		return nil, fmt.Errorf("load learner words: %w", err)
	}
	for textRows.Next() {
		var text string
		if err := textRows.Scan(&text); err != nil {
			textRows.Close()
			return nil, err
		}
		have[text] = true
	}
	textRows.Close()
	stones, err := s.TombstonedLibraryWords(ctx, userID)
	if err != nil {
		return nil, err
	}

	for i := range lists {
		words, err := s.listLibraryWords(ctx, lists[i].Tag, lists[i].AndTags)
		if err != nil {
			return nil, err
		}
		var candidates []int64
		for _, w := range words {
			switch {
			case stones[w.id]:
				lists[i].Removed++
			case !have[w.text]:
				candidates = append(candidates, w.id)
			}
		}
		withGlosses, err := s.LibraryWordsWithGlosses(ctx, userID, candidates)
		if err != nil {
			return nil, err
		}
		lists[i].New = len(withGlosses)
		conflicts, err := s.LibraryConflicts(ctx, userID, lists[i].Tag, lists[i].AndTags)
		if err != nil {
			return nil, err
		}
		lists[i].Conflicts = len(conflicts)
	}
	if lists == nil {
		lists = []models.ImportedList{}
	}
	return lists, nil
}

// LibraryConflicts returns the learner's references in a library list (tag
// and every one of andTags) that still have the learner's own changes, made
// before the library word last changed: the library glosses now next to the
// glosses the learner sees.
func (s *Store) LibraryConflicts(ctx context.Context, userID int64, tag string, andTags []string) ([]models.LibraryConflict, error) {
	words, err := s.listLibraryWords(ctx, tag, andTags)
	if err != nil {
		return nil, err
	}
	inList := map[int64]bool{}
	for _, w := range words {
		inList[w.id] = true
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.text, w.library_word_id FROM words w
		 JOIN words l ON l.id = w.library_word_id
		 WHERE w.user_id = ? AND w.overrides_updated_at IS NOT NULL
		   AND l.library_updated_at > w.overrides_updated_at
		   AND (EXISTS (SELECT 1 FROM translation_deletions d WHERE d.user_word_id = w.id)
		        OR EXISTS (SELECT 1 FROM translations t WHERE t.zh_word_id = w.id))
		 ORDER BY w.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("load conflicts: %w", err)
	}
	type ref struct {
		id, libraryID int64
		text          string
	}
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.id, &r.text, &r.libraryID); err != nil {
			rows.Close()
			return nil, err
		}
		if inList[r.libraryID] {
			refs = append(refs, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := []models.LibraryConflict{}
	for _, r := range refs {
		library, err := visibleLibraryGlosses(ctx, s.db, userID, r.libraryID)
		if err != nil {
			return nil, err
		}
		c := models.LibraryConflict{WordID: r.id, ZhText: r.text, Library: map[string][]string{}}
		for key := range library {
			c.Library[key.lang] = append(c.Library[key.lang], key.text)
		}
		for lang := range c.Library {
			sort.Strings(c.Library[lang])
		}
		if c.Mine, _, err = referenceGlosses(ctx, s.db, r.id); err != nil {
			return nil, err
		}
		for lang := range c.Mine {
			sort.Strings(c.Mine[lang])
		}
		out = append(out, c)
	}
	return out, nil
}

// ResolveLibraryConflicts settles conflicts on the learner's references:
// keep "mine" marks the learner's changes as seen, so the conflict goes away;
// keep "library" removes the learner's changes. Progress stays in both cases.
func (s *Store) ResolveLibraryConflicts(ctx context.Context, userID int64, wordIDs []int64, keep string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	for _, id := range wordIDs {
		var isRef bool
		err := tx.QueryRowContext(ctx,
			`SELECT library_word_id IS NOT NULL FROM words WHERE id = ? AND user_id = ? AND language = 'zh'`, id, userID).Scan(&isRef)
		if err == sql.ErrNoRows || (err == nil && !isRef) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check reference: %w", err)
		}
		switch keep {
		case "mine":
			_, err = tx.ExecContext(ctx, `UPDATE words SET overrides_updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
		case "library":
			if _, err = tx.ExecContext(ctx, `DELETE FROM translation_deletions WHERE user_word_id = ?`, id); err == nil {
				if _, err = tx.ExecContext(ctx, `DELETE FROM translations WHERE zh_word_id = ?`, id); err == nil {
					_, err = tx.ExecContext(ctx, `UPDATE words SET overrides_updated_at = NULL WHERE id = ?`, id)
				}
			}
		default:
			return fmt.Errorf("keep must be mine or library, got %q", keep)
		}
		if err != nil {
			return fmt.Errorf("resolve conflict: %w", err)
		}
	}
	return tx.Commit()
}

// ensureLibraryWord returns the library word for a zh text, creating it from
// the dictionaries when the library does not have it yet (an untagged word,
// like a subword or a word a learner adds by hand). ok is false when neither
// the library nor the dictionaries have glosses for the text, for example a
// sentence: such a word stays the learner's own entry.
func (s *Store) ensureLibraryWord(ctx context.Context, text string) (id int64, ok bool, err error) {
	ids, err := s.ensureLibraryWords(ctx, []string{text})
	if err != nil {
		return 0, false, err
	}
	id, ok = ids[text]
	return id, ok, nil
}

// ensureLibraryWords is ensureLibraryWord for many texts: it returns text →
// library word id for every text that has library glosses afterwards.
func (s *Store) ensureLibraryWords(ctx context.Context, texts []string) (map[string]int64, error) {
	existing := map[string]libraryWord{}
	for start := 0; start < len(texts); start += lookupBatchChunk {
		chunk := texts[start:min(start+lookupBatchChunk, len(texts))]
		args := []any{LibraryUserID}
		for _, t := range chunk {
			args = append(args, t)
		}
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, text, library_removed FROM words WHERE user_id = ? AND language = 'zh'
			   AND text IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("find library words: %w", err)
		}
		for rows.Next() {
			var w libraryWord
			if err := rows.Scan(&w.id, &w.text, &w.removed); err != nil {
				rows.Close()
				return nil, err
			}
			existing[w.text] = w
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	// Words the library lacks: add those the dictionaries have.
	var missing []string
	for _, t := range texts {
		if _, ok := existing[t]; !ok {
			missing = append(missing, t)
		}
	}
	dict, err := s.LookupDictionaryBatch(ctx, missing, libraryLangs)
	if err != nil {
		return nil, err
	}
	for _, t := range missing {
		if len(dict[t]) == 0 {
			continue
		}
		if _, ok := existing[t]; ok {
			continue
		}
		pinyin, err := s.LookupPinyin(ctx, t)
		if err != nil {
			return nil, err
		}
		// Library pinyin is written without spaces between syllables.
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO words (text, language, pinyin, user_id) VALUES (?, 'zh', ?, ?)`,
			t, strings.ReplaceAll(pinyin, " ", ""), LibraryUserID)
		if err != nil {
			return nil, fmt.Errorf("add library word: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		existing[t] = libraryWord{id: id, text: t}
	}

	// Fill glosses of library words that have none yet.
	ids := make([]int64, 0, len(existing))
	for _, w := range existing {
		ids = append(ids, w.id)
	}
	links, err := s.libraryLinks(ctx, ids)
	if err != nil {
		return nil, err
	}
	var empty []libraryWord
	for _, w := range existing {
		if len(links[w.id]) == 0 {
			empty = append(empty, w)
		}
	}
	if len(empty) > 0 {
		if _, err := s.refreshLibraryWords(ctx, empty); err != nil {
			return nil, err
		}
		if links, err = s.libraryLinks(ctx, ids); err != nil {
			return nil, err
		}
	}
	out := map[string]int64{}
	for t, w := range existing {
		if len(links[w.id]) > 0 {
			out[t] = w.id
		}
	}
	return out, nil
}

// linkReference makes the learner's zh word wordID a reference to libraryID
// when the word is new (no gloss links of its own yet), fills in the library
// pinyin when the learner gave none, and clears the word's tombstone. It
// returns the library id the word refers to, or 0 for an own entry.
func (s *Store) linkReference(ctx context.Context, tx *sql.Tx, userID, wordID, libraryID int64) (int64, error) {
	if libraryID != 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE words SET library_word_id = ?,
			        pinyin = COALESCE(NULLIF(pinyin, ''), (SELECT pinyin FROM words WHERE id = ?))
			 WHERE id = ? AND library_word_id IS NULL
			   AND NOT EXISTS (SELECT 1 FROM translations t WHERE t.zh_word_id = words.id)`,
			libraryID, libraryID, wordID); err != nil {
			return 0, fmt.Errorf("link reference: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM word_tombstones WHERE user_id = ? AND library_word_id = ?`, userID, libraryID); err != nil {
			return 0, fmt.Errorf("clear tombstone: %w", err)
		}
	}
	var ref sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT library_word_id FROM words WHERE id = ?`, wordID).Scan(&ref); err != nil {
		return 0, fmt.Errorf("check reference: %w", err)
	}
	return ref.Int64, nil
}
