package handlers

import (
	"context"
	"fmt"
	"log"
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
	removed := map[int64]bool{}
	if !job.IncludeRemoved {
		if removed, err = w.store.TombstonedLibraryWords(ctx, job.UserID); err != nil {
			return fmt.Errorf("load deleted words: %w", err)
		}
	}

	var toImport []models.WordDetail
	for _, sw := range sourceWords {
		// A word the user already has is never imported twice; it only gets
		// the import's tags added, so it also shows up under the new list.
		existingID, exists := existingZhTexts[sw.ZhText]
		if !exists && removed[sw.ID] {
			// The user deleted this word earlier: skip it unless asked.
			p.Done++
			p.Skipped++
			continue
		}
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
		libraryIDs := make([]int64, len(chunk))
		for i, sw := range chunk {
			libraryIDs[i] = sw.ID
		}
		withGlosses, err := w.store.LibraryWordsWithGlosses(ctx, job.UserID, libraryIDs)
		if err != nil {
			return fmt.Errorf("check library glosses: %w", err)
		}
		// The learner gets references to the library words (ADR-0005); a
		// word with no gloss in the learner's languages is skipped.
		refIDs := libraryIDs[:0:0]
		for _, id := range libraryIDs {
			if withGlosses[id] {
				refIDs = append(refIDs, id)
			} else {
				p.Skipped++
			}
		}
		ids, err := w.store.CreateReferences(ctx, job.UserID, refIDs, job.ApplyTags)
		if err != nil {
			return fmt.Errorf("create references: %w", err)
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
		p.Imported += len(ids)
		p.Done += len(chunk)
		if err := w.store.UpdateImportJobProgress(ctx, job.ID, p); err != nil {
			return err
		}
	}
	return nil
}
