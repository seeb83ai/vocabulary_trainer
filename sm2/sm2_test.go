package sm2

import (
	"testing"
	"time"
	"vocabulary_trainer/models"
)

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_CorrectFirstRep(t *testing.T) {
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, IntervalDays: 1}
	got := Update(p, QualityCorrect)

	if got.Repetitions != 1 {
		t.Errorf("repetitions: want 1, got %d", got.Repetitions)
	}
	if got.IntervalDays != 1 {
		t.Errorf("interval_days after first correct: want 1, got %d", got.IntervalDays)
	}
	// quality=4: EF delta = 0.1 - (5-4)*(0.08+(5-4)*0.02) = 0.1 - 0.10 = 0.0 → EF unchanged at 2.5
	// Only quality=5 strictly increases EF from 2.5.
	if got.Easiness < 2.5 {
		t.Errorf("easiness should not drop on a correct answer (quality=4), got %f", got.Easiness)
	}
}

func TestUpdate_Quality5IncreasesEasiness(t *testing.T) {
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, IntervalDays: 1}
	got := Update(p, 5)
	// quality=5: EF delta = 0.1 - 0*(0.08+0*0.02) = 0.1 → EF = 2.6
	if got.Easiness <= 2.5 {
		t.Errorf("easiness should increase after quality=5, got %f", got.Easiness)
	}
}

func TestUpdate_CorrectSecondRep(t *testing.T) {
	p := models.SM2Progress{Repetitions: 1, Easiness: 2.5, IntervalDays: 1}
	got := Update(p, QualityCorrect)

	if got.Repetitions != 2 {
		t.Errorf("repetitions: want 2, got %d", got.Repetitions)
	}
	if got.IntervalDays != 6 {
		t.Errorf("interval_days after second correct: want 6, got %d", got.IntervalDays)
	}
}

func TestUpdate_CorrectThirdRep(t *testing.T) {
	p := models.SM2Progress{Repetitions: 2, Easiness: 2.5, IntervalDays: 6}
	got := Update(p, QualityCorrect)

	if got.Repetitions != 3 {
		t.Errorf("repetitions: want 3, got %d", got.Repetitions)
	}
	// interval = round(6 * 2.5) = 15
	if got.IntervalDays != 15 {
		t.Errorf("interval_days: want 15, got %d", got.IntervalDays)
	}
}

func TestUpdate_WrongResetsRepetitions(t *testing.T) {
	before := time.Now()
	p := models.SM2Progress{Repetitions: 5, Easiness: 2.5, IntervalDays: 30}
	got := Update(p, QualityWrong)

	if got.Repetitions != 0 {
		t.Errorf("repetitions should reset to 0 after wrong, got %d", got.Repetitions)
	}
	if got.IntervalDays != 0 {
		t.Errorf("interval_days should reset to 0 after wrong, got %d", got.IntervalDays)
	}
	wantMin := before.Add(WrongRetryDelay)
	wantMax := time.Now().Add(WrongRetryDelay * 3)
	if got.DueDate.Before(wantMin) || got.DueDate.After(wantMax) {
		t.Errorf("due_date after wrong answer should be in [WrongRetryDelay, 3*WrongRetryDelay] from now, got %v", got.DueDate)
	}
}

func TestUpdate_EasinessFloorAt1_3(t *testing.T) {
	p := models.SM2Progress{Repetitions: 0, Easiness: 1.3, IntervalDays: 1}
	got := Update(p, QualityWrong) // quality=0 drives EF down

	if got.Easiness < 1.3 {
		t.Errorf("easiness must not drop below 1.3, got %f", got.Easiness)
	}
}

func TestUpdate_DueDateInFuture(t *testing.T) {
	before := time.Now().UTC()
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, IntervalDays: 1}
	got := Update(p, QualityCorrect)

	if !got.DueDate.After(before) {
		t.Errorf("due_date should be in the future, got %v", got.DueDate)
	}
}

func TestUpdate_CorrectDueDateJitter(t *testing.T) {
	before := time.Now().UTC()
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, IntervalDays: 1}
	got := Update(p, QualityCorrect)

	// intervalDays=1 → base is 24h; jitter is [-2h, 0h) → due in [22h, 24h)
	wantMin := before.Add(22 * time.Hour)
	wantMax := time.Now().UTC().Add(24 * time.Hour)
	if got.DueDate.Before(wantMin) || got.DueDate.After(wantMax) {
		t.Errorf("due_date after correct answer (1-day interval) should be in [22h, 24h] from now, got %v", got.DueDate)
	}
}

// ── UpdateLearning ───────────────────────────────────────────────────────────

func TestUpdateLearning_CorrectIncrementsReps(t *testing.T) {
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, LearningNewWord: true}
	got, graduated := UpdateLearning(p, QualityCorrect)

	if got.Repetitions != 1 {
		t.Errorf("repetitions: want 1, got %d", got.Repetitions)
	}
	if graduated {
		t.Error("should not graduate after 1 correct")
	}
	if !got.LearningNewWord {
		t.Error("should still be in learning phase")
	}
}

func TestUpdateLearning_WrongResetsReps(t *testing.T) {
	p := models.SM2Progress{Repetitions: 2, Easiness: 2.5, LearningNewWord: true}
	got, graduated := UpdateLearning(p, QualityWrong)

	if got.Repetitions != 0 {
		t.Errorf("repetitions should reset to 0, got %d", got.Repetitions)
	}
	if graduated {
		t.Error("should not graduate after wrong answer")
	}
	if !got.LearningNewWord {
		t.Error("should still be in learning phase after wrong")
	}
}

func TestUpdateLearning_GraduatesAfter3Correct(t *testing.T) {
	p := models.SM2Progress{Repetitions: 2, Easiness: 2.0, LearningNewWord: true, TotalCorrect: 5, TotalAttempts: 8}
	got, graduated := UpdateLearning(p, QualityCorrect)

	if !graduated {
		t.Error("should graduate after 3rd consecutive correct")
	}
	if got.LearningNewWord {
		t.Error("learning_new_word should be false after graduation")
	}
	if got.Repetitions != 0 {
		t.Errorf("repetitions should reset to 0, got %d", got.Repetitions)
	}
	if got.Easiness != 2.5 {
		t.Errorf("easiness should reset to 2.5, got %f", got.Easiness)
	}
	if got.TotalCorrect != 3 {
		t.Errorf("total_correct should reset to 3, got %d", got.TotalCorrect)
	}
	if got.TotalAttempts != 3 {
		t.Errorf("total_attempts should reset to 3, got %d", got.TotalAttempts)
	}
	if got.IntervalDays != 1 {
		t.Errorf("interval_days should be 1, got %d", got.IntervalDays)
	}
}

func TestUpdateLearning_ShortIntervals(t *testing.T) {
	before := time.Now()
	p := models.SM2Progress{Repetitions: 0, Easiness: 2.5, LearningNewWord: true}
	got, _ := UpdateLearning(p, QualityCorrect)

	// Due date should be within minutes, not days
	maxDelay := LearningCorrectDelay * 2
	wantMax := before.Add(maxDelay + time.Second)
	if got.DueDate.After(wantMax) {
		t.Errorf("learning due date should be within %v, got %v from now", maxDelay, got.DueDate.Sub(before))
	}
	if got.DueDate.Before(before) {
		t.Errorf("learning due date should be in the future")
	}
}

func TestUpdateLearning_WrongShortInterval(t *testing.T) {
	before := time.Now()
	p := models.SM2Progress{Repetitions: 1, Easiness: 2.5, LearningNewWord: true}
	got, _ := UpdateLearning(p, QualityWrong)

	maxDelay := WrongRetryDelay * 3
	wantMax := before.Add(maxDelay + time.Second)
	if got.DueDate.After(wantMax) {
		t.Errorf("wrong retry delay too long: %v", got.DueDate.Sub(before))
	}
}

// ── CheckAnswer ───────────────────────────────────────────────────────────────

func TestCheckAnswer_ExactMatch(t *testing.T) {
	if !CheckAnswer("hello", []string{"hello"}) {
		t.Error("exact match should be true")
	}
}

func TestCheckAnswer_CaseInsensitive(t *testing.T) {
	if !CheckAnswer("Hello", []string{"hello"}) {
		t.Error("case-insensitive match should be true")
	}
}

func TestCheckAnswer_TrimWhitespace(t *testing.T) {
	if !CheckAnswer("  hello  ", []string{"hello"}) {
		t.Error("leading/trailing whitespace should be trimmed")
	}
}

func TestCheckAnswer_MultipleAccepted(t *testing.T) {
	if !CheckAnswer("guten tag", []string{"hello", "guten tag"}) {
		t.Error("should match second accepted answer")
	}
}

func TestCheckAnswer_Wrong(t *testing.T) {
	if CheckAnswer("wrong", []string{"hello", "hi"}) {
		t.Error("wrong answer should return false")
	}
}

func TestCheckAnswer_EmptyAnswer(t *testing.T) {
	if CheckAnswer("", []string{"hello"}) {
		t.Error("empty answer should not match non-empty accepted")
	}
}

func TestCheckAnswer_OptionalParens_WithParens(t *testing.T) {
	// Full form is also accepted
	if !CheckAnswer("(das) Essen", []string{"(das) Essen"}) {
		t.Error("full form with parens should be accepted")
	}
}

func TestCheckAnswer_OptionalParens_WithoutParens(t *testing.T) {
	if !CheckAnswer("essen", []string{"(das) Essen"}) {
		t.Error("form without parens should be accepted")
	}
}

func TestCheckAnswer_OptionalParens_Middle(t *testing.T) {
	if !CheckAnswer("nicht verstehen", []string{"(das Gehörte) nicht verstehen"}) {
		t.Error("stripping parens from middle should be accepted")
	}
}

func TestCheckAnswer_Slash_FullForm(t *testing.T) {
	if !CheckAnswer("Essen / Gericht", []string{"Essen / Gericht"}) {
		t.Error("full slash form should be accepted")
	}
}

func TestCheckAnswer_Slash_FirstPart(t *testing.T) {
	if !CheckAnswer("essen", []string{"Essen / Gericht"}) {
		t.Error("first slash part should be accepted")
	}
}

func TestCheckAnswer_Slash_SecondPart(t *testing.T) {
	if !CheckAnswer("gericht", []string{"Essen / Gericht"}) {
		t.Error("second slash part should be accepted")
	}
}

func TestCheckAnswer_SlashAndParens_Combined(t *testing.T) {
	// "(das) Essen / Gericht":
	//   - full form:               "(das) essen / gericht"
	//   - parens stripped:         "essen / gericht"
	//   - slash parts of original: "(das) essen", "gericht"
	//   - slash part paren-stripped: "essen", "gericht"
	// "das essen" is NOT generated — parens are removed entirely, not expanded.
	accepted := []string{"(das) Essen / Gericht"}
	cases := []string{"essen", "gericht", "(das) essen / gericht", "(das) essen", "essen / gericht"}
	for _, c := range cases {
		if !CheckAnswer(c, accepted) {
			t.Errorf("expected %q to be accepted", c)
		}
	}
}

// ── Trailing punctuation stripping ───────────────────────────────────────────

func TestCheckAnswer_TrailingChinesePeriod_UserAdds(t *testing.T) {
	// User types "你好。" but stored answer is "你好"
	if !CheckAnswer("你好。", []string{"你好"}) {
		t.Error("trailing 。 in user answer should be ignored")
	}
}

func TestCheckAnswer_TrailingChinesePeriod_StoredHas(t *testing.T) {
	// Stored answer is "你好。" but user types "你好"
	if !CheckAnswer("你好", []string{"你好。"}) {
		t.Error("trailing 。 in stored answer should be ignored")
	}
}

func TestCheckAnswer_TrailingChinesePeriod_BothHave(t *testing.T) {
	if !CheckAnswer("你好。", []string{"你好。"}) {
		t.Error("both having 。 should still match")
	}
}

func TestCheckAnswer_TrailingASCIIPeriod(t *testing.T) {
	if !CheckAnswer("hello.", []string{"hello"}) {
		t.Error("trailing ASCII period in user answer should be ignored")
	}
}

func TestCheckAnswer_TrailingPunctNotMidString(t *testing.T) {
	// A period in the middle should not be stripped
	if CheckAnswer("hello", []string{"hel.lo"}) {
		t.Error("mid-string period should not be stripped")
	}
}

func TestCheckAnswer_TrailingComma(t *testing.T) {
	if !CheckAnswer("hello,", []string{"hello"}) {
		t.Error("trailing comma in user answer should be ignored")
	}
}

func TestCheckAnswer_TrailingColon(t *testing.T) {
	if !CheckAnswer("hello:", []string{"hello"}) {
		t.Error("trailing colon in user answer should be ignored")
	}
}

func TestCheckAnswer_TrailingWhitespace(t *testing.T) {
	if !CheckAnswer("hello   ", []string{"hello"}) {
		t.Error("trailing whitespace in user answer should be ignored")
	}
}

// ── SelectMode ────────────────────────────────────────────────────────────────

func TestSelectMode_ValidMode(t *testing.T) {
	validModes := map[string]bool{
		models.ModeEnToZh:       true,
		models.ModeZhToEn:       true,
		models.ModeZhPinyinToEn: true,
	}
	for i := 0; i < 50; i++ {
		m := SelectMode()
		if !validModes[m] {
			t.Errorf("SelectMode returned invalid mode %q", m)
		}
	}
}

func TestSelectMode_AllModesReachable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		seen[SelectMode()] = true
	}
	for _, m := range []string{models.ModeEnToZh, models.ModeZhToEn, models.ModeZhPinyinToEn} {
		if !seen[m] {
			t.Errorf("mode %q was never returned in 300 calls", m)
		}
	}
}

// ── SelectProgressiveMode ─────────────────────────────────────────────────────

func TestSelectProgressiveMode_ColdStart(t *testing.T) {
	// < 3 attempts always returns en_to_zh regardless of accuracy
	for _, tc := range []struct{ correct, attempts int }{{0, 0}, {1, 1}, {2, 2}} {
		if m := SelectProgressiveMode(tc.correct, tc.attempts); m != models.ModeEnToZh {
			t.Errorf("attempts=%d correct=%d: want en_to_zh, got %s", tc.attempts, tc.correct, m)
		}
	}
}

func TestSelectProgressiveMode_LowAccuracy(t *testing.T) {
	// accuracy < 50% → en_to_zh
	if m := SelectProgressiveMode(1, 3); m != models.ModeEnToZh { // 33%
		t.Errorf("33%% accuracy: want en_to_zh, got %s", m)
	}
	if m := SelectProgressiveMode(4, 9); m != models.ModeEnToZh { // 44%
		t.Errorf("44%% accuracy: want en_to_zh, got %s", m)
	}
}

func TestSelectProgressiveMode_MidAccuracyFewAttempts(t *testing.T) {
	// accuracy >= 50% but attempts < 10 → zh_pinyin_to_en
	if m := SelectProgressiveMode(2, 3); m != models.ModeZhPinyinToEn { // 67%, 3 attempts
		t.Errorf("67%% 3 attempts: want zh_pinyin_to_en, got %s", m)
	}
	if m := SelectProgressiveMode(8, 9); m != models.ModeZhPinyinToEn { // 89%, 9 attempts
		t.Errorf("89%% 9 attempts: want zh_pinyin_to_en, got %s", m)
	}
}

func TestSelectProgressiveMode_MidAccuracyEnoughAttempts(t *testing.T) {
	// 50% <= accuracy < 70%, attempts >= 10 → zh_pinyin_to_en
	if m := SelectProgressiveMode(5, 10); m != models.ModeZhPinyinToEn { // 50%
		t.Errorf("50%% 10 attempts: want zh_pinyin_to_en, got %s", m)
	}
	if m := SelectProgressiveMode(6, 10); m != models.ModeZhPinyinToEn { // 60%
		t.Errorf("60%% 10 attempts: want zh_pinyin_to_en, got %s", m)
	}
}

func TestSelectProgressiveMode_HighAccuracyEnoughAttempts(t *testing.T) {
	// 70% <= accuracy < 85%, attempts >= 10 → zh_to_en
	if m := SelectProgressiveMode(7, 10); m != models.ModeZhToEn { // 70%
		t.Errorf("70%% 10 attempts: want zh_to_en, got %s", m)
	}
	if m := SelectProgressiveMode(12, 15); m != models.ModeZhToEn { // 80%
		t.Errorf("80%% 15 attempts: want zh_to_en, got %s", m)
	}
}

func TestSelectProgressiveMode_Mastered(t *testing.T) {
	// accuracy >= 85% and attempts >= 10 → random valid mode
	validModes := map[string]bool{
		models.ModeEnToZh:       true,
		models.ModeZhToEn:       true,
		models.ModeZhPinyinToEn: true,
	}
	for i := 0; i < 50; i++ {
		m := SelectProgressiveMode(9, 10) // 90%, 10 attempts
		if !validModes[m] {
			t.Errorf("mastered: got invalid mode %s", m)
		}
	}
}

func TestSelectProgressiveMode_HighAccuracyFewAttempts(t *testing.T) {
	// accuracy >= 85% but attempts < 10 → zh_pinyin_to_en (not yet graduated)
	if m := SelectProgressiveMode(3, 3); m != models.ModeZhPinyinToEn { // 100%, 3 attempts
		t.Errorf("100%% 3 attempts: want zh_pinyin_to_en, got %s", m)
	}
}

// ── expandVariants (internal, tested via CheckAnswer above, but worth direct tests) ──

func TestExpandVariants_NoDuplicates(t *testing.T) {
	variants := expandVariants("hello")
	seen := map[string]bool{}
	for _, v := range variants {
		if seen[v] {
			t.Errorf("duplicate variant %q", v)
		}
		seen[v] = true
	}
}

func TestExpandVariants_NoEmpty(t *testing.T) {
	for _, v := range expandVariants("(foo) bar / baz") {
		if v == "" {
			t.Error("expandVariants returned an empty string")
		}
	}
}
