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
	// Pairs are the mix-up pairs of known words with at least one word in
	// Focus, most mixed up first (only for SentenceFocusMixUps).
	Pairs [][2]string

	known    map[string]int64
	pinyin   map[string]string
	existing map[string]bool // all zh word texts of the user, end punctuation stripped
}

// Focus options for GetSentenceVocab: which known words the sentences are
// built around.
const (
	SentenceFocusAll      = "all"      // random order
	SentenceFocusHardest  = "hardest"  // lowest accuracy, then lowest easiness first
	SentenceFocusMixUps   = "mixups"   // words in mix-up pairs, most mix-ups first
	SentenceFocusFrequent = "frequent" // most common in Chinese (word_frequency_lang) first
)

// knownWord is one row of knownWords.
type knownWord struct {
	ID       int64
	Text     string
	Pinyin   string
	Correct  int
	Attempts int
	Easiness float64
	FreqRank int // 0: not in the zh frequency list
	MixUps   int // set by GetSentenceVocab for SentenceFocusMixUps
}

// knownWords returns the user's zh words that have been reviewed at least once
// (total_attempts > 0), excluding sentence-tagged rows, restricted to the tag
// selection (see tagFilterSQL; nil means all).
func (s *Store) knownWords(ctx context.Context, userID int64, tags []string) ([]knownWord, error) {
	tagSQL, tagArgs := tagFilterSQL(tags)
	args := append([]any{userID}, tagArgs...)
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.text, w.pinyin, p.total_correct + p.streak_bonus, p.total_attempts, p.easiness, COALESCE(f.rank, 0) FROM words w
		JOIN sm2_progress p ON p.word_id = w.id
		LEFT JOIN word_frequency_lang f ON f.word = w.text AND f.lang = 'zh'
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
		if err := rows.Scan(&kw.ID, &kw.Text, &pinyin, &kw.Correct, &kw.Attempts, &kw.Easiness, &kw.FreqRank); err != nil {
			return nil, err
		}
		kw.Pinyin = pinyin.String
		out = append(out, kw)
	}
	return out, rows.Err()
}

// GetSentenceVocab returns the user's known zh words (reviewed at least once,
// sentences excluded) as Allowed, and the known words that match the tag
// selection (tag groups, see tagFilterSQL) as Focus, at most maxFocus, in the
// order of the focus option (SentenceFocus*; ties in random order). With even,
// words that occur least often in the user's existing sentences come first and
// the focus option only breaks ties, so repeated runs spread the words evenly.
func (s *Store) GetSentenceVocab(ctx context.Context, userID int64, tags []string, focus string, even bool, maxFocus int) (*SentenceVocab, error) {
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
	candidates, err := s.knownWords(ctx, userID, tags)
	if err != nil {
		return nil, err
	}
	var pairs []mixUpPair
	if focus == SentenceFocusMixUps {
		if pairs, err = s.mixUpPairs(ctx, userID, v.known); err != nil {
			return nil, err
		}
		mixUps := map[string]int{}
		for _, p := range pairs {
			mixUps[p.A] += p.Count
			mixUps[p.B] += p.Count
		}
		var mixed []knownWord
		for _, kw := range candidates {
			if kw.MixUps = mixUps[kw.Text]; kw.MixUps > 0 {
				mixed = append(mixed, kw)
			}
		}
		candidates = mixed
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

	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if even && uses[a.Text] != uses[b.Text] {
			return uses[a.Text] < uses[b.Text]
		}
		return focusLess(focus, a, b)
	})
	if len(candidates) > maxFocus {
		candidates = candidates[:maxFocus]
	}
	inFocus := map[string]bool{}
	for _, kw := range candidates {
		v.Focus = append(v.Focus, kw.Text)
		inFocus[kw.Text] = true
	}
	for _, p := range pairs {
		if inFocus[p.A] || inFocus[p.B] {
			v.Pairs = append(v.Pairs, [2]string{p.A, p.B})
		}
	}
	return v, nil
}

// mixUpPair is a word-vs-word mix-up of two known words, counted over both
// directions and all modes.
type mixUpPair struct {
	A, B  string
	Count int
}

// mixUpPairs returns the user's word-vs-word mix-ups (component mix-ups
// aside) where both words are in known, most mixed up first.
func (s *Store) mixUpPairs(ctx context.Context, userID int64, known map[string]int64) ([]mixUpPair, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wa.text, wb.text, cp.count FROM confusion_pairs cp
		JOIN words wa ON wa.id = cp.zh_word_id
		JOIN words wb ON wb.id = cp.confused_with_id
		WHERE cp.user_id = ? AND cp.zh_component = '' AND cp.confused_with_component = ''`, userID)
	if err != nil {
		return nil, fmt.Errorf("get mix-up pairs: %w", err)
	}
	defer rows.Close()
	var out []mixUpPair
	index := map[[2]string]int{}
	for rows.Next() {
		var a, b string
		var n int
		if err := rows.Scan(&a, &b, &n); err != nil {
			return nil, err
		}
		if _, ok := known[a]; !ok {
			continue
		}
		if _, ok := known[b]; !ok || a == b {
			continue
		}
		key := [2]string{min(a, b), max(a, b)}
		if i, ok := index[key]; ok {
			out[i].Count += n
			continue
		}
		index[key] = len(out)
		out = append(out, mixUpPair{A: a, B: b, Count: n})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, nil
}

// focusLess reports whether a comes before b under the focus option.
func focusLess(focus string, a, b knownWord) bool {
	switch focus {
	case SentenceFocusHardest:
		// Same accuracy formula and minimum attempts as FlagDifficultWords.
		aFew, bFew := a.Attempts < 3, b.Attempts < 3
		if aFew != bFew {
			return bFew
		}
		accA := float64(a.Correct) / float64(max(a.Attempts, 1))
		accB := float64(b.Correct) / float64(max(b.Attempts, 1))
		if accA != accB {
			return accA < accB
		}
		return a.Easiness < b.Easiness
	case SentenceFocusFrequent:
		if (a.FreqRank == 0) != (b.FreqRank == 0) {
			return b.FreqRank == 0
		}
		return a.FreqRank < b.FreqRank
	case SentenceFocusMixUps:
		return a.MixUps > b.MixUps
	}
	return false
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
