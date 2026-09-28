package handlers

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sort"
	"vocabulary_trainer/models"
)

// importChunkSize is how many words one import transaction writes.
const importChunkSize = 200

// ImportWorker runs queued import jobs one after the other. The job state
// lives in the database, so RunPending at startup resumes a job that a
// restart interrupted: words already imported count as existing and are
// skipped (or tagged), so nothing is imported twice, but the finished job's
// imported/tagged/skipped counters then only describe the last run.
type ImportWorker struct {
	store importStore
	wake  chan struct{}
}

func NewImportWorker(store importStore) *ImportWorker {
	return &ImportWorker{store: store, wake: make(chan struct{}, 1)}
}

// Notify tells a running Run loop that a job was queued.
func (w *ImportWorker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run processes jobs until ctx ends: first the ones left over from an earlier
// run, then every time Notify is called.
func (w *ImportWorker) Run(ctx context.Context) {
	for {
		if err := w.RunPending(ctx); err != nil {
			log.Printf("import worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		}
	}
}

// RunPending processes every queued or running job once, oldest first, and
// marks each done or failed.
func (w *ImportWorker) RunPending(ctx context.Context) error {
	jobs, err := w.store.ListRunnableImportJobs(ctx)
	if err != nil {
		return fmt.Errorf("list import jobs: %w", err)
	}
	for _, job := range jobs {
		errMsg := ""
		if err := w.runJob(ctx, job); err != nil {
			log.Printf("import job %d (%s): %v", job.ID, job.Tag, err)
			errMsg = err.Error()
		}
		if err := w.store.FinishImportJob(ctx, job.ID, errMsg); err != nil {
			return fmt.Errorf("finish import job %d: %w", job.ID, err)
		}
	}
	return nil
}

func (w *ImportWorker) runJob(ctx context.Context, job models.ImportJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if err := w.store.StartImportJob(ctx, job.ID); err != nil {
		return err
	}

	sourceWords, _, err := w.store.GetWords(ctx, sourceUserID, "", 1, 0, "", "", []string{job.Tag}, false, false, "", "", "")
	if err != nil {
		return fmt.Errorf("load source words: %w", err)
	}
	if len(job.AndTags) > 0 {
		matching := sourceWords[:0]
		for _, sw := range sourceWords {
			if hasAllTags(sw.Tags, job.AndTags) {
				matching = append(matching, sw)
			}
		}
		sourceWords = matching
	}
	// Import in list order (oldest word first), so the words a learner meets
	// first are the ones at the start of the list.
	sort.Slice(sourceWords, func(i, j int) bool { return sourceWords[i].ID < sourceWords[j].ID })

	existingWords, _, err := w.store.GetWords(ctx, job.UserID, "", 1, 0, "", "", nil, false, false, "", "", "")
	if err != nil {
		return fmt.Errorf("load existing words: %w", err)
	}
	existingZhTexts := make(map[string]int64, len(existingWords))
	for _, ew := range existingWords {
		existingZhTexts[ew.ZhText] = ew.ID
	}

	p := models.ImportJob{Total: len(sourceWords)}
	importSet := map[string]bool{}
	for _, l := range job.ImportLangs {
		importSet[l] = true
	}

	var toImport []models.WordDetail
	for _, sw := range sourceWords {
		// A word the user already has is never imported twice; it only gets
		// the import's tags added, so it also shows up under the new list.
		existingID, exists := existingZhTexts[sw.ZhText]
		if !exists {
			toImport = append(toImport, sw)
			continue
		}
		p.Done++
		if len(job.ApplyTags) == 0 {
			p.Skipped++
			continue
		}
		if err := w.store.AddWordTags(ctx, job.UserID, existingID, job.ApplyTags); err != nil {
			return fmt.Errorf("tag existing word: %w", err)
		}
		p.Tagged++
	}
	if err := w.store.UpdateImportJobProgress(ctx, job.ID, p); err != nil {
		return err
	}

	for start := 0; start < len(toImport); start += importChunkSize {
		chunk := toImport[start:min(start+importChunkSize, len(toImport))]
		texts := make([]string, len(chunk))
		for i, sw := range chunk {
			texts[i] = sw.ZhText
		}
		dict, err := w.store.LookupDictionaryBatch(ctx, texts, dictLangs)
		if err != nil {
			return fmt.Errorf("load dictionary translations: %w", err)
		}

		reqs := make([]models.CreateWordRequest, 0, len(chunk))
		for _, sw := range chunk {
			translations := map[string][]string{}
			sources := map[string][]string{}
			for lang, defs := range dict[sw.ZhText] {
				if len(job.ImportLangs) == 0 || importSet[lang] {
					translations[lang] = defs
					// The translations come verbatim from the dictionary.
					sources[lang] = slices.Repeat([]string{"cedict"}, len(defs))
				}
			}
			if len(translations) == 0 {
				p.Skipped++
				continue
			}
			pinyin := ""
			if sw.Pinyin != nil {
				pinyin = *sw.Pinyin
			}
			reqs = append(reqs, models.CreateWordRequest{
				ZhText:             sw.ZhText,
				Pinyin:             pinyin,
				Translations:       translations,
				TranslationSources: sources,
				Tags:               job.ApplyTags,
			})
		}
		ids, err := w.store.CreateWordsBatch(ctx, job.UserID, reqs)
		if err != nil {
			return fmt.Errorf("create words: %w", err)
		}
		for _, id := range ids {
			switch job.ImportMode {
			case "known":
				err = w.store.SetWordKnown(ctx, job.UserID, id, true)
			case "review":
				err = w.store.AcknowledgeWord(ctx, job.UserID, id)
			}
			if err != nil {
				return fmt.Errorf("apply import mode: %w", err)
			}
		}
		p.Imported += len(reqs)
		p.Done += len(chunk)
		if err := w.store.UpdateImportJobProgress(ctx, job.ID, p); err != nil {
			return err
		}
	}
	return nil
}
