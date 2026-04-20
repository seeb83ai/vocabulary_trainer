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

type importPreviewWord struct {
	ZhText       string              `json:"zh_text"`
	Pinyin       string              `json:"pinyin"`
	Translations map[string][]string `json:"translations"`
}

type importPreviewResponse struct {
	Tag            string              `json:"tag"`
	Total          int                 `json:"total"`
	AvailableLangs map[string]int      `json:"available_langs"`
	Examples       []importPreviewWord `json:"examples"`
}

type importRequest struct {
	Tag         string   `json:"tag"`
	ImportLangs []string `json:"import_langs"`
	ApplyTags   []string `json:"apply_tags"`
}

type importResponse struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

const sourceUserID int64 = 1

// SourceTags returns importable tags belonging to the shared library user (user_id=1),
// including each tag's description.
func (h *ImportHandler) SourceTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.Store.GetImportableSourceTags(r.Context(), sourceUserID)
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

	availableLangs := map[string]int{}
	examples := make([]importPreviewWord, 0, 50)
	for _, w := range words {
		for lang, texts := range w.Translations {
			if len(texts) > 0 {
				availableLangs[lang]++
			}
		}
		if len(examples) < 50 {
			pinyin := ""
			if w.Pinyin != nil {
				pinyin = *w.Pinyin
			}
			preview := map[string][]string{}
			for lang, texts := range w.Translations {
				if len(texts) > 3 {
					texts = texts[:3]
				}
				preview[lang] = texts
			}
			examples = append(examples, importPreviewWord{
				ZhText:       w.ZhText,
				Pinyin:       pinyin,
				Translations: preview,
			})
		}
	}

	writeJSON(w, http.StatusOK, importPreviewResponse{Tag: tag, Total: total, AvailableLangs: availableLangs, Examples: examples})
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
		pinyin := ""
		if sw.Pinyin != nil {
			pinyin = *sw.Pinyin
		}

		importSet := map[string]bool{}
		for _, l := range req.ImportLangs {
			importSet[l] = true
		}
		translations := map[string][]string{}
		for lang, texts := range sw.Translations {
			if len(req.ImportLangs) == 0 || importSet[lang] {
				translations[lang] = texts
			}
		}
		if len(translations) == 0 {
			skipped++
			continue
		}

		createReq := models.CreateWordRequest{
			ZhText:       sw.ZhText,
			Pinyin:       pinyin,
			Translations: translations,
			Tags:         cleanTags,
		}

		if _, err := h.Store.CreateWord(r.Context(), currentUserID, createReq); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create word")
			return
		}
		imported++
	}

	writeJSON(w, http.StatusOK, importResponse{Imported: imported, Skipped: skipped})
}
