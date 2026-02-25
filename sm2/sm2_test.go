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
