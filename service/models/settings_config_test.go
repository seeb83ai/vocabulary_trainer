package models

import "testing"

func TestUserSettingsQuizConfig(t *testing.T) {
	s := UserSettings{
		ProgNew:            "a",
		ProgTierStruggling: "b",
		ProgTierLearning:   "c",
		ProgTierPracticing: "d",
		ProgTierMastered:   "e",
	}
	got := s.QuizConfig()
	want := ProgressiveModeConfig{New: "a", Struggling: "b", Learning: "c", Practicing: "d", Mastered: "e"}
	if got != want {
		t.Errorf("QuizConfig() = %+v, want %+v", got, want)
	}
}

func TestUserSettingsNewWordConfig(t *testing.T) {
	s := UserSettings{NewWordMode0: "x", NewWordMode1: "y", NewWordMode2: "z"}
	got := s.NewWordConfig()
	want := NewWordModeConfig{Step0: "x", Step1: "y", Step2: "z"}
	if got != want {
		t.Errorf("NewWordConfig() = %+v, want %+v", got, want)
	}
}

// A zero-value (nil-equivalent) settings projects to all-empty fields, which the
// downstream mode resolution turns into the per-tier defaults. This documents the
// nil/default fallback contract used by QuizHandler.Next.
func TestZeroSettingsProjectsEmpty(t *testing.T) {
	var s UserSettings
	if (s.QuizConfig() != ProgressiveModeConfig{}) {
		t.Errorf("zero settings QuizConfig() should be empty, got %+v", s.QuizConfig())
	}
	if (s.NewWordConfig() != NewWordModeConfig{}) {
		t.Errorf("zero settings NewWordConfig() should be empty, got %+v", s.NewWordConfig())
	}
	if (s.RandomModeConfig() != RandomModeConfig{}) {
		t.Errorf("zero settings RandomModeConfig() should be empty, got %+v", s.RandomModeConfig())
	}
}

func TestUserSettingsRandomModeConfig(t *testing.T) {
	s := UserSettings{
		RandomModeRangeTranslToZh:        "new,50-69",
		RandomModeRangeZhToTransl:        "off",
		RandomModeRangeZhPinyinToTransl:  "new,70-84",
		RandomModeRangeZhToTranslNoSound: "50-69,85-100",
		RandomModeRangeVoiceToTransl:     "70-84,85-100",
	}
	got := s.RandomModeConfig()
	want := RandomModeConfig{
		TranslToZh:        "new,50-69",
		ZhToTransl:        "off",
		ZhPinyinToTransl:  "new,70-84",
		ZhToTranslNoSound: "50-69,85-100",
		VoiceToTransl:     "70-84,85-100",
	}
	if got != want {
		t.Errorf("RandomModeConfig() = %+v, want %+v", got, want)
	}
}
