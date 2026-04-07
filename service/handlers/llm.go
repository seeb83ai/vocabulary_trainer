package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"vocabulary_trainer/llm"
)

type LLMHandler struct {
	Client llm.Client
}

type llmSceneRequest struct {
	Prompt string `json:"prompt"`
}

type llmSceneResponse struct {
	Text string `json:"text"`
}

func (h *LLMHandler) GenerateScene(w http.ResponseWriter, r *http.Request) {
	var req llmSceneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	text, err := h.Client.Generate(r.Context(), req.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LLM request failed")
		return
	}
	writeJSON(w, http.StatusOK, llmSceneResponse{Text: text})
}
