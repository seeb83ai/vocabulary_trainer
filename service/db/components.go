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
	return initComponentsForWord(ctx, s.db, userID, zhText, dueDate)
}

// querier is satisfied by both *sql.DB and *sql.Tx, letting initComponentsForWord
// run either standalone or inside an enclosing transaction (e.g. CreateWord).
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func initComponentsForWord(ctx context.Context, q querier, userID int64, zhText string, dueDate time.Time) error {
	dueDateStr := dueDate.UTC().Format("2006-01-02 15:04:05")
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
			return fmt.Errorf("decomp lookup %q: %w", string(r), err)
		}
		parentPy := parsePinyinJSON(parentPinyin.String)
		for _, comp := range extractComponents(decomp.String) {
			var def, compPinyin sql.NullString
			err := q.QueryRowContext(ctx,
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
			if _, err := q.ExecContext(ctx,
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
		whereFrags = append(whereFrags, "EXISTS (SELECT 1 FROM hanzi_decomposition_translation WHERE character = cp.character AND lang = ? AND definition != '')")
		langArgs = append(langArgs, strings.ToUpper(lang))
	}
	if len(whereFrags) == 0 {
		whereFrags = []string{"EXISTS (SELECT 1 FROM hanzi_decomposition_translation WHERE character = cp.character AND lang = 'EN' AND definition != '')"}
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
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, strings.ToUpper(lang), definition)
	return err
}

// SeedHanziDecompositionForTest inserts a hanzi_decomposition row with definition
// and also seeds the EN translation table entry. Intended for use in tests only.
func (s *Store) SeedHanziDecompositionForTest(ctx context.Context, character, definition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition) VALUES (?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition`,
		character, definition); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SeedHanziDecompositionWithDecompForTest inserts a hanzi_decomposition row with
// definition and decomposition string, and also seeds the EN translation table entry.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithDecompForTest(ctx context.Context, character, definition, decomposition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, decomposition) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, decomposition = excluded.decomposition`,
		character, definition, decomposition); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
	return err
}

// SeedHanziDecompositionWithPinyinForTest inserts a hanzi_decomposition row with
// definition and a JSON-encoded pinyin array, and seeds the EN translation entry.
// Intended for use in tests only.
func (s *Store) SeedHanziDecompositionWithPinyinForTest(ctx context.Context, character, definition, pinyinJSON string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition (character, definition, pinyin) VALUES (?, ?, ?)
		 ON CONFLICT(character) DO UPDATE SET definition = excluded.definition, pinyin = excluded.pinyin`,
		character, definition, pinyinJSON); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hanzi_decomposition_translation (character, lang, definition) VALUES (?, 'EN', ?)
		 ON CONFLICT(character, lang) WHERE user_id IS NULL DO UPDATE SET definition = excluded.definition`,
		character, definition)
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

// GetComponentHMMSceneText returns the mnemonic scene text for a component character
// for the given user, or "" if none has been saved.
func (s *Store) GetComponentHMMSceneText(ctx context.Context, userID int64, character string) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get component hmm scene: %w", err)
	}
	return text, nil
}

// UpsertComponentHMMScene saves (or replaces) the mnemonic scene text for a
// component character for the given user.
func (s *Store) UpsertComponentHMMScene(ctx context.Context, userID int64, character, sceneText string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO component_hmm_scenes (character, user_id, scene_text) VALUES (?, ?, ?)
		 ON CONFLICT(character, user_id) DO UPDATE SET scene_text = excluded.scene_text`,
		character, userID, sceneText,
	)
	if err != nil {
		return fmt.Errorf("upsert component hmm scene: %w", err)
	}
	return nil
}

// DeleteComponentHMMScene removes the mnemonic scene for a component character
// for the given user. It is a no-op if no scene exists.
func (s *Store) DeleteComponentHMMScene(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	)
	if err != nil {
		return fmt.Errorf("delete component hmm scene: %w", err)
	}
	return nil
}

// GetComponentHMMSceneRecord returns the saved mnemonic scene for a component character
// as a models.HMMScene, or nil if none exists.
func (s *Store) GetComponentHMMSceneRecord(ctx context.Context, userID int64, character string) (*models.HMMScene, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component hmm scene: %w", err)
	}
	return &models.HMMScene{SceneText: text}, nil
}

// SaveComponentHMMSceneWithLibrary saves the mnemonic scene for a component character
// and updates the shared actor/location/room/props library entries.
func (s *Store) SaveComponentHMMSceneWithLibrary(ctx context.Context, userID int64, character, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO component_hmm_scenes (user_id, character, scene_text) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, character) DO UPDATE SET scene_text = excluded.scene_text`,
		userID, character, req.SceneText); err != nil {
		return fmt.Errorf("upsert component scene: %w", err)
	}

	if req.ActorName != "" && initial != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_actors SET actor_name = ? WHERE user_id = ? AND initial = ?`,
			req.ActorName, userID, initial); err != nil {
			return fmt.Errorf("update actor: %w", err)
		}
	}
	if req.LocationName != "" && finalKey != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_locations SET location_name = ? WHERE user_id = ? AND final_key = ?`,
			req.LocationName, userID, finalKey); err != nil {
			return fmt.Errorf("update location: %w", err)
		}
	}
	if req.RoomName != "" && tone >= 1 && tone <= 5 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_tone_rooms SET room_name = ? WHERE user_id = ? AND tone = ?`,
			req.RoomName, userID, tone); err != nil {
			return fmt.Errorf("update tone room: %w", err)
		}
	}
	for _, p := range req.Props {
		if p.Radical == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO hmm_props (user_id, radical, prop_name) VALUES (?, ?, ?)
			 ON CONFLICT(user_id, radical) DO UPDATE SET prop_name = excluded.prop_name`,
			userID, p.Radical, p.PropName); err != nil {
			return fmt.Errorf("upsert prop %s: %w", p.Radical, err)
		}
	}

	return tx.Commit()
}

// componentPrevState is the JSON-serialisable snapshot stored in component_progress.prev_state.
type componentPrevState struct {
	Repetitions   int     `json:"reps"`
	Easiness      float64 `json:"ef"`
	IntervalDays  int     `json:"iv"`
	TotalCorrect  int     `json:"tc"`
	TotalAttempts int     `json:"ta"`
}

// SaveComponentPrevState serialises p to JSON and stores it in the prev_state column
// of component_progress. Called before applying a wrong answer so AcceptCorrect can
// restore the pre-answer state without trusting client data.
func (s *Store) SaveComponentPrevState(ctx context.Context, userID int64, character string, p models.ComponentProgress) error {
	blob, err := json.Marshal(componentPrevState{
		Repetitions:   p.Repetitions,
		Easiness:      p.Easiness,
		IntervalDays:  p.IntervalDays,
		TotalCorrect:  p.TotalCorrect,
		TotalAttempts: p.TotalAttempts,
	})
	if err != nil {
		return fmt.Errorf("marshal component prev state: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE component_progress SET prev_state = ? WHERE user_id = ? AND character = ?`,
		string(blob), userID, character)
	return err
}

// GetComponentPrevState reads the stored pre-answer state for a component.
// Returns nil, nil when no previous state is stored.
func (s *Store) GetComponentPrevState(ctx context.Context, userID int64, character string) (*models.ComponentProgress, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT prev_state FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component prev state: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var prev componentPrevState
	if err := json.Unmarshal([]byte(raw.String), &prev); err != nil {
		return nil, fmt.Errorf("unmarshal component prev state: %w", err)
	}
	return &models.ComponentProgress{
		UserID:        userID,
		Character:     character,
		Repetitions:   prev.Repetitions,
		Easiness:      prev.Easiness,
		IntervalDays:  prev.IntervalDays,
		TotalCorrect:  prev.TotalCorrect,
		TotalAttempts: prev.TotalAttempts,
	}, nil
}

// ClearComponentPrevState sets prev_state = NULL for the given component.
// Called after a correct answer or after AcceptCorrect.
func (s *Store) ClearComponentPrevState(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE component_progress SET prev_state = NULL WHERE user_id = ? AND character = ?`,
		userID, character)
	return err
}

// SetComponentAttemptsForTest sets total_attempts for a component_progress row.
// Intended for use in tests only.
func (s *Store) SetComponentAttemptsForTest(ctx context.Context, userID int64, character string, attempts int) {
	s.db.ExecContext(ctx, //nolint:errcheck
		`UPDATE component_progress SET total_attempts = ? WHERE user_id = ? AND character = ?`,
		attempts, userID, character)
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

// GetComponentProgressForTest reads a component_progress row directly.
// Intended for use in tests only.
func (s *Store) GetComponentProgressForTest(ctx context.Context, userID int64, character string) (models.ComponentProgress, time.Time, error) {
	var p models.ComponentProgress
	var dueDateStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT repetitions, easiness, interval_days, due_date, total_correct, total_attempts
		 FROM component_progress WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&p.Repetitions, &p.Easiness, &p.IntervalDays, &dueDateStr,
		&p.TotalCorrect, &p.TotalAttempts)
	if err != nil {
		return p, time.Time{}, err
	}
	return p, parseDateTime(dueDateStr), nil
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
		       cp.first_seen_date
		FROM component_progress cp
		LEFT JOIN hanzi_decomposition hd ON hd.character = cp.character
		LEFT JOIN hanzi_decomposition_translation hdt_en
		       ON hdt_en.character = cp.character AND hdt_en.lang = 'EN'
		LEFT JOIN hanzi_decomposition_translation hdt_de
		       ON hdt_de.character = cp.character AND hdt_de.lang = 'DE'
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

// GetComponentCounts returns the number of components due today and the total
// number of components in training for the given user. dueToday is filtered
// to components with a translation in one of langs (defaulting to EN when
// langs is empty), matching the language filter GetNextComponentCard uses —
// otherwise a component due only in a non-English language would be served
// as a card while being undercounted here (#230, #232).
func (s *Store) GetComponentCounts(ctx context.Context, userID int64, langs []string) (dueToday, total int, err error) {
	whereFrags := []string{}
	langArgs := []any{}
	for _, lang := range langs {
		whereFrags = append(whereFrags, "EXISTS (SELECT 1 FROM hanzi_decomposition_translation WHERE character = cp.character AND lang = ? AND definition != '')")
		langArgs = append(langArgs, strings.ToUpper(lang))
	}
	if len(whereFrags) == 0 {
		whereFrags = []string{"EXISTS (SELECT 1 FROM hanzi_decomposition_translation WHERE character = cp.character AND lang = 'EN' AND definition != '')"}
	}
	langFilter := strings.Join(whereFrags, " OR ")

	args := append([]any{userID}, langArgs...)
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM component_progress cp
		 WHERE cp.user_id = ?
		   AND (`+langFilter+`)
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
