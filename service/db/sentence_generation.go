package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// SentenceVocab is the word material for LLM sentence generation: the focus
// words each generated sentence is built around and the allowed words a
// sentence may consist of. Allowed is the same known-word set that decides
// sentence fill-in-the-blank eligibility, so a sentence that Check accepts can
// show up in that mode at once.
type SentenceVocab struct {
	Focus   []string
	Allowed []string

	known    map[string]int64
	pinyin   map[string]string
	existing map[string]bool // all zh word texts of the user, end punctuation stripped
}

// knownWord is one row of knownWords.
type knownWord struct {
	ID     int64
	Text   string
	Pinyin string
}

// knownWords returns the user's zh words that have been reviewed at least once
// (total_attempts > 0), excluding sentence-tagged rows, restricted to the tag
// selection (see tagFilterSQL; nil means all).
func (s *Store) knownWords(ctx context.Context, userID int64, tags []string) ([]knownWord, error) {
	tagSQL, tagArgs := tagFilterSQL(tags)
	args := append([]any{userID}, tagArgs...)
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text, w.pinyin FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		WHERE w.user_id = ? AND w.language = 'zh' AND p.total_attempts > 0
		  AND NOT EXISTS (
		    SELECT 1 FROM word_tags wt JOIN tags tg ON tg.id = wt.tag_id
		    WHERE wt.word_id = w.id AND tg.name LIKE '`+sentenceTagLike+`
		  )`+tagSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("get known words: %w", err)
	}
	defer rows.Close()
	var out []knownWord
	for rows.Next() {
		var kw knownWord
		var pinyin sql.NullString
		if err := rows.Scan(&kw.ID, &kw.Text, &pinyin); err != nil {
			return nil, err
		}
		kw.Pinyin = pinyin.String
		out = append(out, kw)
	}
	return out, rows.Err()
}

// GetSentenceVocab returns the user's known zh words (reviewed at least once,
// sentences excluded) as Allowed, and the known words that match the tag
// selection (tag groups, see tagFilterSQL) as Focus, at most maxFocus. Focus
// words that occur least often in the user's existing sentences come first
// (ties in random order), so repeated runs spread the words evenly.
func (s *Store) GetSentenceVocab(ctx context.Context, userID int64, tags []string, maxFocus int) (*SentenceVocab, error) {
	all, err := s.knownWords(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	v := &SentenceVocab{known: map[string]int64{}, pinyin: map[string]string{}, existing: map[string]bool{}}
	for _, kw := range all {
		v.Allowed = append(v.Allowed, kw.Text)
		v.known[kw.Text] = kw.ID
		v.pinyin[kw.Text] = kw.Pinyin
	}
	focus, err := s.knownWords(ctx, userID, tags)
	if err != nil {
		return nil, err
	}
	for _, kw := range focus {
		v.Focus = append(v.Focus, kw.Text)
	}

	sentences, err := s.getSentenceCandidates(ctx, userID)
	if err != nil {
		return nil, err
	}
	uses := map[string]int{}
	for _, sent := range sentences {
		segs, _ := segmentSentence(stripSentencePunctuation(sent.Text), v.known)
		for _, seg := range segs {
			uses[seg.Text]++
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT text FROM words WHERE user_id = ? AND language = 'zh'`, userID)
	if err != nil {
		return nil, fmt.Errorf("get zh word texts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		v.existing[stripSentencePunctuation(text)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rand.Shuffle(len(v.Focus), func(i, j int) { v.Focus[i], v.Focus[j] = v.Focus[j], v.Focus[i] })
	sort.SliceStable(v.Focus, func(i, j int) bool { return uses[v.Focus[i]] < uses[v.Focus[j]] })
	if len(v.Focus) > maxFocus {
		v.Focus = v.Focus[:maxFocus]
	}
	return v, nil
}

// pinyinPunctuation maps full-width punctuation to the form used in pinyin.
var pinyinPunctuation = strings.NewReplacer("，", ",", "。", ".", "！", "!", "？", "?", "；", ";", "：", ":", "、", ",", "　", " ")

// Check reports whether text consists of at least two allowed words
// (punctuation aside) and is not a word or sentence the user already has —
// adding such a text would tag the existing word as a sentence. For an
// accepted sentence it returns the pinyin built from the words' own pinyin,
// or "" when one of the words has no pinyin.
func (v *SentenceVocab) Check(text string) (pinyin string, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" || v.existing[stripSentencePunctuation(text)] {
		return "", false
	}
	segs, covered := segmentSentence(text, v.known)
	if !covered || len(segs) < 2 {
		return "", false
	}
	var sb strings.Builder
	runes := []rune(text)
	next := 0
	for i := 0; i < len(runes); {
		if isSentenceInternalPunctuation(runes[i]) {
			sb.WriteString(pinyinPunctuation.Replace(string(runes[i])))
			i++
			continue
		}
		seg := segs[next]
		next++
		p := v.pinyin[seg.Text]
		if p == "" {
			return "", true
		}
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(p)
		i += len([]rune(seg.Text))
	}
	return strings.Join(strings.Fields(sb.String()), " "), true
}
