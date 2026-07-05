package handlers

import (
	"encoding/csv"
	"net/http"
	"strings"
	"vocabulary_trainer/models"
)

// CSVImportHandler handles importing words from a simple CSV file upload
// inside the Import tab. The format supports zh, pinyin (optional), en, de columns.
// Pinyin is auto-generated from the zh text when omitted.
type CSVImportHandler struct {
	Store importStore
}

type csvImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

func (h *CSVImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid CSV: "+err.Error())
		return
	}
	if len(records) < 2 {
		writeError(w, http.StatusBadRequest, "CSV must have a header row and at least one data row")
		return
	}

	// Parse header to find column positions
	colIndex := map[string]int{}
	for i, col := range records[0] {
		colIndex[strings.ToLower(strings.TrimSpace(col))] = i
	}
	zhCol, hasZh := colIndex["zh"]
	if !hasZh {
		writeError(w, http.StatusBadRequest, "CSV must have a 'zh' column")
		return
	}
	pinyinCol, hasPinyin := colIndex["pinyin"]
	enCol, hasEn := colIndex["en"]
	deCol, hasDe := colIndex["de"]

	if !hasEn && !hasDe {
		writeError(w, http.StatusBadRequest, "CSV must have at least one translation column ('en' or 'de')")
		return
	}

	userID := UserIDFromContext(r.Context())
	result := csvImportResult{}

	for _, row := range records[1:] {
		if zhCol >= len(row) {
			continue
		}
		zhText := strings.TrimSpace(row[zhCol])
		if zhText == "" {
			continue
		}

		// Auto-generate pinyin when column is absent or cell is empty
		py := ""
		if hasPinyin && pinyinCol < len(row) {
			py = strings.TrimSpace(row[pinyinCol])
		}
		if py == "" {
			py = toPinyin(zhText)
		}

		translations := map[string][]string{}
		if hasEn && enCol < len(row) {
			if parts := splitCSVImportTranslations(strings.TrimSpace(row[enCol])); len(parts) > 0 {
				translations["en"] = parts
			}
		}
		if hasDe && deCol < len(row) {
			if parts := splitCSVImportTranslations(strings.TrimSpace(row[deCol])); len(parts) > 0 {
				translations["de"] = parts
			}
		}
		if len(translations) == 0 {
			result.Skipped++
			continue
		}

		exists, err := h.Store.IsZhWordForUser(r.Context(), userID, zhText)
		if err != nil {
			result.Errors = append(result.Errors, zhText+": "+err.Error())
			continue
		}
		if exists {
			result.Skipped++
			continue
		}

		createReq := models.CreateWordRequest{
			ZhText:       zhText,
			Pinyin:       py,
			Translations: translations,
		}
		if _, err := h.Store.CreateWord(r.Context(), userID, createReq); err != nil {
			result.Errors = append(result.Errors, zhText+": "+err.Error())
			continue
		}
		result.Imported++
	}

	writeJSON(w, http.StatusOK, result)
}

// splitCSVImportTranslations splits a translations cell on commas.
func splitCSVImportTranslations(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
