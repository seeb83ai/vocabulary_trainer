package sm2

import (
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
	"vocabulary_trainer/models"
)

// reParens matches a parenthesized segment (no nested parens) — ASCII () or fullwidth （）—
// and any surrounding whitespace. Applied iteratively so that nested parens are stripped inside-out.
var reParens = regexp.MustCompile(`\s*[(（][^()（）]*[)）]\s*`)

// reTrailingPunct matches any trailing punctuation (Unicode \p{P} and \p{S}) and whitespace.
var reTrailingPunct = regexp.MustCompile(`[\p{P}\p{S}\s]+$`)

// reDotsRun matches a run of one or more halfwidth periods and/or ideographic
// ellipsis characters (U+2026), in any combination, anywhere in the string —
// so "……", "。。。" (after fullwidth conversion), and "..." are all treated as
// the same pause punctuation regardless of position.
var reDotsRun = regexp.MustCompile(`[.…]+`)

// fullwidthToHalfwidth converts common fullwidth punctuation characters to their
// ASCII halfwidth equivalents so that, e.g., ？ and ? are interchangeable in answers.
var fullwidthToHalfwidth = strings.NewReplacer(
	"？", "?",
	"！", "!",
	"，", ",",
	"。", ".",
	"：", ":",
	"；", ";",
)

// apostropheVariants converts curly/typographic apostrophe variants (’ ‘ ´ `)
// to the ASCII straight apostrophe ' so "Don't" and "Don't" compare equal
// regardless of which one was typed or stored.
var apostropheVariants = strings.NewReplacer(
	"’", "'",
	"‘", "'",
	"´", "'",
	"`", "'",
)

const (
	QualityCorrect       = 4
	QualityWrong         = 0
	WrongRetryDelay      = 3 * time.Minute
	LearningCorrectDelay = 2 * time.Minute
	LearningGraduateReps = 3
)

// Update applies the SM-2 algorithm and returns an updated SM2Progress.
func Update(p models.SM2Progress, quality int) models.SM2Progress {
	// Update easiness factor
	ef := p.Easiness + (0.1 - float64(5-quality)*(0.08+float64(5-quality)*0.02))
	if ef < 1.3 {
		ef = 1.3
	}

	var repetitions int
	var intervalDays int

	if quality < 3 {
		repetitions = 0
		intervalDays = 0
		p.Easiness = ef
		p.Repetitions = repetitions
		p.IntervalDays = intervalDays
		p.DueDate = time.Now().UTC().Add(WrongRetryDelay + time.Duration(rand.Int63n(int64(WrongRetryDelay*2))))
		return p
	} else {
		switch p.Repetitions {
		case 0:
			intervalDays = 1
		case 1:
			intervalDays = 6
		default:
			intervalDays = int(math.Round(float64(p.IntervalDays) * ef))
		}
		repetitions = p.Repetitions + 1
	}

	p.Easiness = ef
	p.Repetitions = repetitions
	p.IntervalDays = intervalDays
	jitter := time.Duration(rand.Int63n(int64(2*time.Hour))) - 2*time.Hour
	p.DueDate = time.Now().UTC().Add(time.Duration(intervalDays)*24*time.Hour + jitter)
	return p
}

// UpdateLearning applies a simplified update for words still in the learning phase.
// Uses short intervals (minutes) so all 3 correct answers can happen in one session.
// Returns the updated progress and whether the word has graduated (repetitions >= 3).
func UpdateLearning(p models.SM2Progress, quality int) (models.SM2Progress, bool) {
	if quality < 3 {
		// Wrong answer: reset streak
		p.Repetitions = 0
		p.DueDate = time.Now().UTC().Add(WrongRetryDelay + time.Duration(rand.Int63n(int64(WrongRetryDelay*2))))
		return p, false
	}

	p.Repetitions++
	jitter := time.Duration(rand.Int63n(int64(LearningCorrectDelay)))
	p.DueDate = time.Now().UTC().Add(LearningCorrectDelay + jitter)

	if p.Repetitions >= LearningGraduateReps {
		// Graduate: reset SM-2 state for a clean start
		p.LearningNewWord = false
		p.Repetitions = 0
		p.Easiness = 2.5
		p.IntervalDays = 1
		p.TotalCorrect = 3
		p.TotalAttempts = 3
		p.StreakBonus = 0
		p.DueDate = time.Now().UTC().Add(24*time.Hour + time.Duration(rand.Int63n(int64(2*time.Hour))))
		return p, true
	}

	return p, false
}

// CalcStreakBonus computes the streak_bonus so that effective accuracy
// (total_correct + streak_bonus) / total_attempts reaches the minimum for
// the bucket corresponding to the current streak length (repetitions).
// The bonus never decreases below currentBonus.
func CalcStreakBonus(currentBonus, repetitions, totalCorrect, totalAttempts int) int {
	if totalAttempts == 0 {
		return currentBonus
	}
	var targetAcc float64
	switch {
	case repetitions >= 9:
		targetAcc = 0.85
	case repetitions >= 6:
		targetAcc = 0.70
	case repetitions >= 3:
		targetAcc = 0.50
	default:
		return currentBonus
	}
	needed := int(math.Ceil(targetAcc*float64(totalAttempts))) - totalCorrect
	if needed < 0 {
		needed = 0
	}
	if needed > currentBonus {
		return needed
	}
	return currentBonus
}

// ProcessAnswer applies a graded answer to an SM-2 progress record and returns
// the updated record. It encapsulates the full quiz-answer state transition:
//   - learning-phase words go through UpdateLearning (graduating after enough
//     correct reps); regular words go through Update;
//   - TotalAttempts/TotalCorrect are incremented for the attempt, except on
//     graduation (UpdateLearning already resets those counters);
//   - CalcStreakBonus is always applied last.
//
// Graduation can be detected by the caller as: was LearningNewWord before and
// is no longer afterwards.
func ProcessAnswer(p models.SM2Progress, correct bool) models.SM2Progress {
	quality := QualityWrong
	if correct {
		quality = QualityCorrect
	}

	var updated models.SM2Progress
	if p.LearningNewWord {
		var graduated bool
		updated, graduated = UpdateLearning(p, quality)
		if !graduated {
			updated.TotalAttempts++
			if correct {
				updated.TotalCorrect++
			}
		}
	} else {
		updated = Update(p, quality)
		updated.TotalAttempts++
		if correct {
			updated.TotalCorrect++
		}
	}
	updated.StreakBonus = CalcStreakBonus(updated.StreakBonus, updated.Repetitions, updated.TotalCorrect, updated.TotalAttempts)
	return updated
}

// CheckAnswer returns true if the user's answer matches any accepted answer
// (case-insensitive, whitespace-trimmed).
//
// Two normalisation rules apply to each accepted answer before comparing:
//  1. Parenthesized segments are optional: "(das Gehörte) nicht verstehen"
//     also accepts "nicht verstehen".
//  2. Slash- or comma-separated alternatives are each valid on their own:
//     "Essen / Gericht" also accepts "Essen" or "Gericht", and
//     "topic, item" also accepts "topic" or "item".
//
// All combinations of the two rules are tried.
func CheckAnswer(userAnswer string, accepted []string) bool {
	ua := normalize(userAnswer)
	for _, a := range accepted {
		for _, variant := range expandVariants(a) {
			if variant == ua {
				return true
			}
		}
	}
	return false
}

// NormalizeAnswer lowercases, collapses internal whitespace, converts common
// fullwidth punctuation to their ASCII equivalents, converts curly/typographic
// apostrophes to the ASCII straight apostrophe, collapses any run of periods
// and/or ideographic ellipsis characters (anywhere in the string) into a
// single space, and strips all trailing punctuation and whitespace.
func NormalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = fullwidthToHalfwidth.Replace(s)
	s = apostropheVariants.Replace(s)
	s = reDotsRun.ReplaceAllString(s, " ")
	s = reTrailingPunct.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// normalize is the package-internal alias used by expandVariants/CheckAnswer.
func normalize(s string) string { return NormalizeAnswer(s) }

// stripParens removes all parenthesised segments from s, iterating until stable
// to handle nested parens (inner-most first).
func stripParens(s string) string {
	for {
		next := strings.TrimSpace(reParens.ReplaceAllString(s, " "))
		if next == strings.TrimSpace(s) {
			break
		}
		s = next
	}
	return s
}

// ExpandVariants returns all valid answer strings derived from a single
// accepted answer by applying the optional-parens and slash/comma-split rules.
func ExpandVariants(a string) []string { return expandVariants(a) }

func expandVariants(a string) []string {
	seen := map[string]struct{}{}
	add := func(s string) {
		s = normalize(s)
		if s != "" {
			seen[s] = struct{}{}
		}
	}

	// Full form (with parens, with slashes)
	add(a)

	noParens := stripParens(a)
	add(noParens)

	// Slash- or comma-split variants of both the original and the paren-stripped form
	for _, base := range []string{a, noParens} {
		for _, part := range strings.FieldsFunc(base, func(r rune) bool { return r == '/' || r == ',' }) {
			add(part)
			add(stripParens(part))
		}
	}

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// CheckComponentAnswer checks whether the user's answer matches any part of a
// hanzi component definition. The definition is split on ';' and ',' — each
// resulting part is a valid answer after normalisation. Parenthesised segments
// within a part are optional (the answer is accepted with or without them).
func CheckComponentAnswer(userAnswer, definition string) bool {
	ua := normalize(userAnswer)
	for _, part := range strings.FieldsFunc(definition, func(r rune) bool { return r == ';' || r == ',' }) {
		if normalize(part) == ua {
			return true
		}
		if normalize(stripParens(part)) == ua {
			return true
		}
	}
	return false
}

// CheckHMMAnswer checks whether userAnswer matches correctName after stripping
// optional parenthesised segments from both sides (case-insensitive, no
// trailing-punctuation stripping — matching current HMM quiz behaviour).
func CheckHMMAnswer(userAnswer, correctName string) bool {
	norm := func(s string) string { return strings.Join(strings.Fields(stripParens(s)), " ") }
	return strings.EqualFold(norm(userAnswer), norm(correctName))
}

// MaskPinyin returns a masked pinyin hint for learning-phase transl_to_zh cards.
// The masking level depends on totalCorrect:
//
//	0 → first char of each syllable visible, rest replaced with * per char ("nǐ hǎo" → "n** h**")
//	1 → first char of full pinyin visible + * per remaining char            ("nǐ hǎo" → "n*****")
//	2+ → empty string (no hint)
func MaskPinyin(pinyin string, totalCorrect int) string {
	if pinyin == "" || totalCorrect >= 2 {
		return ""
	}
	runes := []rune(pinyin)
	if totalCorrect == 1 {
		var b strings.Builder
		b.WriteRune(runes[0])
		for _, r := range runes[1:] {
			if r == ' ' {
				b.WriteRune(' ')
			} else {
				b.WriteRune('*')
			}
		}
		return b.String()
	}
	// totalCorrect == 0: mask each space-separated syllable
	words := strings.Split(pinyin, " ")
	for i, w := range words {
		wr := []rune(w)
		if len(wr) == 0 {
			continue
		}
		var b strings.Builder
		b.WriteRune(wr[0])
		for range wr[1:] {
			b.WriteRune('*')
		}
		words[i] = b.String()
	}
	return strings.Join(words, " ")
}

// ProgressiveModeConfig is defined in the models package so UserSettings can
// project to it (UserSettings.QuizConfig) without an import cycle; this alias
// keeps sm2.ProgressiveModeConfig working for existing callers.
type ProgressiveModeConfig = models.ProgressiveModeConfig

// DefaultProgressiveModeConfig returns the built-in defaults.
func DefaultProgressiveModeConfig() ProgressiveModeConfig {
	return ProgressiveModeConfig{
		New:        models.ModeTranslToZh,
		Struggling: models.ModeTranslToZh,
		Learning:   models.ModeZhPinyinToTransl,
		Practicing: models.ModeZhToTransl,
		Mastered:   "random",
	}
}

// NewWordModeConfig is defined in the models package (see ProgressiveModeConfig);
// this alias keeps sm2.NewWordModeConfig working for existing callers.
type NewWordModeConfig = models.NewWordModeConfig

// DefaultNewWordModeConfig returns the built-in defaults.
func DefaultNewWordModeConfig() NewWordModeConfig {
	return NewWordModeConfig{
		Step0: models.ModeTranslToZh,
		Step1: models.ModeTranslToZh,
		Step2: models.ModeZhToTransl,
	}
}

func resolveMode(configured, fallback string) string {
	if configured == "" {
		return fallback
	}
	if configured == "random" {
		return selectModeUniform()
	}
	return configured
}

// SelectProgressiveMode picks a quiz mode based on the word's accuracy and the
// user's per-tier configuration. The progressive training ladder:
//   - totalAttempts < 3                       → cfg.New
//   - accuracy < 50%                          → cfg.Struggling
//   - accuracy < 70% or totalAttempts < 10    → cfg.Learning
//   - accuracy < 85%                          → cfg.Practicing
//   - accuracy ≥ 85% and totalAttempts ≥ 10   → cfg.Mastered
func SelectProgressiveMode(totalCorrect, totalAttempts, streakBonus int, cfg ProgressiveModeConfig) string {
	if totalAttempts < 3 {
		return resolveMode(cfg.New, models.ModeTranslToZh)
	}
	accuracy := float64(totalCorrect+streakBonus) / float64(totalAttempts)
	switch {
	case accuracy < 0.50:
		return resolveMode(cfg.Struggling, models.ModeTranslToZh)
	case accuracy < 0.70 || totalAttempts < 10:
		return resolveMode(cfg.Learning, models.ModeZhPinyinToTransl)
	case accuracy < 0.85:
		return resolveMode(cfg.Practicing, models.ModeZhToTransl)
	default:
		return resolveMode(cfg.Mastered, "random")
	}
}

// SelectNewWordMode picks the quiz mode for a LearningNewWord word (after its
// initial introduction) based on how many times the user has answered correctly.
func SelectNewWordMode(totalCorrect int, cfg NewWordModeConfig) string {
	switch {
	case totalCorrect <= 0:
		return resolveMode(cfg.Step0, models.ModeTranslToZh)
	case totalCorrect == 1:
		return resolveMode(cfg.Step1, models.ModeTranslToZh)
	default:
		return resolveMode(cfg.Step2, models.ModeZhToTransl)
	}
}

// allRandomModes is the fixed candidate pool that SelectMode/SelectCycleMode
// pick from and that RandomModeConfig governs per learning bucket.
var allRandomModes = []string{
	models.ModeTranslToZh,
	models.ModeZhToTransl,
	models.ModeZhPinyinToTransl,
	models.ModeZhToTranslNoSound,
	models.ModeVoiceToTransl,
}

// controlledRandomModes is the set of modes RandomModeConfig governs. A cycle
// step outside this set (e.g. mask_pinyin) is never restricted by bucket.
var controlledRandomModes = map[string]bool{
	models.ModeTranslToZh:        true,
	models.ModeZhToTransl:        true,
	models.ModeZhPinyinToTransl:  true,
	models.ModeZhToTranslNoSound: true,
	models.ModeVoiceToTransl:     true,
}

// selectModeUniform randomly picks one of the 5 quiz modes with equal
// probability, ignoring any bucket restriction. Used internally by
// resolveMode's "random" fallback for the progressive/new-word ladders,
// which are intentionally unaffected by RandomModeConfig.
func selectModeUniform() string {
	return allRandomModes[rand.Intn(len(allRandomModes))]
}

// RandomModeConfig is defined in the models package (see ProgressiveModeConfig);
// this alias keeps sm2.RandomModeConfig working for existing callers.
type RandomModeConfig = models.RandomModeConfig

// DefaultRandomModeConfig returns the built-in default per-mode bucket ranges
// used when a mode's RandomModeConfig field is unset (""). Ranges increase in
// difficulty (fewer hints) for higher buckets; every bucket has at least one
// eligible mode (see TestDefaultRandomModeConfig_EveryBucketHasEligibleMode).
func DefaultRandomModeConfig() RandomModeConfig {
	return RandomModeConfig{
		TranslToZh:        "new,50-69",
		ZhPinyinToTransl:  "new,70-84",
		ZhToTransl:        "0-49,85-100",
		ZhToTranslNoSound: "50-69,85-100",
		VoiceToTransl:     "70-84,85-100",
	}
}

// bucketOrder lists the tier bucket keys used by RandomModeConfig ranges, in
// increasing-difficulty order. Mirrors tierFilter (db/words.go) and TIERS
// (frontend/app.js).
var bucketOrder = []string{"new", "0-49", "50-69", "70-84", "85-100"}

func bucketIndex(b string) int {
	for i, x := range bucketOrder {
		if x == b {
			return i
		}
	}
	return -1
}

// parseModeRange parses a "<from>,<to>" RandomModeConfig field value into
// bucketOrder indices. ok is false for malformed values, unknown bucket keys,
// or a from-index greater than the to-index.
func parseModeRange(v string) (from, to int, ok bool) {
	parts := strings.SplitN(v, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	from, to = bucketIndex(strings.TrimSpace(parts[0])), bucketIndex(strings.TrimSpace(parts[1]))
	if from == -1 || to == -1 || from > to {
		return 0, 0, false
	}
	return from, to, true
}

// ValidModeRange reports whether v is a valid RandomModeConfig field value:
// "" (default), "off", or "<from>,<to>" using valid bucket keys with from<=to.
func ValidModeRange(v string) bool {
	if v == "" || v == "off" {
		return true
	}
	_, _, ok := parseModeRange(v)
	return ok
}

// resolveEligibleModes returns the controlled quiz modes eligible for bucket,
// resolved from cfg's per-mode ranges (falling back to DefaultRandomModeConfig
// for unset "" fields). "off" disables a mode in every bucket. An unknown
// bucket or malformed range value yields no eligibility for that mode. The
// returned order matches allRandomModes.
func resolveEligibleModes(bucket string, cfg RandomModeConfig) []string {
	def := DefaultRandomModeConfig()
	bi := bucketIndex(bucket)
	if bi == -1 {
		return nil
	}
	ranges := []struct{ mode, value, fallback string }{
		{models.ModeTranslToZh, cfg.TranslToZh, def.TranslToZh},
		{models.ModeZhToTransl, cfg.ZhToTransl, def.ZhToTransl},
		{models.ModeZhPinyinToTransl, cfg.ZhPinyinToTransl, def.ZhPinyinToTransl},
		{models.ModeZhToTranslNoSound, cfg.ZhToTranslNoSound, def.ZhToTranslNoSound},
		{models.ModeVoiceToTransl, cfg.VoiceToTransl, def.VoiceToTransl},
	}
	var eligible []string
	for _, r := range ranges {
		v := r.value
		if v == "" {
			v = r.fallback
		}
		if v == "off" {
			continue
		}
		from, to, ok := parseModeRange(v)
		if !ok {
			continue
		}
		if bi >= from && bi <= to {
			eligible = append(eligible, r.mode)
		}
	}
	return eligible
}

// BucketsWithoutEligibleMode returns the bucket keys (in bucketOrder) that
// have zero eligible modes under cfg. Used to validate, at settings-save
// time, that every learning bucket has at least one eligible random/cycle
// mode before persisting.
func BucketsWithoutEligibleMode(cfg RandomModeConfig) []string {
	var uncovered []string
	for _, b := range bucketOrder {
		if len(resolveEligibleModes(b, cfg)) == 0 {
			uncovered = append(uncovered, b)
		}
	}
	return uncovered
}

// isEligibleForBucket reports whether mode may be picked for bucket under cfg.
// Modes RandomModeConfig does not govern (e.g. mask_pinyin as a cycle step)
// are always eligible.
func isEligibleForBucket(mode, bucket string, cfg RandomModeConfig) bool {
	if !controlledRandomModes[mode] {
		return true
	}
	for _, m := range resolveEligibleModes(bucket, cfg) {
		if m == mode {
			return true
		}
	}
	return false
}

// SelectMode randomly picks one of the quiz modes eligible for bucket under
// cfg, with equal probability among the eligible set. Falls back to the full
// 5-mode pool when bucket is empty/unrecognized or resolves to zero eligible
// modes (e.g. cfg turns every mode off for this bucket), so a mode is always
// returned.
func SelectMode(bucket string, cfg RandomModeConfig) string {
	modes := resolveEligibleModes(bucket, cfg)
	if len(modes) == 0 {
		modes = allRandomModes
	}
	return modes[rand.Intn(len(modes))]
}

// DefaultCycleSequence is the default cycle mode direction sequence.
const DefaultCycleSequence = "zh_pinyin_to_transl,transl_to_zh,zh_to_transl"

// ParseCycleSequence splits a comma-separated cycle sequence string into a
// slice of mode strings. Falls back to DefaultCycleSequence when seq is empty
// or contains only whitespace.
func ParseCycleSequence(seq string) []string {
	if seq == "" {
		seq = DefaultCycleSequence
	}
	var result []string
	for _, p := range strings.Split(seq, ",") {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return strings.Split(DefaultCycleSequence, ",")
	}
	return result
}

// SelectCycleMode returns the quiz mode for the current cycle position derived
// from totalAttempts. Position is (totalAttempts-1) so that a word with
// total_attempts=1 (just acknowledged) starts at position 0. The configured
// sequence is first filtered down to the modes eligible for bucket under cfg;
// if that intersection is empty (e.g. every configured step is disabled for
// this bucket), the full bucket-eligible mode list is used instead so a valid
// mode is always returned.
func SelectCycleMode(totalAttempts int, sequence []string, bucket string, cfg RandomModeConfig) string {
	filtered := make([]string, 0, len(sequence))
	for _, m := range sequence {
		if isEligibleForBucket(m, bucket, cfg) {
			filtered = append(filtered, m)
		}
	}
	pool := filtered
	if len(pool) == 0 {
		pool = resolveEligibleModes(bucket, cfg)
	}
	if len(pool) == 0 {
		pool = sequence
	}
	pos := totalAttempts - 1
	if pos < 0 {
		pos = 0
	}
	return pool[pos%len(pool)]
}
