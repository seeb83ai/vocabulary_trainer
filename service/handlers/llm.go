package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
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
	actor := sanitizeLLMField(req.Actor)
	location := sanitizeLLMField(req.Location)
	room := sanitizeLLMField(req.Room)
	props := make([]string, 0, len(req.Props))
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
	zhText := word.ZhText
	enTexts := strings.Join(word.EnTexts, ", ")
	if enTexts == "" {
		enTexts = "unknown"
	}

	// User message: trusted template with untrusted values inside XML tags.
	// XML tags make it clear to the LLM which parts are data vs instructions,
	// reducing the risk of injected content hijacking the task.
	// TODO: response should be written in language as defined by user
	userMsg := fmt.Sprintf(
		"Chinese word: <word>%s</word>\n"+
			"Meaning: <meaning>%s</meaning>\n"+
			"Actor: <actor>%s</actor> (initial consonant: <initial>%s</initial>)\n"+
			"Location: <location>%s</location> (final sound: <final>%s</final>)\n"+
			"Room: <room>%s</room> (tone: <tone>%d</tone>)\n"+
			"Props: <props>%s</props>\n\n"+
			"Answer in German\n"+
			"Write one vivid, memorable movie scene where the actor is in the location, "+
			"in the room, interacting with the props in a way that encodes the word's meaning. "+
			"Be concrete, visual, and strange enough to be memorable.",
		zhText, enTexts,
		actor, initialDisplay,
		location, finalDisplay,
		room, tone,
		propsStr,
	)

	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := h.Client.Generate(r.Context(), llm.Request{
			System: llmSystemPrompt,
			User:   userMsg,
		})
		done <- result{text, err}
	}()

	// Flush a space byte every 5s so the WSL2→Windows TCP connection is not
	// treated as idle and reset before the LLM finishes generating.
	flusher, canFlush := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/json")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case res := <-done:
			if res.err != nil {
				log.Printf("Error: LLM request failed: %v\n", res.err)
				// Headers already sent; write a JSON error body and let the
				// client detect it via the error field.
				w.Write([]byte(`{"error":"LLM request failed"}`)) //nolint:errcheck
				return
			}
			log.Printf("LLM response: %v\n", res.text)
			json.NewEncoder(w).Encode(llmGenerateResponse{Text: res.text})
			return
		case <-ticker.C:
			if canFlush {
				w.Write([]byte(" ")) //nolint:errcheck
				flusher.Flush()
			}
		}
	}
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
