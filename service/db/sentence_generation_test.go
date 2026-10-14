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

	v, err := s.GetSentenceVocab(ctx, testUserID, []string{"topic-food"}, 30)
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

	v, err := s.GetSentenceVocab(ctx, testUserID, []string{"topic-food"}, 2)
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

	v, err := s.GetSentenceVocab(ctx, testUserID, nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		text, pinyin string
		ok           bool
	}{
		{"你吃米饭，我吃米饭！", "nǐ chī mǐ fàn, wǒ chī mǐ fàn!", true},
		{"你吃面条。", "", false},   // 面条 is not known
		{" 我吃米饭。", "", false}, // the user already has this sentence
	}
	for _, c := range cases {
		pinyin, ok := v.Check(c.text)
		if ok != c.ok || pinyin != c.pinyin {
			t.Errorf("Check(%q) = %q, %v; want %q, %v", c.text, pinyin, ok, c.pinyin, c.ok)
		}
	}
}
