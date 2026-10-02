package db

import (
	"context"
	"reflect"
	"testing"
	"vocabulary_trainer/models"
)

func TestImportJob_CreateAndGetAreScopedToUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	job, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1", ImportLangs: []string{"en", "de"}, ApplyTags: []string{"mine"}, AndTags: []string{"Food"}, ImportMode: "review"})
	if err != nil {
		t.Fatalf("CreateImportJob: %v", err)
	}
	if job.Status != "queued" || job.Tag != "HSK1" {
		t.Errorf("job = %+v, want queued HSK1", job)
	}

	got, err := s.GetImportJob(ctx, 2, job.ID)
	if err != nil || got == nil {
		t.Fatalf("GetImportJob: %v, %v", got, err)
	}
	if !reflect.DeepEqual(got.ImportLangs, []string{"en", "de"}) || !reflect.DeepEqual(got.ApplyTags, []string{"mine"}) {
		t.Errorf("langs/tags = %v / %v", got.ImportLangs, got.ApplyTags)
	}
	if !reflect.DeepEqual(got.AndTags, []string{"Food"}) || got.ImportMode != "review" {
		t.Errorf("and_tags/import_mode = %v / %q", got.AndTags, got.ImportMode)
	}

	other, err := s.GetImportJob(ctx, 3, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob other user: %v", err)
	}
	if other != nil {
		t.Errorf("another user must not see the job, got %+v", other)
	}
}

func TestImportJob_CreateReturnsActiveJobForSameUserAndTag(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	first, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Errorf("second create returned job %d, want the active job %d", again.ID, first.ID)
	}
	narrowed, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1", AndTags: []string{"Food"}})
	if err != nil {
		t.Fatal(err)
	}
	if narrowed.ID == first.ID {
		t.Errorf("the same tag narrowed by another tag must get its own job")
	}
	otherTag, err := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK2"})
	if err != nil {
		t.Fatal(err)
	}
	if otherTag.ID == first.ID {
		t.Errorf("a different tag must get its own job")
	}
	otherUser, err := s.CreateImportJob(ctx, 1, models.ImportJob{Tag: "HSK1"})
	if err != nil {
		t.Fatal(err)
	}
	if otherUser.ID == first.ID {
		t.Errorf("a different user must get their own job")
	}
}

func TestImportJob_LifecycleAndListings(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	a, _ := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1"})
	b, _ := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK2"})
	c, _ := s.CreateImportJob(ctx, 1, models.ImportJob{Tag: "HSK3"})

	if err := s.StartImportJob(ctx, a.ID); err != nil {
		t.Fatalf("StartImportJob: %v", err)
	}
	if err := s.UpdateImportJobProgress(ctx, a.ID, models.ImportJob{Total: 10, Done: 4, Imported: 3, Tagged: 1}); err != nil {
		t.Fatalf("UpdateImportJobProgress: %v", err)
	}
	got, _ := s.GetImportJob(ctx, 2, a.ID)
	if got.Status != "running" || got.Total != 10 || got.Done != 4 || got.Imported != 3 || got.Tagged != 1 {
		t.Errorf("running job = %+v", got)
	}

	ids := func(jobs []models.ImportJob) []int64 {
		var out []int64
		for _, j := range jobs {
			out = append(out, j.ID)
		}
		return out
	}
	runnable, err := s.ListRunnableImportJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(runnable), []int64{a.ID, b.ID, c.ID}) {
		t.Errorf("runnable = %v, want all three in id order", ids(runnable))
	}

	if err := s.FinishImportJob(ctx, a.ID, ""); err != nil {
		t.Fatalf("FinishImportJob: %v", err)
	}
	if err := s.FinishImportJob(ctx, b.ID, "boom"); err != nil {
		t.Fatalf("FinishImportJob failed: %v", err)
	}
	done, _ := s.GetImportJob(ctx, 2, a.ID)
	failed, _ := s.GetImportJob(ctx, 2, b.ID)
	if done.Status != "done" || failed.Status != "failed" || failed.Error != "boom" {
		t.Errorf("finished jobs = %+v / %+v", done, failed)
	}

	runnable, _ = s.ListRunnableImportJobs(ctx)
	if !reflect.DeepEqual(ids(runnable), []int64{c.ID}) {
		t.Errorf("runnable after finish = %v, want only the queued job", ids(runnable))
	}
	active, err := s.ListActiveImportJobs(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Errorf("user 2 active = %v, want none", ids(active))
	}
	active, _ = s.ListActiveImportJobs(ctx, 1)
	if !reflect.DeepEqual(ids(active), []int64{c.ID}) {
		t.Errorf("user 1 active = %v, want [%d]", ids(active), c.ID)
	}

	again, _ := s.CreateImportJob(ctx, 2, models.ImportJob{Tag: "HSK1"})
	if again.ID == a.ID {
		t.Errorf("a finished job must not be reused")
	}
}
