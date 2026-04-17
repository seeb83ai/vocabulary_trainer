package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

// ImportHandler handles cross-user word import from the shared library user (user_id=1).
type ImportHandler struct {
	Store *db.Store
}

type importPreviewResponse struct {
	Tag      string   `json:"tag"`
	Total    int      `json:"total"`
	Examples []string `json:"examples"`
}

type importRequest struct {
	Tag       string   `json:"tag"`
	ImportEn  bool     `json:"import_en"`
	ImportDe  bool     `json:"import_de"`
	ApplyTags []string `json:"apply_tags"`
}

type importResponse struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

const sourceUserID int64 = 1

// SourceTags returns all tags belonging to the shared library user (user_id=1).
func (h *ImportHandler) SourceTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Store.GetAllTags(r.Context(), sourceUserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// Preview returns a brief summary of words that would be imported for a given tag.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	words, total, err := h.Store.GetWords(r.Context(), sourceUserID, "", 1, 0, "", "", []string{tag}, false, false, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load preview")
		return
	}

	examples := make([]string, 0, 5)
	for i, w := range words {
		if i >= 5 {
			break
		}
		examples = append(examples, w.ZhText)
	}

	writeJSON(w, http.StatusOK, importPreviewResponse{Tag: tag, Total: total, Examples: examples})
}

// Import fetches all words for the source user with the given tag and creates
// them for the requesting user, skipping words the user already has.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Tag = strings.TrimSpace(req.Tag)
	if req.Tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	if len(req.ApplyTags) > 20 {
		writeError(w, http.StatusBadRequest, "too many apply_tags (max 20)")
		return
	}
	var cleanTags []string
	for _, tg := range req.ApplyTags {
		tg = strings.TrimSpace(tg)
		if tg == "" {
			continue
		}
		if utf8.RuneCountInString(tg) > 50 {
			writeError(w, http.StatusBadRequest, "tag too long (max 50 chars)")
			return
		}
		cleanTags = append(cleanTags, tg)
	}
	if cleanTags == nil {
		cleanTags = []string{}
	}

	currentUserID := UserIDFromContext(r.Context())

	// Fetch all source words for the given tag.
	sourceWords, _, err := h.Store.GetWords(r.Context(), sourceUserID, "", 1, 0, "", "", []string{req.Tag}, false, false, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load source words")
		return
	}

	// Build a set of the current user's existing zh_texts.
	existingWords, _, err := h.Store.GetWords(r.Context(), currentUserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load existing words")
		return
	}
	existingZhTexts := make(map[string]struct{}, len(existingWords))
	for _, ew := range existingWords {
		existingZhTexts[ew.ZhText] = struct{}{}
	}

	imported := 0
	skipped := 0

	for _, sw := range sourceWords {
		if _, exists := existingZhTexts[sw.ZhText]; exists {
			skipped++
			continue
		}
		// Words without EN translations cannot be created (quiz requires EN).
		if len(sw.EnTexts) == 0 {
			skipped++
			continue
		}

		pinyin := ""
		if sw.Pinyin != nil {
			pinyin = *sw.Pinyin
		}

		deTexts := []string{}
		if req.ImportDe {
			deTexts = sw.DeTexts
		}

		createReq := models.CreateWordRequest{
			ZhText:  sw.ZhText,
			Pinyin:  pinyin,
			EnTexts: sw.EnTexts,
			DeTexts: deTexts,
			Tags:    cleanTags,
		}

		if _, err := h.Store.CreateWord(r.Context(), currentUserID, createReq); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create word")
			return
		}
		imported++
	}

	writeJSON(w, http.StatusOK, importResponse{Imported: imported, Skipped: skipped})
}
