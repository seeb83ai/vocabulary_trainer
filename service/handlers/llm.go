package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"vocabulary_trainer/db"
	"vocabulary_trainer/llm"
)

type LLMHandler struct {
	Client llm.Client
	Store  *db.Store
}

type llmGenerateRequest struct {
	Actor    string   `json:"actor"`
	Location string   `json:"location"`
	Room     string   `json:"room"`
	Props    []string `json:"props"`
}

type llmGenerateResponse struct {
	Text string `json:"text"`
}

const llmSystemPrompt = `You are a creative writing assistant for the Hanzi Movie Method, a mnemonic system for memorizing Chinese characters. Your task is to write exactly one short, vivid scene. Respond with only the scene text — no preamble, no numbering, no labels, no explanation.`

func (h *LLMHandler) GenerateScene(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid word id")
		return
	}

	word, err := h.Store.GetWordByID(r.Context(), id)
	if err != nil || word == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	var req llmGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Determine initial/final/tone from the word's pinyin — not from the request.
	var initial, final string
	var tone int
	if word.Pinyin != nil && *word.Pinyin != "" {
		initial, final, tone = parsePinyin(*word.Pinyin)
	}
	initialDisplay := initial
	if initial == "null" || initial == "" {
		initialDisplay = "Ø"
	}
	finalDisplay := final
	if final == "null" || final == "" {
		finalDisplay = "Ø"
	}

	// Sanitize user-controlled fields before interpolating into the prompt.
	actor    := sanitizeLLMField(req.Actor)
	location := sanitizeLLMField(req.Location)
	room     := sanitizeLLMField(req.Room)
	props    := make([]string, 0, len(req.Props))
	for _, p := range req.Props {
		if s := sanitizeLLMField(p); s != "" {
			props = append(props, s)
		}
	}
	propsStr := "(none)"
	if len(props) > 0 {
		propsStr = strings.Join(props, ", ")
	}

	// zh_text and en_texts come from the DB — not from the request.
	zhText  := word.ZhText
	enTexts := strings.Join(word.EnTexts, ", ")
	if enTexts == "" {
		enTexts = "unknown"
	}

	// User message: trusted template with untrusted values inside XML tags.
	// XML tags make it clear to the LLM which parts are data vs instructions,
	// reducing the risk of injected content hijacking the task.
	userMsg := fmt.Sprintf(
		"Chinese word: <word>%s</word>\n"+
			"Meaning: <meaning>%s</meaning>\n"+
			"Actor: <actor>%s</actor> (initial consonant: <initial>%s</initial>)\n"+
			"Location: <location>%s</location> (final sound: <final>%s</final>)\n"+
			"Room: <room>%s</room> (tone: <tone>%d</tone>)\n"+
			"Props: <props>%s</props>\n\n"+
			"Write one vivid, memorable movie scene where the actor is in the location, "+
			"in the room, interacting with the props in a way that encodes the word's meaning. "+
			"Be concrete, visual, and strange enough to be memorable.",
		zhText, enTexts,
		actor, initialDisplay,
		location, finalDisplay,
		room, tone,
		propsStr,
	)

	text, err := h.Client.Generate(r.Context(), llm.Request{
		System: llmSystemPrompt,
		User:   userMsg,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LLM request failed")
		return
	}
	writeJSON(w, http.StatusOK, llmGenerateResponse{Text: text})
}

// sanitizeLLMField strips control characters (including newlines) and limits
// length to 100 runes to reduce prompt injection surface.
func sanitizeLLMField(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 32 {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	runes := []rune(out)
	if len(runes) > 100 {
		out = string(runes[:100])
	}
	return out
}
