package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vocabulary_trainer/models"
	"vocabulary_trainer/sm2"
)

// sentenceBlankToken is the placeholder inserted where the target word or
// translation substring was removed.
const sentenceBlankToken = "___"

// sentenceTagLike is the SQL LIKE pattern (with its ESCAPE clause) that
// identifies a "sentence" tag by the `s_` prefix convention (e.g. s_hsk1).
const sentenceTagLike = `s\_%' ESCAPE '\'`

// wordSegment is one token produced by segmentSentence: the matched text and
// the zh word_id it corresponds to in the user's vocabulary.
type wordSegment struct {
	Text   string
	WordID int64
}

// sentenceBlankMatch is the result of findSentenceBlank: which due word was
// found inside which eligible sentence, and the exact substring to blank.
type sentenceBlankMatch struct {
	SentenceID   int64
	SentenceText string
	TargetWordID int64
	TargetText   string
}

// sentencePunctuationCutset is trailing sentence-ending punctuation stripped
// before segmentation, mirroring the punctuation-stripping rule in
// sm2.NormalizeAnswer (zh full-width and ASCII forms).
const sentencePunctuationCutset = "。.！!？? \t\n"

func stripSentencePunctuation(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), sentencePunctuationCutset)
}

// sentenceInternalPunctuation is punctuation segmentSentence skips over
// (mid-sentence, not just the trailing terminator stripSentencePunctuation
// handles) rather than treating as an unmatched, coverage-breaking
// character. Real sentences are usually multiple clauses joined by a comma,
// enumeration mark, colon, semicolon, or quotation marks — without this, a
// sentence containing any of them could never reach 100% word coverage no
// matter how complete the known-word set was (issue #351).
const sentenceInternalPunctuation = "，,。.！!？?；;：:、…—-—–　 \t\n\"'“”‘’（）()《》〈〉"

func isSentenceInternalPunctuation(r rune) bool {
	return strings.ContainsRune(sentenceInternalPunctuation, r)
}

// segmentSentence greedily tokenizes text against known (a map of zh word
// text -> word_id, e.g. the user's own reviewed vocabulary) using
// longest-match-first at each position. Punctuation (see
// sentenceInternalPunctuation) is skipped rather than matched. fullyCovered
// is false if any non-punctuation character of text could not be matched to
// a known word.
func segmentSentence(text string, known map[string]int64) (segments []wordSegment, fullyCovered bool) {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, true
	}
	maxLen := 0
	for k := range known {
		if l := len([]rune(k)); l > maxLen {
			maxLen = l
		}
	}
	if maxLen == 0 {
		return nil, false
	}
	i := 0
	for i < len(runes) {
		if isSentenceInternalPunctuation(runes[i]) {
			i++
			continue
		}
		limit := maxLen
		if i+limit > len(runes) {
			limit = len(runes) - i
		}
		matched := false
		for l := limit; l >= 1; l-- {
			cand := string(runes[i : i+l])
			if id, ok := known[cand]; ok {
				segments = append(segments, wordSegment{Text: cand, WordID: id})
				i += l
				matched = true
				break
			}
		}
		if !matched {
			return segments, false
		}
	}
	return segments, true
}

// getSentenceCandidates returns the user's zh words tagged with an `s_`-prefixed
// tag (the sentence-marker convention — see CLAUDE.md).
func (s *Store) getSentenceCandidates(ctx context.Context, userID int64) ([]models.Word, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT w.id, w.text, w.language, w.pinyin, w.created_at
		FROM words w
		JOIN word_tags wt ON wt.word_id = w.id
		JOIN tags tg ON tg.id = wt.tag_id
		WHERE w.user_id = ? AND w.language = 'zh' AND tg.name LIKE '`+sentenceTagLike+`
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get sentence candidates: %w", err)
	}
	defer rows.Close()
	var out []models.Word
	for rows.Next() {
		var w models.Word
		var createdAt string
		if err := rows.Scan(&w.ID, &w.Text, &w.Language, &w.Pinyin, &createdAt); err != nil {
			return nil, err
		}
		w.CreatedAt = parseDateTime(createdAt)
		out = append(out, w)
	}
	return out, rows.Err()
}

// getKnownWordSet returns the user's zh words that have been reviewed at
// least once (total_attempts > 0), excluding sentence-tagged rows — this is
// the segmentation dictionary used to decide sentence eligibility.
func (s *Store) getKnownWordSet(ctx context.Context, userID int64) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.user_id = ? AND w.language = 'zh' AND p.total_attempts > 0
		  AND NOT EXISTS (
		    SELECT 1 FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id
		    WHERE wt.word_id = w.id AND tg.name LIKE '`+sentenceTagLike+`
		  )
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get known word set: %w", err)
	}
	defer rows.Close()
	known := map[string]int64{}
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		known[text] = id
	}
	return known, rows.Err()
}

// dueZhWordSet returns a map of word_id → due_date for due non-sentence zh
// words belonging to the user, restricted to the given tags (same filter as
// GetNextCard). No LIMIT — the full tag-filtered due set is needed so that
// sentence-compatible words are not silently excluded by an arbitrary cutoff.
func (s *Store) dueZhWordSet(ctx context.Context, userID int64, tags []string) (map[int64]time.Time, error) {
	tagFilter := ""
	var args []any
	args = append(args, userID)
	if len(tags) > 0 {
		placeholders := make([]string, len(tags))
		for i, t := range tags {
			placeholders[i] = "?"
			args = append(args, t)
		}
		tagFilter = ` AND EXISTS (
			SELECT 1 FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id
			WHERE wt.word_id = w.id AND tg.name IN (` + strings.Join(placeholders, ",") + `))`
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, p.due_date FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.language = 'zh' AND w.user_id = ?
		  AND p.due_date <= CURRENT_TIMESTAMP`+tagFilter+`
		  AND NOT EXISTS (
		    SELECT 1 FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id
		    WHERE wt.word_id = w.id AND tg.name LIKE '`+sentenceTagLike+`
		  )
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("get due zh words for sentence blank: %w", err)
	}
	defer rows.Close()
	due := map[int64]time.Time{}
	for rows.Next() {
		var id int64
		var dueDateStr string
		if err := rows.Scan(&id, &dueDateStr); err != nil {
			return nil, err
		}
		due[id] = parseDateTime(dueDateStr)
	}
	return due, rows.Err()
}

// findSentenceBlank walks eligible sentences and returns the match whose due
// word has the earliest due_date. Returns (nil, nil) when nothing matches.
func (s *Store) findSentenceBlank(ctx context.Context, userID int64, tags []string) (*sentenceBlankMatch, error) {
	due, err := s.dueZhWordSet(ctx, userID, tags)
	if err != nil || len(due) == 0 {
		return nil, err
	}
	known, err := s.getKnownWordSet(ctx, userID)
	if err != nil || len(known) == 0 {
		return nil, err
	}
	candidates, err := s.getSentenceCandidates(ctx, userID)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}

	var best *sentenceBlankMatch
	var bestDue time.Time
	for _, c := range candidates {
		segs, ok := segmentSentence(stripSentencePunctuation(c.Text), known)
		if !ok || len(segs) == 0 {
			continue
		}
		for _, seg := range segs {
			dueDate, isDue := due[seg.WordID]
			if !isDue {
				continue
			}
			if best == nil || dueDate.Before(bestDue) {
				best = &sentenceBlankMatch{
					SentenceID:   c.ID,
					SentenceText: c.Text,
					TargetWordID: seg.WordID,
					TargetText:   seg.Text,
				}
				bestDue = dueDate
			}
		}
	}
	return best, nil
}

// locateTranslationBlank looks for any of wordTranslations as a
// case-insensitive substring of sentenceTranslation and, if found, returns
// sentenceTranslation with that substring replaced by the blank token.
func locateTranslationBlank(sentenceTranslation string, wordTranslations []string) (blanked string, found bool) {
	lower := strings.ToLower(sentenceTranslation)
	for _, wt := range wordTranslations {
		wt = strings.TrimSpace(wt)
		if wt == "" {
			continue
		}
		idx := strings.Index(lower, strings.ToLower(wt))
		if idx == -1 {
			continue
		}
		return sentenceTranslation[:idx] + sentenceBlankToken + sentenceTranslation[idx+len(wt):], true
	}
	return "", false
}

// NextSentenceBlankCard finds the next due word reachable through an
// eligible sentence and builds the fill-in-the-blank card for it, resolving
// direction via the same progressive-mode logic normal word cards use.
// Returns (nil, nil) when no eligible sentence/due-word pair exists.
func (s *Store) NextSentenceBlankCard(ctx context.Context, userID int64, cfg models.ProgressiveModeConfig, nwCfg models.NewWordModeConfig, langs []string, tags []string) (*models.QuizCard, error) {
	match, err := s.findSentenceBlank(ctx, userID, tags)
	if err != nil || match == nil {
		return nil, err
	}
	progress, err := s.GetSM2Progress(ctx, match.TargetWordID)
	if err != nil {
		return nil, err
	}
	if progress == nil {
		return nil, nil
	}

	var mode string
	if progress.LearningNewWord {
		mode = sm2.SelectNewWordMode(progress.TotalCorrect, nwCfg)
	} else {
		mode = sm2.SelectProgressiveMode(progress.TotalCorrect, progress.TotalAttempts, progress.StreakBonus, cfg)
	}

	var sentenceTransTexts []string
	for _, lang := range langs {
		texts, err := s.getTranslationTextsForZhWord(ctx, match.SentenceID, lang)
		if err != nil {
			return nil, err
		}
		sentenceTransTexts = append(sentenceTransTexts, texts...)
	}

	zhBlankCard := func() *models.QuizCard {
		return &models.QuizCard{
			CardType:        "sentence",
			WordID:          match.TargetWordID,
			Mode:            models.ModeTranslToZh,
			SentenceContext: strings.Join(sentenceTransTexts, " · "),
			SentenceBlank:   strings.Replace(match.SentenceText, match.TargetText, sentenceBlankToken, 1),
			DueDate:         progress.DueDate,
			IntervalDays:    progress.IntervalDays,
		}
	}

	if mode == models.ModeTranslToZh {
		return zhBlankCard(), nil
	}

	// Translation-blank direction: locate the blanked word's own translation
	// as a substring of one of the sentence's translations.
	var wordTransTexts []string
	for _, lang := range langs {
		texts, err := s.getTranslationTextsForZhWord(ctx, match.TargetWordID, lang)
		if err != nil {
			return nil, err
		}
		wordTransTexts = append(wordTransTexts, texts...)
	}
	for _, sentenceTrans := range sentenceTransTexts {
		if blanked, ok := locateTranslationBlank(sentenceTrans, wordTransTexts); ok {
			return &models.QuizCard{
				CardType:        "sentence",
				WordID:          match.TargetWordID,
				Mode:            mode,
				SentenceContext: match.SentenceText,
				SentenceBlank:   blanked,
				DueDate:         progress.DueDate,
				IntervalDays:    progress.IntervalDays,
			}, nil
		}
	}

	// No substring match for direction (b) — fall back to direction (a),
	// which never fails since the target text was derived from the sentence
	// text itself.
	return zhBlankCard(), nil
}
