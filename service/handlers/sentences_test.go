package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/llm"
	"vocabulary_trainer/models"

	"github.com/go-chi/chi/v5"
)

// seedKnownWord creates a zh word for user 2 and marks it as reviewed once.
func seedKnownWord(t *testing.T, s *db.Store, zh, pinyin, en string, tags ...string) {
	t.Helper()
	ctx := context.Background()
	id, err := s.CreateWord(ctx, 2, models.CreateWordRequest{
		ZhText: zh, Pinyin: pinyin, Translations: map[string][]string{"en": {en}}, Tags: tags,
	})
	if err != nil {
		t.Fatalf("CreateWord %s: %v", zh, err)
	}
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil {
		t.Fatalf("GetSM2Progress %s: %v", zh, err)
	}
	p.TotalAttempts, p.TotalCorrect = 1, 1
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatalf("UpdateSM2Progress %s: %v", zh, err)
	}
}

func buildSentenceRouter(t *testing.T, client llm.Client, userID int64) (http.Handler, *db.Store) {
	t.Helper()
	store := openTestDB(t)
	seedKnownWord(t, store, "我", "wǒ", "I", "hsk1")
	seedKnownWord(t, store, "吃", "chī", "eat", "topic-food")
	seedKnownWord(t, store, "米饭", "mǐ fàn", "rice", "topic-food")
	seedKnownWord(t, store, "你", "nǐ", "you", "hsk1")
	seedKnownWord(t, store, "吗", "ma", "question particle", "hsk1")
	llmH := &handlers.LLMHandler{Client: client, Store: store}
	r := chi.NewRouter()
	r.Use(handlers.WithUserID(userID))
	r.Post("/api/sentences/generate", llmH.GenerateSentences)
	return r, store
}

func postGenerateSentences(t *testing.T, r http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sentences/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

type generatedSentences struct {
	FocusWords []string `json:"focus_words"`
	Sentences  []struct {
		ZhText       string              `json:"zh_text"`
		Pinyin       string              `json:"pinyin"`
		Translations map[string][]string `json:"translations"`
	} `json:"sentences"`
}

func TestGenerateSentences_KeepsOnlySentencesOfKnownWords(t *testing.T) {
	llmOut := "```json\n[" +
		`{"zh":"我吃米饭。","pinyin":"wo chi mifan","translations":{"en":"I eat rice."}},` +
		`{"zh":"我吃面条。","pinyin":"wo chi miantiao","translations":{"en":"I eat noodles."}},` +
		`{"zh":"我吃米饭。","pinyin":"wo chi mifan","translations":{"en":"I eat rice."}},` +
		`{"zh":"你吃米饭吗？","pinyin":"ni chi mifan ma?","translations":{"en":"Do you eat rice?"}}` +
		"]\n```"
	r, _ := buildSentenceRouter(t, &mockLLMClient{text: llmOut}, 2)

	rec := postGenerateSentences(t, r, `{"tags":["topic-food"],"count":5}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var got generatedSentences
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 面条 is unknown and the second 我吃米饭。 is a repeat: both are dropped.
	if len(got.Sentences) != 2 {
		t.Fatalf("sentences = %+v, want 2", got.Sentences)
	}
	first := got.Sentences[0]
	if first.ZhText != "我吃米饭。" || first.Pinyin != "wǒ chī mǐ fàn." {
		t.Errorf("first = %+v, want 我吃米饭。 with built pinyin wǒ chī mǐ fàn.", first)
	}
	if en := first.Translations["en"]; len(en) != 1 || en[0] != "I eat rice." {
		t.Errorf("first translations = %v", first.Translations)
	}
	if len(got.FocusWords) != 2 {
		t.Errorf("focus_words = %v, want the 2 topic-food words", got.FocusWords)
	}
}

func TestGenerateSentences_StopsAtCount(t *testing.T) {
	llmOut := `[{"zh":"我吃米饭。","translations":{"en":"I eat rice."}},{"zh":"你吃米饭吗？","translations":{"en":"Do you eat rice?"}}]`
	r, _ := buildSentenceRouter(t, &mockLLMClient{text: llmOut}, 2)

	rec := postGenerateSentences(t, r, `{"tags":["topic-food"],"count":1}`)
	var got generatedSentences
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Sentences) != 1 {
		t.Fatalf("sentences = %+v, want 1", got.Sentences)
	}
}

func TestGenerateSentences_PromptListsFocusAndOtherWords(t *testing.T) {
	var sent llm.Request
	r, _ := buildSentenceRouter(t, &captureLLMClient{out: &sent, text: "[]"}, 2)

	rec := postGenerateSentences(t, r, `{"tags":["topic-food"],"count":10}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	focus := between(sent.User, "<focus_words>", "</focus_words>")
	others := between(sent.User, "<other_words>", "</other_words>")
	for _, w := range []string{"吃", "米饭"} {
		if !strings.Contains(focus, w) || strings.Contains(others, w) {
			t.Errorf("%s: want it in focus words only; focus %q, others %q", w, focus, others)
		}
	}
	for _, w := range []string{"我", "你", "吗"} {
		if !strings.Contains(others, w) {
			t.Errorf("%s: want it in other words %q", w, others)
		}
	}
	// Asks for 1.5× the count because sentences with unknown words are dropped.
	if !strings.Contains(sent.User, "Write 15 ") {
		t.Errorf("prompt does not ask for 15 sentences: %s", sent.User)
	}
	for _, want := range []string{"equally", "day-to-day", `"en" (English)`} {
		if !strings.Contains(sent.User, want) {
			t.Errorf("prompt is missing %q: %s", want, sent.User)
		}
	}
}

func between(s, open, close string) string {
	// The rules text names the tags too: the data block is the last one.
	i := strings.LastIndex(s, open)
	j := strings.LastIndex(s, close)
	if i < 0 || j < i {
		return ""
	}
	return s[i+len(open) : j]
}

func TestGenerateSentences_Errors(t *testing.T) {
	cases := []struct {
		name   string
		client llm.Client
		userID int64
		body   string
		want   int
	}{
		{"no LLM configured", nil, 2, `{"tags":[],"count":5}`, http.StatusServiceUnavailable},
		{"free user without own key", &mockLLMClient{text: "[]"}, 99, `{"tags":[],"count":5}`, http.StatusForbidden},
		{"invalid JSON", &mockLLMClient{text: "[]"}, 2, `nope`, http.StatusBadRequest},
		{"count too high", &mockLLMClient{text: "[]"}, 2, `{"tags":[],"count":21}`, http.StatusBadRequest},
		{"count missing", &mockLLMClient{text: "[]"}, 2, `{"tags":[]}`, http.StatusBadRequest},
		{"no known words in tags", &mockLLMClient{text: "[]"}, 2, `{"tags":["topic-travel"],"count":5}`, http.StatusBadRequest},
		{"unknown focus", &mockLLMClient{text: "[]"}, 2, `{"tags":[],"count":5,"focus":"newest"}`, http.StatusBadRequest},
		{"mix-ups focus without mix-ups", &mockLLMClient{text: "[]"}, 2, `{"tags":[],"count":5,"focus":"mixups"}`, http.StatusBadRequest},
		{"LLM reply is not JSON", &mockLLMClient{text: "Sorry, I cannot."}, 2, `{"tags":[],"count":5}`, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := buildSentenceRouter(t, c.client, c.userID)
			rec := postGenerateSentences(t, r, c.body)
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, c.want, rec.Body)
			}
		})
	}
}

// setProgress sets the review counts of user 2's zh word text.
func setProgress(t *testing.T, s *db.Store, zh string, correct, attempts int) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := s.GetWordIDByZhText(ctx, 2, zh)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.GetSM2Progress(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	p.TotalAttempts, p.TotalCorrect = attempts, correct
	if err := s.UpdateSM2Progress(ctx, *p); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGenerateSentences_FocusOptionOrdersFocusWords(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		// 米饭 is the hardest word, but it is already in a sentence.
		{"hardest, even coverage by default", `{"tags":["topic-food"],"count":5,"focus":"hardest"}`, []string{"吃", "米饭"}},
		{"hardest without even coverage", `{"tags":["topic-food"],"count":5,"focus":"hardest","even":false}`, []string{"米饭", "吃"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, store := buildSentenceRouter(t, &mockLLMClient{text: "[]"}, 2)
			setProgress(t, store, "米饭", 1, 5)
			setProgress(t, store, "吃", 5, 5)
			if _, err := store.CreateWord(context.Background(), 2, models.CreateWordRequest{
				ZhText: "米饭，米饭！", Translations: map[string][]string{"en": {"Rice, rice!"}}, Tags: []string{"food_s"},
			}); err != nil {
				t.Fatal(err)
			}
			rec := postGenerateSentences(t, r, c.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
			}
			var got generatedSentences
			json.NewDecoder(rec.Body).Decode(&got)
			if strings.Join(got.FocusWords, " ") != strings.Join(c.want, " ") {
				t.Errorf("focus_words = %v, want %v", got.FocusWords, c.want)
			}
		})
	}
}

func TestGenerateSentences_MixUpsPromptAsksForBothWordsOfAPair(t *testing.T) {
	var sent llm.Request
	r, store := buildSentenceRouter(t, &captureLLMClient{out: &sent, text: "[]"}, 2)
	rice := setProgress(t, store, "米饭", 1, 1)
	eat := setProgress(t, store, "吃", 1, 1)
	if err := store.UpsertConfusion(context.Background(), 2, rice, eat, models.ModeZhToTransl); err != nil {
		t.Fatal(err)
	}

	rec := postGenerateSentences(t, r, `{"tags":[],"count":5,"focus":"mixups"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	if pairs := between(sent.User, "<pairs>", "</pairs>"); pairs != "米饭/吃" {
		t.Errorf("pairs = %q, want 米饭/吃; prompt: %s", pairs, sent.User)
	}
	if !strings.Contains(sent.User, "both words of a pair") {
		t.Errorf("prompt has no pair rule: %s", sent.User)
	}

	// Other focus options have no pairs.
	postGenerateSentences(t, r, `{"tags":[],"count":5}`)
	if strings.Contains(sent.User, "<pairs>") || strings.Contains(sent.User, "both words of a pair") {
		t.Errorf("prompt without mix-ups focus has pairs: %s", sent.User)
	}
}
