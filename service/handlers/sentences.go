package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/llm"
)

// Limits for LLM sentence generation.
const (
	maxSentenceCount   = 20   // sentences per request
	maxSentenceFocus   = 30   // focus words per request
	maxSentenceAllowed = 1000 // other allowed words listed in the prompt
)

type generateSentencesRequest struct {
	Tags  []string `json:"tags"`
	Count int      `json:"count"`
	Focus string   `json:"focus"` // db.SentenceFocus*; "" means all
	Even  *bool    `json:"even"`  // nil means true
}

type generatedSentence struct {
	ZhText       string              `json:"zh_text"`
	Pinyin       string              `json:"pinyin"`
	Translations map[string][]string `json:"translations"`
}

type generateSentencesResponse struct {
	FocusWords []string            `json:"focus_words"`
	Sentences  []generatedSentence `json:"sentences"`
}

// llmSentence is one item of the JSON array the LLM is asked to return.
type llmSentence struct {
	Zh           string            `json:"zh"`
	Pinyin       string            `json:"pinyin"`
	Translations map[string]string `json:"translations"`
}

const sentenceSystemPrompt = `You write example sentences for a learner of Mandarin Chinese (simplified characters). You follow the word rules exactly. Respond with only a JSON array — no preamble, no explanation, no markdown.`

var sentenceLangNames = map[string]string{"en": "English", "de": "German"}

// GenerateSentences asks the LLM for example sentences built from the user's
// known words, around the known words of the selected tags (focus words).
// Sentences with a word the user does not know yet, and sentences the user
// already has, are dropped; the pinyin comes from the user's own words.
func (h *LLMHandler) GenerateSentences(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientFor(w, r)
	if !ok {
		return
	}
	userID := UserIDFromContext(r.Context())

	var req generateSentencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Count < 1 || req.Count > maxSentenceCount {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("count must be between 1 and %d", maxSentenceCount))
		return
	}

	switch req.Focus {
	case "":
		req.Focus = db.SentenceFocusAll
	case db.SentenceFocusAll, db.SentenceFocusHardest, db.SentenceFocusMixUps, db.SentenceFocusFrequent:
	default:
		writeError(w, http.StatusBadRequest, "focus must be all, hardest, mixups or frequent")
		return
	}
	even := req.Even == nil || *req.Even

	vocab, err := h.Store.GetSentenceVocab(r.Context(), userID, req.Tags, req.Focus, even, maxSentenceFocus)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load known words")
		return
	}
	if len(vocab.Focus) == 0 {
		if req.Focus == db.SentenceFocusMixUps {
			writeError(w, http.StatusBadRequest, "no mix-ups of known words in the selected tags")
		} else {
			writeError(w, http.StatusBadRequest, "no known words in the selected tags")
		}
		return
	}

	langs := []string{"en"}
	if st, _ := h.Store.GetUserSettings(r.Context(), userID); st != nil && st.PrimaryLang != "" {
		langs = []string{st.PrimaryLang}
		if st.SecondaryLang != "" {
			langs = append(langs, st.SecondaryLang)
		}
	}

	userMsg := sentencePrompt(vocab.Focus, vocab.Allowed, vocab.Pairs, langs, (req.Count*3+1)/2)

	writeLLMJSON(w, func() (any, error) {
		text, err := client.Generate(r.Context(), llm.Request{System: sentenceSystemPrompt, User: userMsg})
		if err != nil {
			return nil, err
		}
		items, err := parseLLMSentences(text)
		if err != nil {
			return nil, err
		}
		resp := generateSentencesResponse{FocusWords: vocab.Focus, Sentences: []generatedSentence{}}
		seen := map[string]bool{}
		for _, it := range items {
			zh := strings.TrimSpace(it.Zh)
			pinyin, ok := vocab.Check(zh)
			if !ok || seen[zh] {
				continue
			}
			seen[zh] = true
			if pinyin == "" {
				pinyin = strings.TrimSpace(it.Pinyin)
			}
			transl := map[string][]string{}
			for _, lang := range langs {
				if t := strings.TrimSpace(it.Translations[lang]); t != "" {
					transl[lang] = []string{t}
				}
			}
			if len(transl) == 0 {
				continue
			}
			resp.Sentences = append(resp.Sentences, generatedSentence{ZhText: zh, Pinyin: pinyin, Translations: transl})
			if len(resp.Sentences) == req.Count {
				break
			}
		}
		return resp, nil
	})
}

// sentencePrompt builds the user message for GenerateSentences. Word lists
// come from the DB and sit inside XML tags so the LLM treats them as data.
// With pairs (mix-ups), the LLM is asked to contrast both words of a pair.
func sentencePrompt(focus, allowed []string, pairs [][2]string, langs []string, n int) string {
	isFocus := map[string]bool{}
	for _, f := range focus {
		isFocus[f] = true
	}
	var others []string
	for _, a := range allowed {
		if !isFocus[a] && len(others) < maxSentenceAllowed {
			others = append(others, sanitizeLLMField(a))
		}
	}
	var focusClean []string
	for _, f := range focus {
		focusClean = append(focusClean, sanitizeLLMField(f))
	}
	var langList, langKeys []string
	for _, l := range langs {
		name := sentenceLangNames[l]
		if name == "" {
			name = l
		}
		langList = append(langList, fmt.Sprintf("%q (%s)", l, name))
		langKeys = append(langKeys, fmt.Sprintf("%q:\"…\"", l))
	}
	pairRule, pairData := "", ""
	if len(pairs) > 0 {
		var list []string
		for _, p := range pairs {
			list = append(list, sanitizeLLMField(p[0])+"/"+sanitizeLLMField(p[1]))
		}
		pairRule = "6. The learner mixes up the words of each pair in <pairs>. Where it is natural, use both words of a pair in the same sentence, so the difference becomes clear.\n"
		pairData = "<pairs>" + strings.Join(list, " ") + "</pairs>\n"
	}
	return fmt.Sprintf(
		"Write %d different Chinese sentences.\n\n"+
			"Rules:\n"+
			"1. Use only words from <focus_words> and <other_words>, plus punctuation. Do not use any other word or character — no extra particles, measure words, numbers or names.\n"+
			"2. Every sentence contains at least one focus word. Use all focus words about equally often across the sentences: do not use a focus word again before every focus word is used.\n"+
			"3. Every sentence is useful in day-to-day life: something people really say, read or hear (at home, shopping, eating, travel, work, small talk, signs, messages). No textbook clichés, no nonsense, no poetry.\n"+
			"4. Keep the sentences short and natural. Mix statements, questions and requests.\n"+
			"5. Translate every sentence naturally into %s.\n"+
			"%s\n"+
			"<focus_words>%s</focus_words>\n"+
			"<other_words>%s</other_words>\n"+
			"%s\n"+
			"Output: a JSON array of objects {\"zh\":\"…\",\"pinyin\":\"…\",\"translations\":{%s}}.",
		n, strings.Join(langList, " and "), pairRule,
		strings.Join(focusClean, " "), strings.Join(others, " "), pairData,
		strings.Join(langKeys, ","),
	)
}

// parseLLMSentences extracts the JSON array from an LLM reply, ignoring any
// text or markdown fence around it.
func parseLLMSentences(text string) ([]llmSentence, error) {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end < start {
		return nil, fmt.Errorf("LLM reply has no JSON array: %q", text)
	}
	var items []llmSentence
	if err := json.Unmarshal([]byte(text[start:end+1]), &items); err != nil {
		return nil, fmt.Errorf("parse LLM sentences: %w", err)
	}
	return items, nil
}
