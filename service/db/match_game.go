package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"
	"vocabulary_trainer/models"
)

// This file holds the word-based match-game modes added in issue #288
// (newest / hardest / last-mistakes), which sit alongside the pre-existing
// mismatch mode in quiz.go. Keeping them separate from quiz.go's core SM-2
// logic follows CLAUDE.md's one-logical-domain-per-file convention.

// wordToMatchGameWord resolves a zh word id into a MatchGameWord, hydrating
// its translations across every known language (mirrors the word branch of
// resolveConfusionEntity). ok=false means the word no longer exists for this
// user and the caller should skip it.
func (s *Store) wordToMatchGameWord(ctx context.Context, userID, wordID int64, langs []string) (word models.MatchGameWord, ok bool, err error) {
	var text string
	var pinyin sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT text, pinyin FROM words WHERE id = ? AND user_id = ?`, wordID, userID).
		Scan(&text, &pinyin)
	if err == sql.ErrNoRows {
		return models.MatchGameWord{}, false, nil
	}
	if err != nil {
		return models.MatchGameWord{}, false, fmt.Errorf("resolve match-game word %d: %w", wordID, err)
	}
	translations := map[string][]string{}
	for _, lang := range langs {
		texts, terr := s.getTranslationTextsForZhWord(ctx, wordID, lang)
		if terr != nil {
			return models.MatchGameWord{}, false, terr
		}
		if len(texts) > 0 {
			translations[lang] = texts
		}
	}
	word = models.MatchGameWord{
		Kind:         models.ConfusionKindWord,
		ZhWordID:     wordID,
		ZhText:       text,
		Translations: translations,
	}
	if pinyin.Valid {
		word.Pinyin = pinyin.String
	}
	return word, true, nil
}

// wordIDsToMatchGameWords resolves a slice of zh word ids (already ordered by
// the caller) into hydrated MatchGameWords, skipping any id that no longer
// resolves to a word.
func (s *Store) wordIDsToMatchGameWords(ctx context.Context, userID int64, ids []int64) ([]models.MatchGameWord, error) {
	if len(ids) == 0 {
		return []models.MatchGameWord{}, nil
	}
	langs, err := s.GetTranslationLanguages(ctx)
	if err != nil {
		return nil, err
	}
	words := make([]models.MatchGameWord, 0, len(ids))
	for _, id := range ids {
		w, ok, err := s.wordToMatchGameWord(ctx, userID, id, langs)
		if err != nil {
			return nil, err
		}
		if ok {
			words = append(words, w)
		}
	}
	return words, nil
}

// newestWordsGamePoolSize is how many of the user's most-recently-created zh
// words form the eligible pool for the "newest words" game mode.
const newestWordsGamePoolSize = 30

// GetNewestWordsForGame returns up to `count` distinct zh words drawn from the
// newestWordsGamePoolSize most recently created words for the user, via
// weighted random sampling without replacement. Needs no "shown" bookkeeping
// (unlike the other modes): a word simply ages out of the pool as newer words
// are added, and even within the pool its own pick weight only ever falls, so
// nothing needs to be recorded after a round is shown.
func (s *Store) GetNewestWordsForGame(ctx context.Context, userID int64, count int) ([]models.MatchGameWord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM words
		WHERE language = 'zh' AND user_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, userID, newestWordsGamePoolSize)
	if err != nil {
		return nil, fmt.Errorf("get newest words pool: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan newest word id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	picked := weightedSampleWithoutReplacement(ids, count)
	return s.wordIDsToMatchGameWords(ctx, userID, picked)
}

// weightedSampleWithoutReplacement draws up to n distinct entries from ids
// (already ordered newest-first, i.e. index 0 = rank 0 = newest). Each
// remaining candidate's draw weight is 1/(rank+1) — a simple harmonic decay
// so probability strictly decreases with age inside the pool while every
// pool member keeps some chance of being picked.
func weightedSampleWithoutReplacement(ids []int64, n int) []int64 {
	if n > len(ids) {
		n = len(ids)
	}
	remaining := make([]int64, len(ids))
	copy(remaining, ids)
	weights := make([]float64, len(ids))
	for i := range weights {
		weights[i] = 1.0 / float64(i+1)
	}

	picked := make([]int64, 0, n)
	for len(picked) < n && len(remaining) > 0 {
		total := 0.0
		for _, w := range weights {
			total += w
		}
		r := rand.Float64() * total
		idx := len(weights) - 1
		cum := 0.0
		for i, w := range weights {
			cum += w
			if r <= cum {
				idx = i
				break
			}
		}
		picked = append(picked, remaining[idx])
		remaining = append(remaining[:idx], remaining[idx+1:]...)
		weights = append(weights[:idx], weights[idx+1:]...)
	}
	return picked
}

// GetHardestWordsForGame returns up to `count` "hardest words" game
// candidates: seen, graduated, >=3-attempt zh words ranked by lowest accuracy
// (mirrors FlagDifficultWords' accuracy formula), excluding any word shown in
// this game mode more recently than its most recent quiz attempt. The
// qualifying re-eligibility event is any new attempt, right or wrong.
func (s *Store) GetHardestWordsForGame(ctx context.Context, userID int64, count int) ([]models.MatchGameWord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.word_id
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		LEFT JOIN word_game_shown g
		  ON g.user_id = ? AND g.word_id = p.word_id AND g.game_mode = 'hardest'
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.first_seen_at IS NOT NULL
		  AND p.learning_new_word = 0
		  AND p.total_attempts >= 3
		  AND (g.last_shown_in_game IS NULL OR p.last_attempt_at > g.last_shown_in_game)
		ORDER BY `+difficultCandidateAccuracy+` ASC, p.word_id ASC
		LIMIT ?`, userID, userID, count)
	if err != nil {
		return nil, fmt.Errorf("get hardest words for game: %w", err)
	}
	ids, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}
	return s.wordIDsToMatchGameWords(ctx, userID, ids)
}

// GetLastMistakesForGame returns up to `count` "last mistakes" game
// candidates: zh words with at least one recorded wrong answer, most-recent
// mistake first, excluding any word shown in this game mode more recently
// than its most recent wrong answer. Unlike hardest-words, the qualifying
// re-eligibility event is specifically a new wrong answer — a correct
// re-attempt alone does not resurface the word.
func (s *Store) GetLastMistakesForGame(ctx context.Context, userID int64, count int) ([]models.MatchGameWord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.word_id
		FROM sm2_progress p
		JOIN words w ON w.id = p.word_id
		LEFT JOIN word_game_shown g
		  ON g.user_id = ? AND g.word_id = p.word_id AND g.game_mode = 'last_mistakes'
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.last_wrong_at IS NOT NULL
		  AND (g.last_shown_in_game IS NULL OR p.last_wrong_at > g.last_shown_in_game)
		ORDER BY p.last_wrong_at DESC, p.word_id ASC
		LIMIT ?`, userID, userID, count)
	if err != nil {
		return nil, fmt.Errorf("get last mistakes for game: %w", err)
	}
	ids, err := scanInt64Rows(rows)
	if err != nil {
		return nil, err
	}
	return s.wordIDsToMatchGameWords(ctx, userID, ids)
}

// scanInt64Rows drains a single-column int64 *sql.Rows result, closing it
// before returning (per CLAUDE.md, so a follow-up query in the same call
// chain never deadlocks against SetMaxOpenConns(1)).
func scanInt64Rows(rows *sql.Rows) ([]int64, error) {
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return ids, nil
}

// MarkWordsShownInGame upserts the current time as "last shown" for each
// (userID, wordID, mode) pair, so GetHardestWordsForGame and
// GetLastMistakesForGame suppress a word until their respective qualifying
// event happens again. Mirrors MarkConfusionsShownInGame's idiom for the new,
// per-word rather than per-confusion-pair, game modes.
func (s *Store) MarkWordsShownInGame(ctx context.Context, userID int64, wordIDs []int64, mode string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	for _, id := range wordIDs {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO word_game_shown (user_id, word_id, game_mode, last_shown_in_game)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(user_id, word_id, game_mode) DO UPDATE SET last_shown_in_game = excluded.last_shown_in_game`,
			userID, id, mode, now,
		); err != nil {
			return fmt.Errorf("mark word shown in game: %w", err)
		}
	}
	return nil
}
