package handlers

import (
	"encoding/csv"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
	"vocabulary_trainer/db"
	"vocabulary_trainer/models"
)

type UploadCSVHandler struct {
	Store *db.Store
	Audio *AudioHandler
}

func (h *UploadCSVHandler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	// Parse and validate tags (required).
	rawTags := strings.TrimSpace(r.FormValue("tags"))
	if rawTags == "" {
		writeError(w, http.StatusBadRequest, "tags is required")
		return
	}
	var tags []string
	for _, t := range strings.Split(rawTags, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 50 {
			writeError(w, http.StatusBadRequest, "tag too long (max 50 characters)")
			return
		}
		tags = append(tags, t)
	}
	if len(tags) == 0 {
		writeError(w, http.StatusBadRequest, "tags is required")
		return
	}
	if len(tags) > 20 {
		writeError(w, http.StatusBadRequest, "too many tags (max 20)")
		return
	}

	startTrainingCount := 0
	if v := r.FormValue("start_training_count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid start_training_count")
			return
		}
		startTrainingCount = n
	}

	f, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read CSV header")
		return
	}
	if len(header) < 3 {
		writeError(w, http.StatusBadRequest, "CSV must have at least 3 columns: chinese, pinyin, <lang>")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(header[0]), "chinese") {
		writeError(w, http.StatusBadRequest, "first CSV column must be 'chinese'")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(header[1]), "pinyin") {
		writeError(w, http.StatusBadRequest, "second CSV column must be 'pinyin'")
		return
	}
	langCols := make([]string, len(header)-2)
	for i, col := range header[2:] {
		langCols[i] = strings.ToLower(strings.TrimSpace(col))
	}

	userID := UserIDFromContext(r.Context())
	ctx := r.Context()

	var importedIDs []int64
	var updatedIDs []int64
	skipped := 0

	for {
		row, err := reader.Read()
		if err != nil {
			break // io.EOF or parse error — stop processing
		}
		if len(row) < 3 {
			skipped++
			continue
		}
		zhText := strings.TrimSpace(row[0])
		pinyin := strings.TrimSpace(row[1])
		if zhText == "" {
			skipped++
			continue
		}

		translations := map[string][]string{}
		for i, lang := range langCols {
			colIdx := i + 2
			if colIdx >= len(row) {
				continue
			}
			cell := row[colIdx]
			for _, seg := range strings.Split(cell, ";") {
				seg = strings.TrimSpace(seg)
				if seg != "" {
					translations[lang] = append(translations[lang], seg)
				}
			}
		}
		totalTr := 0
		for _, v := range translations {
			totalTr += len(v)
		}
		if totalTr == 0 {
			skipped++
			continue
		}

		req := models.CreateWordRequest{
			ZhText:       zhText,
			Pinyin:       pinyin,
			Translations: translations,
			Tags:         tags,
		}

		exists, err := h.Store.IsZhWordForUser(ctx, userID, zhText)
		if err != nil {
			log.Printf("upload-csv IsZhWordForUser %q: %v", zhText, err)
			skipped++
			continue
		}

		if !exists {
			id, err := h.Store.CreateWord(ctx, userID, req)
			if err != nil {
				log.Printf("upload-csv CreateWord %q: %v", zhText, err)
				skipped++
				continue
			}
			if h.Audio != nil {
				go h.Audio.generate(id, zhText)
			}
			importedIDs = append(importedIDs, id)
		} else {
			id, err := h.Store.GetWordIDByZhText(ctx, userID, zhText)
			if err != nil || id == 0 {
				log.Printf("upload-csv GetWordIDByZhText %q: %v", zhText, err)
				skipped++
				continue
			}
			updateReq := models.UpdateWordRequest{
				ZhText:       req.ZhText,
				Pinyin:       req.Pinyin,
				Translations: req.Translations,
				Tags:         req.Tags,
			}
			if err := h.Store.UpdateWord(ctx, userID, id, updateReq); err != nil {
				log.Printf("upload-csv UpdateWord %q: %v", zhText, err)
				skipped++
				continue
			}
			if h.Audio != nil {
				go h.Audio.regenerate(id, zhText)
			}
			updatedIDs = append(updatedIDs, id)
		}
	}

	// Apply start_training to a random subset of all processed words.
	allIDs := append(importedIDs, updatedIDs...)
	if startTrainingCount > len(allIDs) {
		startTrainingCount = len(allIDs)
	}
	if startTrainingCount > 0 {
		rand.Shuffle(len(allIDs), func(i, j int) { allIDs[i], allIDs[j] = allIDs[j], allIDs[i] })
		for _, id := range allIDs[:startTrainingCount] {
			if err := h.Store.AcknowledgeWord(ctx, userID, id); err != nil {
				log.Printf("upload-csv AcknowledgeWord %d: %v", id, err)
				continue
			}
			wd, err := h.Store.GetWordByID(ctx, userID, id)
			if err == nil && wd != nil {
				initComponents(ctx, h.Store, userID, id, wd.ZhText)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{
		"imported": len(importedIDs),
		"updated":  len(updatedIDs),
		"total":    len(importedIDs) + len(updatedIDs),
		"skipped":  skipped,
	})
}
