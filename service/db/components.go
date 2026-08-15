package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// InitComponentsForWord adds a component_progress row for every component
// extracted from the hanzi decomposition of each Han rune in zhText.
// Components come from hanzi_decomposition.decomposition (one level deep)
// and are filtered by shouldKeepComponent (etymology label, with pinyin
// similarity fallback) plus the requirement that the component has a
// non-empty definition.
// Rows are INSERT OR IGNORE so calling this multiple times is safe.
// dueDate is copied from the origin zh word's sm2_progress.due_date.
func (s *Store) InitComponentsForWord(ctx context.Context, userID int64, zhText string, dueDate time.Time) error {
	return initComponentsForWord(ctx, s.db, userID, zhText, dueDate)
}

// querier is satisfied by both *sql.DB and *sql.Tx, letting initComponentsForWord
// (and the coverage helpers below) run either standalone or inside an enclosing
// transaction (e.g. CreateWord) — always on the single connection SQLite is
// capped to (db.SetMaxOpenConns(1)), never a fresh one that would deadlock
// against an open transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// wordRequiredComponents returns the deduped set of qualifying component
// characters for zhText: the direct sub-parts of each Han character's
// hanzi_decomposition, filtered by shouldKeepComponent (etymology label, with
// pinyin similarity fallback) and requiring the component to have its own
// non-empty dictionary definition. This is the same one-level "character
// breakdown" rule GetHanziDecomposition and train.js use to decide what's
// shown to a learner.
func wordRequiredComponents(ctx context.Context, q querier, zhText string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, r := range []rune(zhText) {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		var decomp, etymology, radical, parentPinyin sql.NullString
		err := q.QueryRowContext(ctx,
			`SELECT decomposition, etymology, radical, pinyin FROM hanzi_decomposition WHERE character = ?`,
			string(r),
		).Scan(&decomp, &etymology, &radical, &parentPinyin)
		if err == sql.ErrNoRows || !decomp.Valid || decomp.String == "" {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("decomp lookup %q: %w", string(r), err)
		}
		parentPy := parsePinyinJSON(parentPinyin.String)
		for _, comp := range extractComponents(decomp.String) {
			compStr := string(comp)
			if seen[compStr] {
				continue
			}
			var def, compPinyin sql.NullString
			err := q.QueryRowContext(ctx,
				`SELECT definition, pinyin FROM hanzi_decomposition WHERE character = ?`,
				compStr,
			).Scan(&def, &compPinyin)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("component def lookup %q: %w", compStr, err)
			}
			if !def.Valid || def.String == "" {
				continue
			}
			if !shouldKeepComponent(r, comp, etymology.String, radical.String, parentPy, parsePinyinJSON(compPinyin.String)) {
				continue
			}
			seen[compStr] = true
			out = append(out, compStr)
		}
	}
	return out, nil
}

// getComponentCoverageThreshold reads the user's target word-coverage
// percentage (0-100, default 0 = no filtering) — see selectComponentsForCoverage
// for how it decides which components enter training. A missing user_settings
// row (not yet created) is treated as 0, same as the column's default.
func getComponentCoverageThreshold(ctx context.Context, q querier, userID int64) (float64, error) {
	var threshold float64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(component_coverage_threshold, 0) FROM user_settings WHERE user_id = ?`,
		userID,
	).Scan(&threshold)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get component coverage threshold: %w", err)
	}
	return threshold, nil
}

// componentWordSets returns, for every qualifying component character across
// all of userID's zh words, the set of zh word IDs that require it — the same
// rule wordRequiredComponents applies per word, just accumulated. Also
// returns the total zh word count and a per-word component count map (word ID
// → number of trainable components; 0 means the word is always covered).
func componentWordSets(ctx context.Context, q querier, userID int64) (wordSets map[string]map[int64]bool, wordComponentCounts map[int64]int, totalWords int, err error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, text FROM words WHERE user_id = ? AND language = 'zh'`, userID)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("list zh words: %w", err)
	}
	type zhWord struct {
		id   int64
		text string
	}
	var words []zhWord
	for rows.Next() {
		var wd zhWord
		if err := rows.Scan(&wd.id, &wd.text); err != nil {
			rows.Close()
			return nil, nil, 0, fmt.Errorf("scan zh word: %w", err)
		}
		words = append(words, wd)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("list zh words rows: %w", err)
	}

	wordSets = make(map[string]map[int64]bool)
	wordComponentCounts = make(map[int64]int, len(words))
	for _, wd := range words {
		components, err := wordRequiredComponents(ctx, q, wd.text)
		if err != nil {
			return nil, nil, 0, err
		}
		wordComponentCounts[wd.id] = len(components)
		for _, c := range components {
			if wordSets[c] == nil {
				wordSets[c] = make(map[int64]bool)
			}
			wordSets[c][wd.id] = true
		}
	}
	return wordSets, wordComponentCounts, len(words), nil
}

// selectComponentsForCoverage greedily picks components — always the one that
// fully covers the most additional zh words next — until the count of fully-
// covered words reaches targetPct percent of totalWords, or no more progress
// is possible. A word is fully covered when every one of its trainable
// components has been selected. Words with zero trainable components
// (wordComponentCounts[id]==0) are counted as covered from the start.
// Ties are broken by character (ascending) for determinism.
func selectComponentsForCoverage(wordSets map[string]map[int64]bool, wordComponentCounts map[int64]int, totalWords int, targetPct float64) []string {
	if totalWords == 0 || targetPct <= 0 {
		return nil
	}
	// Integer target: ceil(targetPct/100 * totalWords) — "cover at least X% of words".
	target := int(math.Ceil(targetPct / 100 * float64(totalWords)))

	// remaining[wid] = number of components still unselected for that word.
	remaining := make(map[int64]int, len(wordComponentCounts))
	coveredCount := 0
	for wid, cnt := range wordComponentCounts {
		if cnt == 0 {
			coveredCount++
		} else {
			remaining[wid] = cnt
		}
	}

	if coveredCount >= target {
		return nil
	}

	candidates := make([]string, 0, len(wordSets))
	for c := range wordSets {
		candidates = append(candidates, c)
	}
	sort.Strings(candidates)

	var selected []string
	for coveredCount < target && len(candidates) > 0 {
		bestIdx, bestGain := -1, 0
		for i, c := range candidates {
			gain := 0
			for wid := range wordSets[c] {
				if r, ok := remaining[wid]; ok && r == 1 {
					// selecting c would fully cover this word
					gain++
				}
			}
			if gain > bestGain {
				bestGain = gain
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			break
		}
		best := candidates[bestIdx]
		selected = append(selected, best)
		for wid := range wordSets[best] {
			if r, ok := remaining[wid]; ok {
				if r == 1 {
					delete(remaining, wid)
					coveredCount++
				} else {
					remaining[wid] = r - 1
				}
			}
		}
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)
	}
	return selected
}

func initComponentsForWord(ctx context.Context, q querier, userID int64, zhText string, dueDate time.Time) error {
	dueDateStr := dueDate.UTC().Format("2006-01-02 15:04:05")
	components, err := wordRequiredComponents(ctx, q, zhText)
	if err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}

	threshold, err := getComponentCoverageThreshold(ctx, q, userID)
	if err != nil {
		return err
	}
	var selected map[string]bool
	if threshold > 0 {
		wordSets, wordComponentCounts, totalWords, err := componentWordSets(ctx, q, userID)
		if err != nil {
			return err
		}
		sel := selectComponentsForCoverage(wordSets, wordComponentCounts, totalWords, threshold)
		selected = make(map[string]bool, len(sel))
		for _, c := range sel {
			selected[c] = true
		}
	}

	for _, comp := range components {
		if threshold > 0 && !selected[comp] {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
			userID, comp, dueDateStr,
		); err != nil {
			return fmt.Errorf("init component %q: %w", comp, err)
		}
	}
	return nil
}

// componentCard is the internal representation (includes definitions per lang for answer checking).
type componentCard struct {
	Character   string
	Pinyin      string
	Definitions map[string]string // lowercase lang → definition
	Progress    models.ComponentProgress
}

// GetNextComponentCard returns the most-overdue component due today for the user,
// considering only characters that have a definition in at least one of langs.
// Returns nil if nothing is due.
func (s *Store) GetNextComponentCard(ctx context.Context, userID int64, langs []string) (*componentCard, error) {
	// Only return cards the user can answer in one of langs (defaulting to EN).
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	placeholders := make([]string, len(langs))
	langArgs := make([]any, len(langs))
	for i, lang := range langs {
		placeholders[i] = "?"
		langArgs[i] = strings.ToUpper(lang)
	}

	args := append([]any{userID}, langArgs...)

	var c componentCard
	var dueDateStr string
	var firstSeenDate sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.character, cp.due_date,
		       cp.repetitions, cp.easiness, cp.interval_days,
		       cp.total_correct, cp.total_attempts, cp.first_seen_date
		FROM component_progress cp
		WHERE cp.user_id = ?
		  AND EXISTS (SELECT 1 FROM hanzi_decomposition_translation
		              WHERE character = cp.character AND lang IN (`+strings.Join(placeholders, ",")+`) AND definition != '')
		  AND cp.due_date <= CURRENT_TIMESTAMP
		ORDER BY cp.due_date ASC
		LIMIT 1`,
		args...,
	).Scan(
		&c.Character, &dueDateStr,
		&c.Progress.Repetitions, &c.Progress.Easiness, &c.Progress.IntervalDays,
		&c.Progress.TotalCorrect, &c.Progress.TotalAttempts, &firstSeenDate,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get next component card: %w", err)
	}

	defs, err := s.GetComponentTranslations(ctx, userID, c.Character)
	if err != nil {
		return nil, err
	}
	c.Definitions = defs
	var rawPinyin sql.NullString
	_ = s.db.QueryRowContext(ctx, `SELECT pinyin FROM hanzi_decomposition WHERE character = ?`, c.Character).Scan(&rawPinyin)
	c.Pinyin = joinPinyinJSON(rawPinyin.String)
	c.Progress.UserID = userID
	c.Progress.Character = c.Character
	c.Progress.DueDate = dueDateStr
	if firstSeenDate.Valid {
		c.Progress.FirstSeenDate = &firstSeenDate.String
	}
	return &c, nil
}

// GetComponentDefinitions returns definitions for a character keyed by lowercase lang code.
// Each definition prefers the user's own override (user_id = userID) and falls back to
// the shared seeded default (user_id IS NULL). Missing/empty definitions are omitted.
func (s *Store) GetComponentDefinitions(ctx context.Context, userID int64, character string, langs []string) (map[string]string, error) {
	defs := make(map[string]string)
	for _, lang := range langs {
		langUpper := strings.ToUpper(lang)
		langLower := strings.ToLower(lang)
		var def string
		err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(definition, '') FROM hanzi_decomposition_translation
			 WHERE character = ? AND lang = ? AND (user_id = ? OR user_id IS NULL)
			 ORDER BY user_id IS NULL LIMIT 1`,
			character, langUpper, userID,
		).Scan(&def)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("get %s definition for %q: %w", langLower, character, err)
		}
		if def != "" {
			defs[langLower] = def
		}
	}
	return defs, nil
}

// StoreComponentTranslation upserts a per-user override translation for a hanzi
// component character. Seeded shared defaults (user_id IS NULL) are never touched.
func (s *Store) StoreComponentTranslation(ctx context.Context, userID int64, character, lang, definition string) error {
	langUpper := strings.ToUpper(lang)
	res, err := s.db.ExecContext(ctx,
		`UPDATE hanzi_decomposition_translation SET definition = ?
		 WHERE character = ? AND lang = ? AND user_id = ?`,
		definition, character, langUpper, userID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition, user_id)
		 VALUES (?, ?, ?, ?)`,
		character, langUpper, definition, userID,
	)
	return err
}

// GetComponentTranslations returns all language translations for a component character,
// keyed by lowercase language code. The user's own overrides take precedence over the
// shared seeded defaults.
func (s *Store) GetComponentTranslations(ctx context.Context, userID int64, character string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT lang, definition, user_id FROM hanzi_decomposition_translation
		 WHERE character = ? AND (user_id = ? OR user_id IS NULL)`,
		character, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	global := make(map[string]string)
	userDefs := make(map[string]string)
	for rows.Next() {
		var lang, def string
		var uid *int64
		if err := rows.Scan(&lang, &def, &uid); err != nil {
			return nil, err
		}
		if uid != nil {
			userDefs[strings.ToLower(lang)] = def
		} else {
			global[strings.ToLower(lang)] = def
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for lang, def := range userDefs {
		global[lang] = def // user override wins
	}
	return global, nil
}

// MarkComponentForReview sets needs_review = 1 for a component_progress row.
func (s *Store) MarkComponentForReview(userID int64, character string) error {
	_, err := s.db.Exec(
		`UPDATE component_progress SET needs_review = 1 WHERE user_id = ? AND character = ?`,
		userID, character,
	)
	return err
}

// MarkComponentSeen sets first_seen_date = date('now') if it is currently NULL.
func (s *Store) MarkComponentSeen(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET first_seen_date = COALESCE(first_seen_date, date('now'))
		 WHERE user_id = ? AND character = ?`,
		userID, character)
	return err
}

// SkipComponent moves a component's due date forward by the given number of days
// without touching attempt counters or SM-2 state.
func (s *Store) SkipComponent(ctx context.Context, userID int64, character string, days int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET due_date = datetime('now', ?)
		 WHERE user_id = ? AND character = ?`,
		fmt.Sprintf("+%d days", days), userID, character)
	if err != nil {
		return fmt.Errorf("skip component: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetComponentProgress returns the total_correct/total_attempts counts for a
// component, or nil if no progress row exists yet.
func (s *Store) GetComponentProgress(ctx context.Context, userID int64, character string) (*models.ComponentProgress, error) {
	var p models.ComponentProgress
	err := s.db.QueryRowContext(ctx,
		`SELECT total_correct, total_attempts FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&p.TotalCorrect, &p.TotalAttempts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component progress: %w", err)
	}
	return &p, nil
}

// RecordComponentAnswer updates SM-2 state for a component after an answer.
// Returns the updated progress and the next due time.Time (for JSON responses).
func (s *Store) RecordComponentAnswer(ctx context.Context, userID int64, character string, correct bool) (models.ComponentProgress, time.Time, error) {
	var p models.ComponentProgress
	var dueDateStr string
	var firstSeenDate sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT repetitions, easiness, interval_days, due_date, total_correct, total_attempts, first_seen_date
		 FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDateStr,
		&p.TotalCorrect, &p.TotalAttempts, &firstSeenDate)
	if err == sql.ErrNoRows {
		return p, time.Time{}, fmt.Errorf("component progress not found for %q", character)
	}
	if err != nil {
		return p, time.Time{}, fmt.Errorf("get component progress: %w", err)
	}
	if firstSeenDate.Valid {
		p.FirstSeenDate = &firstSeenDate.String
	}

	sm2p := models.SM2Progress{
		Repetitions:   p.Repetitions,
		Easiness:      p.Easiness,
		IntervalDays:  p.IntervalDays,
		DueDate:       parseDateTime(dueDateStr),
		TotalCorrect:  p.TotalCorrect,
		TotalAttempts: p.TotalAttempts,
	}
	quality := sm2.QualityWrong
	if correct {
		quality = sm2.QualityCorrect
	}
	updated := sm2.Update(sm2p, quality)
	updated.TotalAttempts++
	if correct {
		updated.TotalCorrect++
	}

	newDue := updated.DueDate.UTC().Format("2006-01-02 15:04:05")

	// Save pre-answer state before applying wrong result so AcceptCorrect can restore it.
	var prevStateJSON sql.NullString
	if !correct {
		blob, merr := json.Marshal(componentPrevState{
			Repetitions:   p.Repetitions,
			Easiness:      p.Easiness,
			IntervalDays:  p.IntervalDays,
			TotalCorrect:  p.TotalCorrect,
			TotalAttempts: p.TotalAttempts,
		})
		if merr == nil {
			prevStateJSON = sql.NullString{String: string(blob), Valid: true}
		}
	}

	if correct {
		_, err = s.db.ExecContext(ctx,
			`UPDATE component_progress
			 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
			     total_correct = ?, total_attempts = ?,
			     first_seen_date = COALESCE(first_seen_date, date('now')),
			     prev_state = NULL
			 WHERE user_id = ? AND character = ?`,
			updated.Repetitions, updated.Easiness, updated.IntervalDays, newDue,
			updated.TotalCorrect, updated.TotalAttempts,
			userID, character,
		)
	} else {
		_, err = s.db.ExecContext(ctx,
			`UPDATE component_progress
			 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
			     total_correct = ?, total_attempts = ?,
			     first_seen_date = COALESCE(first_seen_date, date('now')),
			     prev_state = ?
			 WHERE user_id = ? AND character = ?`,
			updated.Repetitions, updated.Easiness, updated.IntervalDays, newDue,
			updated.TotalCorrect, updated.TotalAttempts,
			prevStateJSON,
			userID, character,
		)
	}
	if err != nil {
		return p, time.Time{}, fmt.Errorf("update component progress: %w", err)
	}

	p.UserID = userID
	p.Character = character
	p.Repetitions = updated.Repetitions
	p.Easiness = updated.Easiness
	p.IntervalDays = updated.IntervalDays
	p.DueDate = newDue
	p.TotalCorrect = updated.TotalCorrect
	p.TotalAttempts = updated.TotalAttempts
	return p, updated.DueDate, nil
}

// componentPrevState is the JSON-serialisable snapshot stored in component_progress.prev_state.
type componentPrevState struct {
	Repetitions   int     `json:"reps"`
	Easiness      float64 `json:"ef"`
	IntervalDays  int     `json:"iv"`
	TotalCorrect  int     `json:"tc"`
	TotalAttempts int     `json:"ta"`
}

// UpdateComponentProgress writes an SM-2 update to component_progress and clears
// prev_state. Used by AcceptCorrect after applying a correct-quality update.
func (s *Store) UpdateComponentProgress(ctx context.Context, userID int64, character string, p models.SM2Progress) error {
	newDue := p.DueDate.UTC().Format("2006-01-02 15:04:05")
	_, err := s.db.ExecContext(ctx,
		`UPDATE component_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?, prev_state = NULL
		 WHERE user_id = ? AND character = ?`,
		p.Repetitions, p.Easiness, p.IntervalDays, newDue,
		p.TotalCorrect, p.TotalAttempts,
		userID, character,
	)
	return err
}

// ComponentListItem is one row in the component list view.
type ComponentListItem struct {
	Character     string  `json:"character"`
	Pinyin        string  `json:"pinyin,omitempty"`
	DefinitionEN  string  `json:"definition_en"`
	DefinitionDE  string  `json:"definition_de"`
	DueDate       string  `json:"due_date"`
	TotalCorrect  int     `json:"total_correct"`
	TotalAttempts int     `json:"total_attempts"`
	Easiness      float64 `json:"easiness"`
	IntervalDays  int     `json:"interval_days"`
	FirstSeenDate *string `json:"first_seen_date,omitempty"`
	IsAlsoWord    bool    `json:"is_also_word,omitempty"`
}

// GetComponentList returns a paginated list of component_progress rows for a user,
// optionally filtered by a search string matched against character or EN definition,
// and optionally restricted to rows with needs_review = 1.
func (s *Store) GetComponentList(ctx context.Context, userID int64, search string, page, perPage int, reviewOnly bool) ([]ComponentListItem, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	var args []any
	whereExtra := ""
	if search != "" {
		whereExtra += " AND (cp.character LIKE ? OR LOWER(hdt_en.definition) LIKE LOWER(?))"
		like := "%" + search + "%"
		args = append(args, like, like)
	}
	if reviewOnly {
		whereExtra += " AND cp.needs_review = 1"
	}

	countArgs := append([]any{userID}, args...)
	var total int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM component_progress cp
		LEFT JOIN hanzi_decomposition_translation hdt_en
		       ON hdt_en.character = cp.character AND hdt_en.lang = 'EN'
		WHERE cp.user_id = ?`+whereExtra,
		countArgs...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count components: %w", err)
	}

	listArgs := append([]any{userID}, args...)
	listArgs = append(listArgs, perPage, offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT cp.character,
		       COALESCE(hd.pinyin, '') AS pinyin,
		       COALESCE(hdt_en.definition, '') AS def_en,
		       COALESCE(hdt_de.definition, '') AS def_de,
		       date(cp.due_date) AS due_date,
		       cp.total_correct, cp.total_attempts,
		       cp.easiness, cp.interval_days,
		       cp.first_seen_date,
		       w.id IS NOT NULL AS is_also_word
		FROM component_progress cp
		LEFT JOIN hanzi_decomposition hd ON hd.character = cp.character
		LEFT JOIN hanzi_decomposition_translation hdt_en
		       ON hdt_en.character = cp.character AND hdt_en.lang = 'EN'
		LEFT JOIN hanzi_decomposition_translation hdt_de
		       ON hdt_de.character = cp.character AND hdt_de.lang = 'DE'
		LEFT JOIN words w ON w.text = cp.character AND w.user_id = cp.user_id AND w.language = 'zh'
		WHERE cp.user_id = ?`+whereExtra+`
		ORDER BY cp.due_date ASC
		LIMIT ? OFFSET ?`,
		listArgs...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list components: %w", err)
	}
	var items []ComponentListItem
	for rows.Next() {
		var it ComponentListItem
		var rawPinyin string
		var firstSeen sql.NullString
		if err := rows.Scan(
			&it.Character, &rawPinyin, &it.DefinitionEN, &it.DefinitionDE,
			&it.DueDate, &it.TotalCorrect, &it.TotalAttempts,
			&it.Easiness, &it.IntervalDays, &firstSeen, &it.IsAlsoWord,
		); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("scan component list: %w", err)
		}
		it.Pinyin = joinPinyinJSON(rawPinyin)
		if firstSeen.Valid {
			it.FirstSeenDate = &firstSeen.String
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("component list rows: %w", err)
	}
	return items, total, nil
}

func joinPinyinJSON(s string) string {
	if s == "" {
		return ""
	}
	var parts []string
	if err := json.Unmarshal([]byte(s), &parts); err != nil {
		return ""
	}
	return strings.Join(parts, " / ")
}

// GetComponentPinyin returns the first pinyin value (e.g. "mù") for a component
// character by reading hanzi_decomposition.pinyin (a JSON array). Returns "" if
// no pinyin is stored.
func (s *Store) GetComponentPinyin(ctx context.Context, character string) string {
	var raw sql.NullString
	_ = s.db.QueryRowContext(ctx,
		`SELECT pinyin FROM hanzi_decomposition WHERE character = ?`, character,
	).Scan(&raw)
	return joinPinyinJSON(raw.String)
}

// GetComponentCountByDueDate returns the number of seen components grouped by
// due date, covering overdue (grouped as today), today, and the next 30 days.
// Unseen components (first_seen_date IS NULL) are excluded.
func (s *Store) GetComponentCountByDueDate(ctx context.Context, userID int64) ([]models.DueDateCount, error) {
	query := `SELECT
		CASE
			WHEN date(cp.due_date) <= date('now') THEN date('now')
			ELSE date(cp.due_date)
		END AS bucket_date,
		COUNT(*) AS cnt
	FROM component_progress cp
	WHERE cp.user_id = ?
	  AND cp.first_seen_date IS NOT NULL
	  AND date(cp.due_date) <= date('now', '+30 days')
	GROUP BY bucket_date
	ORDER BY bucket_date`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get component count by due date: %w", err)
	}
	defer rows.Close()
	var result []models.DueDateCount
	for rows.Next() {
		var d models.DueDateCount
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, fmt.Errorf("scan component due date count: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetComponentCounts returns the number of components due today and the total
// number of components in training for the given user. dueToday is filtered
// to components with a translation in one of langs (defaulting to EN when
// langs is empty), matching the language filter GetNextComponentCard uses —
// otherwise a component due only in a non-English language would be served
// as a card while being undercounted here (#230, #232).
func (s *Store) GetComponentCounts(ctx context.Context, userID int64, langs []string) (dueToday, total int, err error) {
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	placeholders := make([]string, len(langs))
	langArgs := make([]any, len(langs))
	for i, lang := range langs {
		placeholders[i] = "?"
		langArgs[i] = strings.ToUpper(lang)
	}

	args := append([]any{userID}, langArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress cp
		 WHERE cp.user_id = ?
		   AND EXISTS (SELECT 1 FROM hanzi_decomposition_translation
		               WHERE character = cp.character AND lang IN (`+strings.Join(placeholders, ",")+`) AND definition != '')
		   AND cp.due_date < date('now', '+1 day')`,
		args...,
	).Scan(&dueToday)
	if err != nil {
		return 0, 0, fmt.Errorf("get component due count: %w", err)
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress WHERE user_id = ?`,
		userID,
	).Scan(&total)
	if err != nil {
		return 0, 0, fmt.Errorf("get component total count: %w", err)
	}
	return dueToday, total, nil
}
