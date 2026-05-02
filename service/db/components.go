package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	dueDateStr := dueDate.UTC().Format("2006-01-02 15:04:05")
	for _, r := range []rune(zhText) {
		if !unicode.Is(unicode.Han, r) {
			continue
		}
		var decomp, etymology, radical, parentPinyin sql.NullString
		err := s.db.QueryRowContext(ctx,
			`SELECT decomposition, etymology, radical, pinyin FROM hanzi_decomposition WHERE character = ?`,
			string(r),
		).Scan(&decomp, &etymology, &radical, &parentPinyin)
		if err == sql.ErrNoRows || !decomp.Valid || decomp.String == "" {
			continue
		}
		if err != nil {
			return fmt.Errorf("decomp lookup %q: %w", string(r), err)
		}
		parentPy := parsePinyinJSON(parentPinyin.String)
		for _, comp := range extractComponents(decomp.String) {
			var def, compPinyin sql.NullString
			err := s.db.QueryRowContext(ctx,
				`SELECT definition, pinyin FROM hanzi_decomposition WHERE character = ?`,
				string(comp),
			).Scan(&def, &compPinyin)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return fmt.Errorf("component def lookup %q: %w", string(comp), err)
			}
			if !def.Valid || def.String == "" {
				continue
			}
			if !shouldKeepComponent(r, comp, etymology.String, radical.String, parentPy, parsePinyinJSON(compPinyin.String)) {
				continue
			}
			if _, err := s.db.ExecContext(ctx,
				`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
				userID, string(comp), dueDateStr,
			); err != nil {
				return fmt.Errorf("init component %q: %w", string(comp), err)
			}
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
	// Build a per-lang filter so we only return cards the user can answer.
	whereFrags := []string{}
	langArgs := []any{}
	for _, lang := range langs {
		switch strings.ToUpper(lang) {
		case "EN":
			whereFrags = append(whereFrags, "(hd.definition IS NOT NULL AND hd.definition != '')")
		default:
			whereFrags = append(whereFrags, "EXISTS (SELECT 1 FROM hanzi_decomposition_translation WHERE character = cp.character AND lang = ?)")
			langArgs = append(langArgs, strings.ToUpper(lang))
		}
	}
	if len(whereFrags) == 0 {
		whereFrags = []string{"(hd.definition IS NOT NULL AND hd.definition != '')"}
	}
	langFilter := strings.Join(whereFrags, " OR ")

	args := append([]any{userID}, langArgs...)

	var c componentCard
	var dueDateStr string
	var firstSeenDate sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT cp.character, cp.due_date,
		       cp.repetitions, cp.easiness, cp.interval_days,
		       cp.total_correct, cp.total_attempts, cp.first_seen_date
		FROM component_progress cp
		JOIN hanzi_decomposition hd ON hd.character = cp.character
		WHERE cp.user_id = ?
		  AND (`+langFilter+`)
		  AND cp.due_date < datetime('now', '+1 day')
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

	defs, err := s.GetComponentDefinitions(ctx, c.Character, langs)
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
// "en" is read from hanzi_decomposition.definition; other langs from hanzi_decomposition_translation.
// Missing or empty definitions are omitted from the result.
func (s *Store) GetComponentDefinitions(ctx context.Context, character string, langs []string) (map[string]string, error) {
	defs := make(map[string]string)
	for _, lang := range langs {
		langUpper := strings.ToUpper(lang)
		langLower := strings.ToLower(lang)
		var def string
		var err error
		if langUpper == "EN" {
			err = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(definition, '') FROM hanzi_decomposition WHERE character = ?`, character,
			).Scan(&def)
		} else {
			err = s.db.QueryRowContext(ctx,
				`SELECT COALESCE(definition, '') FROM hanzi_decomposition_translation WHERE character = ? AND lang = ?`,
				character, langUpper,
			).Scan(&def)
		}
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("get %s definition for %q: %w", langLower, character, err)
		}
		if def != "" {
			defs[langLower] = def
		}
	}
	return defs, nil
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
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_progress
		 SET repetitions = ?, easiness = ?, interval_days = ?, due_date = ?,
		     total_correct = ?, total_attempts = ?,
		     first_seen_date = COALESCE(first_seen_date, date('now'))
		 WHERE user_id = ? AND character = ?`,
		updated.Repetitions, updated.Easiness, updated.IntervalDays, newDue,
		updated.TotalCorrect, updated.TotalAttempts,
		userID, character,
	)
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

// RecordComponentStat increments today's correct or wrong count in component_stats
// and snapshots the current total number of components in training for the user.
func (s *Store) RecordComponentStat(ctx context.Context, userID int64, correct bool) error {
	col := "wrong"
	if correct {
		col = "correct"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO component_stats (user_id, date, correct, wrong, components_total) VALUES (?, date('now'), 0, 0, 0)
		 ON CONFLICT(user_id, date) DO NOTHING`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("upsert component_stats row: %w", err)
	}
	if _, err = s.db.ExecContext(ctx,
		`UPDATE component_stats SET `+col+` = `+col+` + 1 WHERE user_id = ? AND date = date('now')`,
		userID,
	); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_stats
		    SET components_total = (SELECT COUNT(*) FROM component_progress WHERE user_id = ?)
		  WHERE user_id = ? AND date = date('now')`,
		userID, userID,
	)
	return err
}

// GetComponentStatsHistory returns daily component training stats for a user.
func (s *Store) GetComponentStatsHistory(ctx context.Context, userID int64) ([]models.ComponentDailyStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT date, correct, wrong, components_total FROM component_stats WHERE user_id = ? ORDER BY date ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get component stats history: %w", err)
	}
	var stats []models.ComponentDailyStat
	for rows.Next() {
		var s models.ComponentDailyStat
		if err := rows.Scan(&s.Date, &s.Correct, &s.Wrong, &s.ComponentsTotal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan component stat: %w", err)
		}
		stats = append(stats, s)
	}
	rows.Close()
	return stats, rows.Err()
}

// SeedHanziTranslationForTest inserts a hanzi_decomposition_translation row.
// Intended for use in tests only.
func (s *Store) SeedHanziTranslationForTest(ctx context.Context, character, lang, definition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, ?, ?)
		 ON CONFLICT(character, lang) DO UPDATE SET definition = excluded.definition`,
		character, strings.ToUpper(lang), definition)
	return err
}

// SeedHanziDecompositionForTest inserts a hanzi_decomposition row with definition.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionForTest(ctx context.Context, character, definition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SeedHanziDecompositionWithDecompForTest inserts a hanzi_decomposition row with
// definition and decomposition string (IDS operators + component characters).
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithDecompForTest(ctx context.Context, character, definition, decomposition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, decomposition) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, decomposition = excluded.decomposition`,
		character, definition, decomposition)
	return err
}

// SeedHanziDecompositionWithPinyinForTest inserts a hanzi_decomposition row with
// definition and a JSON-encoded pinyin array. Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithPinyinForTest(ctx context.Context, character, definition, pinyinJSON string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, pinyin) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, pinyin = excluded.pinyin`,
		character, definition, pinyinJSON)
	return err
}

// SetComponentSeenForTest marks a component as seen. Intended for use in tests only.
func (s *Store) SetComponentSeenForTest(ctx context.Context, userID int64, character string) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET first_seen_date = date('now') WHERE user_id = ? AND character = ?`,
		userID, character)
}

// InsertComponentProgressForTest inserts a component_progress row directly.
// Intended for use in tests only.
func (s *Store) InsertComponentProgressForTest(ctx context.Context, userID int64, character string, dueDate time.Time) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`INSERT OR IGNORE INTO component_progress (user_id, character, due_date) VALUES (?, ?, ?)`,
		userID, character, dueDate.UTC().Format("2006-01-02 15:04:05"))
}

// SetComponentAttemptsForTest sets total_attempts for a component_progress row.
// Intended for use in tests only.
func (s *Store) SetComponentAttemptsForTest(ctx context.Context, userID int64, character string, attempts int) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET total_attempts = ? WHERE user_id = ? AND character = ?`,
		attempts, userID, character)
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
}

// GetComponentList returns a paginated list of component_progress rows for a user,
// optionally filtered by a search string matched against character or EN definition.
func (s *Store) GetComponentList(ctx context.Context, userID int64, search string, page, perPage int) ([]ComponentListItem, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * perPage

	var args []any
	whereExtra := ""
	if search != "" {
		whereExtra = " AND (cp.character LIKE ? OR LOWER(hd.definition) LIKE LOWER(?))"
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	countArgs := append([]any{userID}, args...)
	var total int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM component_progress cp
		JOIN hanzi_decomposition hd ON hd.character = cp.character
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
		       COALESCE(hd.definition, '') AS def_en,
		       COALESCE(hdt.definition, '') AS def_de,
		       date(cp.due_date) AS due_date,
		       cp.total_correct, cp.total_attempts,
		       cp.easiness, cp.interval_days,
		       cp.first_seen_date
		FROM component_progress cp
		JOIN hanzi_decomposition hd ON hd.character = cp.character
		LEFT JOIN hanzi_decomposition_translation hdt
		       ON hdt.character = cp.character AND hdt.lang = 'DE'
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
			&it.Easiness, &it.IntervalDays, &firstSeen,
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

// GetComponentCounts returns the number of components due today and the total
// number of components in training for the given user.
func (s *Store) GetComponentCounts(ctx context.Context, userID int64) (dueToday, total int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress cp
		 JOIN hanzi_decomposition hd ON hd.character = cp.character
		 WHERE cp.user_id = ?
		   AND hd.definition IS NOT NULL AND hd.definition != ''
		   AND cp.due_date < date('now', '+1 day')`,
		userID,
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
