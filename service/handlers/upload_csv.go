package handlers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"vocabulary_trainer/models"
)

// Default DoS bounds for the CSV upload path; overridable via main.go env config.
const (
	defaultCSVMaxBytes = 8 << 20 // 8 MB request-body cap
	defaultCSVMaxRows  = 5000    // max data rows per upload
	ttsWorkers         = 4       // bound on concurrent background TTS jobs
)

type UploadCSVHandler struct {
	Store uploadCSVStore
	Audio *AudioHandler
	// MaxBytes / MaxRows bound the upload; zero means use the package defaults.
	MaxBytes int64
	MaxRows  int
}

func (h *UploadCSVHandler) maxBytes() int64 {
	if h.MaxBytes > 0 {
		return h.MaxBytes
	}
	return defaultCSVMaxBytes
}

func (h *UploadCSVHandler) maxRows() int {
	if h.MaxRows > 0 {
		return h.MaxRows
	}
	return defaultCSVMaxRows
}

func (h *UploadCSVHandler) UploadCSV(w http.ResponseWriter, r *http.Request) {
	// Cap the request body so an oversized upload can't exhaust memory/disk.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes())
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
			return
		}
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

	// Read all data rows up front, enforcing the row cap before importing
	// anything so an over-cap upload is rejected atomically (no partial import).
	var rows [][]string
	for {
		row, err := reader.Read()
		if err != nil {
			break // io.EOF or parse error — stop reading
		}
		rows = append(rows, row)
		if len(rows) > h.maxRows() {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("too many rows (max %d)", h.maxRows()))
			return
		}
	}

	var importedIDs []int64
	var updatedIDs []int64
	// ttsTask records a deferred TTS (re)generation to run in a bounded pool.
	type ttsTask struct {
		id    int64
		text  string
		regen bool
	}
	var ttsTasks []ttsTask
	skipped := 0

	for _, row := range rows {
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
				ttsTasks = append(ttsTasks, ttsTask{id: id, text: zhText})
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
				ttsTasks = append(ttsTasks, ttsTask{id: id, text: zhText, regen: true})
			}
			updatedIDs = append(updatedIDs, id)
		}
	}

	// Run the deferred TTS jobs in a background worker pool with bounded
	// concurrency so a large import can't spawn thousands of TTS goroutines.
	if h.Audio != nil && len(ttsTasks) > 0 {
		go func(tasks []ttsTask) {
			sem := make(chan struct{}, ttsWorkers)
			var wg sync.WaitGroup
			for _, t := range tasks {
				sem <- struct{}{}
				wg.Add(1)
				go func(t ttsTask) {
					defer wg.Done()
					defer func() { <-sem }()
					if t.regen {
						h.Audio.RegenerateAsync(t.id, t.text)
					} else {
						h.Audio.GenerateAsync(t.id, t.text)
					}
				}(t)
			}
			wg.Wait()
		}(ttsTasks)
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
