package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
	"vocabulary_trainer/models"
)

// ImportHandler handles cross-user word import from the shared library user (user_id=1).
type ImportHandler struct {
	Store  importStore
	Worker *ImportWorker
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
	// AndTags narrows the import to words that also carry every one of
	// these tags (Tag AND AndTags), e.g. HSK 1 + Food.
	AndTags []string `json:"and_tags"`
	// ImportMode says how new words start: "include" (default, unseen),
	// "review" (skip the intro, due once) or "known" (never quizzed).
	ImportMode string `json:"import_mode"`
}

const sourceUserID int64 = 1

// dictLangs are the languages the tag-based import feature can pull
// translations for. User_id=1 stores only zh words and tags; translations
// come live from cedict_entries (CC-CEDICT for en, HanDeDict for de).
var dictLangs = []string{"en", "de"}

// dictionaryTranslations looks up every cedict_entries definition for
// zhText across dictLangs, keyed by language.
func dictionaryTranslations(ctx context.Context, store importStore, zhText string) (map[string][]string, error) {
	translations := map[string][]string{}
	for _, lang := range dictLangs {
		defs, err := store.LookupDictionary(ctx, zhText, lang)
		if err != nil {
			return nil, err
		}
		if len(defs) > 0 {
			translations[lang] = defs
		}
	}
	return translations, nil
}

// hasAllTags reports whether tags contains every name in want.
func hasAllTags(tags, want []string) bool {
	for _, w := range want {
		found := false
		for _, tg := range tags {
			if tg == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

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

// Preview returns a brief summary of words that would be imported for one or
// more tags (?tag=a&tag=b). A word in several of the tags counts once.
func (h *ImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var tags []string
	for _, tg := range r.URL.Query()["tag"] {
		if tg = strings.TrimSpace(tg); tg != "" {
			tags = append(tags, tg)
		}
	}
	if len(tags) == 0 {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}
	tag := strings.Join(tags, ",")

	words, total, err := h.Store.GetWords(r.Context(), sourceUserID, "", 1, 0, "", "", tags, false, false, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load preview")
		return
	}
	if r.URL.Query().Get("match") == "all" {
		matching := words[:0]
		for _, word := range words {
			if hasAllTags(word.Tags, tags) {
				matching = append(matching, word)
			}
		}
		words, total = matching, len(matching)
	}

	availableLangs := map[string]int{}
	examples := make([]importPreviewWord, 0, 50)
	for _, word := range words {
		translations, err := dictionaryTranslations(r.Context(), h.Store, word.ZhText)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load dictionary translations")
			return
		}
		for lang, texts := range translations {
			if len(texts) > 0 {
				availableLangs[lang]++
			}
		}
		if len(examples) < 50 {
			pinyin := ""
			if word.Pinyin != nil {
				pinyin = *word.Pinyin
			}
			preview := map[string][]string{}
			for lang, texts := range translations {
				if len(texts) > 3 {
					texts = texts[:3]
				}
				preview[lang] = texts
			}
			examples = append(examples, importPreviewWord{
				ZhText:       word.ZhText,
				Pinyin:       pinyin,
				Translations: preview,
			})
		}
	}

	writeJSON(w, http.StatusOK, importPreviewResponse{Tag: tag, Total: total, AvailableLangs: availableLangs, Examples: examples})
}

// Import queues a background job that copies all words of the source user's
// tag to the requesting user, skipping words the user already has (see
// ImportWorker). It returns the queued job with 202.
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

	switch req.ImportMode {
	case "", "include", "review", "known":
	default:
		writeError(w, http.StatusBadRequest, "import_mode must be include, review or known")
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

	var andTags []string
	for _, tg := range req.AndTags {
		if tg = strings.TrimSpace(tg); tg != "" {
			andTags = append(andTags, tg)
		}
	}
	if req.ImportMode == "" {
		req.ImportMode = "include"
	}

	job, err := h.Store.CreateImportJob(r.Context(), UserIDFromContext(r.Context()), models.ImportJob{
		Tag:         req.Tag,
		ImportLangs: req.ImportLangs,
		ApplyTags:   cleanTags,
		AndTags:     andTags,
		ImportMode:  req.ImportMode,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue import")
		return
	}
	if h.Worker != nil {
		h.Worker.Notify()
	}
	writeJSON(w, http.StatusAccepted, job)
}

// Job returns the status and progress of one of the user's import jobs.
func (h *ImportHandler) Job(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := h.Store.GetImportJob(r.Context(), UserIDFromContext(r.Context()), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load import job")
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "import job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// ActiveJobs returns the user's queued and running import jobs, so the page
// can show the progress of an import that started before it was opened.
func (h *ImportHandler) ActiveJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.Store.ListActiveImportJobs(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load import jobs")
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}
