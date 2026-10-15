package db

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestGetSentenceVocab_FocusIsKnownWordsOfTags_AllowedIsAllKnownWords(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	rice := seedWordWithTags(t, s, "米饭", "mǐ fàn", []string{"rice"}, []string{"topic-food"})
	eat := seedWordWithTags(t, s, "吃", "chī", []string{"eat"}, []string{"topic-food"})
	seedWordWithTags(t, s, "面条", "miàn tiáo", []string{"noodles"}, []string{"topic-food"}) // never reviewed
	wo := seedWordWithTags(t, s, "我", "wǒ", []string{"I"}, []string{"hsk1"})
	markReviewed(t, s, rice, 1, 1, now)
	markReviewed(t, s, eat, 1, 1, now)
	markReviewed(t, s, wo, 1, 1, now)

	v, err := s.GetSentenceVocab(ctx, testUserID, []string{"topic-food"}, SentenceFocusAll, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	focus := slices.Sorted(slices.Values(v.Focus))
	if !slices.Equal(focus, []string{"吃", "米饭"}) {
		t.Errorf("focus = %v, want [吃 米饭]", focus)
	}
	allowed := slices.Sorted(slices.Values(v.Allowed))
	if !slices.Equal(allowed, []string{"吃", "我", "米饭"}) {
		t.Errorf("allowed = %v, want [吃 我 米饭]", allowed)
	}
}

func TestGetSentenceVocab_FocusPrefersWordsLeastUsedInSentences(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []int64{
		seedWordWithTags(t, s, "米饭", "mǐ fàn", []string{"rice"}, []string{"topic-food"}),
		seedWordWithTags(t, s, "吃", "chī", []string{"eat"}, []string{"topic-food"}),
		seedWordWithTags(t, s, "面条", "miàn tiáo", []string{"noodles"}, []string{"topic-food"}),
		seedWord(t, s, "我", "wǒ", []string{"I"}),
	} {
		markReviewed(t, s, id, 1, 1, now)
	}
	seedWordWithTags(t, s, "我吃米饭。", "wǒ chī mǐ fàn.", []string{"I eat rice."}, []string{"food_s"})
	seedWordWithTags(t, s, "米饭，米饭！", "mǐ fàn, mǐ fàn!", []string{"Rice, rice!"}, []string{"food_s"})

	v, err := s.GetSentenceVocab(ctx, testUserID, []string{"topic-food"}, SentenceFocusAll, true, 2)
	if err != nil {
		t.Fatal(err)
	}
	// 面条 is in no sentence, 吃 in one, 米饭 three times.
	if !slices.Equal(v.Focus, []string{"面条", "吃"}) {
		t.Errorf("focus = %v, want [面条 吃]", v.Focus)
	}
}

func TestSentenceVocabCheck_AcceptsKnownWordsOnlyAndBuildsPinyin(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []int64{
		seedWord(t, s, "我", "wǒ", []string{"I"}),
		seedWord(t, s, "吃", "chī", []string{"eat"}),
		seedWord(t, s, "米饭", "mǐ fàn", []string{"rice"}),
		seedWord(t, s, "你", "nǐ", []string{"you"}),
	} {
		markReviewed(t, s, id, 1, 1, now)
	}
	seedWordWithTags(t, s, "我吃米饭。", "wǒ chī mǐ fàn.", []string{"I eat rice."}, []string{"food_s"})
	seedWord(t, s, "你吃", "nǐ chī", []string{"not reviewed yet"})

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, SentenceFocusAll, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		text, pinyin string
		ok           bool
	}{
		{"你吃米饭，我吃米饭！", "nǐ chī mǐ fàn, wǒ chī mǐ fàn!", true},
		{"你吃面条。", "", false},  // 面条 is not known
		{" 我吃米饭。", "", false}, // the user already has this sentence
		{"你吃？", "", false},    // the user has this word (not reviewed yet)
		{"米饭！", "", false},    // one word is no sentence: adding it would tag the word as a sentence
	}
	for _, c := range cases {
		pinyin, ok := v.Check(c.text)
		if ok != c.ok || pinyin != c.pinyin {
			t.Errorf("Check(%q) = %q, %v; want %q, %v", c.text, pinyin, ok, c.pinyin, c.ok)
		}
	}
}

// setDifficulty sets the fields that rank a word for the Hardest focus.
func setDifficulty(t *testing.T, s *Store, wordID int64, correct, attempts int, easiness float64) {
	t.Helper()
	markReviewed(t, s, wordID, correct, attempts, time.Now())
	if _, err := s.db.Exec(`UPDATE sm2_progress SET easiness = ? WHERE word_id = ?`, easiness, wordID); err != nil {
		t.Fatal(err)
	}
}

func TestGetSentenceVocab_HardestFocusRanksLowAccuracyThenLowEasinessFirst(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	setDifficulty(t, s, seedWord(t, s, "米饭", "mǐ fàn", []string{"rice"}), 9, 10, 2.5)
	setDifficulty(t, s, seedWord(t, s, "吃", "chī", []string{"eat"}), 3, 10, 2.5)
	setDifficulty(t, s, seedWord(t, s, "面条", "miàn tiáo", []string{"noodles"}), 0, 2, 1.3) // too few attempts
	setDifficulty(t, s, seedWord(t, s, "喝", "hē", []string{"drink"}), 3, 10, 1.5)

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, SentenceFocusHardest, false, 30)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"喝", "吃", "米饭", "面条"}; !slices.Equal(v.Focus, want) {
		t.Errorf("focus = %v, want %v", v.Focus, want)
	}
}

func TestGetSentenceVocab_EvenCoverageComesFirstAndFocusBreaksTies(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	setDifficulty(t, s, seedWord(t, s, "米饭", "mǐ fàn", []string{"rice"}), 9, 10, 2.5)
	setDifficulty(t, s, seedWord(t, s, "吃", "chī", []string{"eat"}), 1, 10, 2.5) // hardest, but already in a sentence
	setDifficulty(t, s, seedWord(t, s, "喝", "hē", []string{"drink"}), 5, 10, 2.5)
	seedWordWithTags(t, s, "吃米饭！", "chī mǐ fàn!", []string{"Eat rice!"}, []string{"food_s"})
	seedWordWithTags(t, s, "吃，吃！", "chī, chī!", []string{"Eat, eat!"}, []string{"food_s"})

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, SentenceFocusHardest, true, 30)
	if err != nil {
		t.Fatal(err)
	}
	// 喝 is in no sentence, 米饭 in one, 吃 in two.
	if want := []string{"喝", "米饭", "吃"}; !slices.Equal(v.Focus, want) {
		t.Errorf("focus = %v, want %v", v.Focus, want)
	}
}

func TestGetSentenceVocab_FrequentFocusRanksCommonWordsFirst(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	markReviewed(t, s, seedWord(t, s, "米饭", "mǐ fàn", []string{"rice"}), 1, 1, now)
	markReviewed(t, s, seedWord(t, s, "吃", "chī", []string{"eat"}), 1, 1, now)
	markReviewed(t, s, seedWord(t, s, "无频词", "wú pín cí", []string{"not in the list"}), 1, 1, now)
	markReviewed(t, s, seedWord(t, s, "喝", "hē", []string{"drink"}), 1, 1, now)
	if _, err := s.db.Exec(`DELETE FROM word_frequency_lang WHERE lang = 'zh' AND word IN ('米饭','吃','无频词','喝')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO word_frequency_lang (word, lang, rank) VALUES ('米饭','zh',900), ('吃','zh',50), ('喝','zh',300), ('喝','en',1)`); err != nil {
		t.Fatal(err)
	}

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, SentenceFocusFrequent, false, 30)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"吃", "喝", "米饭", "无频词"}; !slices.Equal(v.Focus, want) {
		t.Errorf("focus = %v, want %v", v.Focus, want)
	}
}

func TestGetSentenceVocab_MixUpsFocusUsesKnownPairsMostMixedUpFirst(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	rice := seedWord(t, s, "米饭", "mǐ fàn", []string{"rice"})
	eat := seedWord(t, s, "吃", "chī", []string{"eat"})
	drink := seedWord(t, s, "喝", "hē", []string{"drink"})
	noodles := seedWord(t, s, "面条", "miàn tiáo", []string{"noodles"})
	you := seedWord(t, s, "你", "nǐ", []string{"you"})  // known, no mix-up
	tea := seedWord(t, s, "茶", "chá", []string{"tea"}) // not reviewed yet
	for _, id := range []int64{rice, eat, drink, noodles, you} {
		markReviewed(t, s, id, 1, 1, now)
	}
	mixUp := func(a, b int64, mode string, n int) {
		for range n {
			if err := s.UpsertConfusion(ctx, testUserID, a, b, mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	mixUp(eat, drink, "zh_to_transl", 3)
	mixUp(drink, eat, "transl_to_zh", 1)
	mixUp(eat, rice, "zh_to_transl", 2)
	mixUp(rice, noodles, "zh_to_transl", 1)
	mixUp(eat, tea, "zh_to_transl", 9) // 茶 is not known: no pair
	if _, err := s.db.Exec(`INSERT INTO confusion_pairs (user_id, zh_word_id, confused_with_component, mode, count)
		VALUES (?, ?, '米', 'zh_to_transl', 9)`, testUserID, noodles); err != nil {
		t.Fatal(err)
	}

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, SentenceFocusMixUps, false, 30)
	if err != nil {
		t.Fatal(err)
	}
	// 吃 6 mix-ups, 喝 4, 米饭 3, 面条 1 (component mix-ups do not count).
	if want := []string{"吃", "喝", "米饭", "面条"}; !slices.Equal(v.Focus, want) {
		t.Errorf("focus = %v, want %v", v.Focus, want)
	}
	var pairs []string
	for _, p := range v.Pairs {
		pair := []string{p[0], p[1]}
		slices.Sort(pair)
		pairs = append(pairs, pair[0]+"/"+pair[1])
	}
	slices.Sort(pairs)
	if want := []string{"吃/喝", "吃/米饭", "米饭/面条"}; !slices.Equal(pairs, want) {
		t.Errorf("pairs = %v, want %v", pairs, want)
	}
}
