package handlers

import (
	"context"
	"time"
	"vocabulary_trainer/models"

	"testing"
)

// blockingComponentIniter is a componentIniter whose InitComponentsForWord
// blocks until the test unblocks it, so tests can prove initComponentsAsync
// returns to its caller without waiting for the store work to finish.
type blockingComponentIniter struct {
	unblock chan struct{}
	called  chan struct{}
}

func (f *blockingComponentIniter) GetSM2Progress(ctx context.Context, wordID int64) (*models.SM2Progress, error) {
	return &models.SM2Progress{WordID: wordID, DueDate: time.Now()}, nil
}

func (f *blockingComponentIniter) InitComponentsForWord(ctx context.Context, userID int64, zhText string, dueDate time.Time) error {
	<-f.unblock
	close(f.called)
	return nil
}

func (f *blockingComponentIniter) CreateSubwordsForWord(ctx context.Context, userID, zhWordID int64, zhText string) error {
	return nil
}

func (f *blockingComponentIniter) GetUserSettings(ctx context.Context, userID int64) (*models.UserSettings, error) {
	return nil, nil
}

// TestInitComponentsAsync_ReturnsBeforeStoreWorkCompletes locks in the fix for
// slow "got it" clicks: Acknowledge used to run initComponents synchronously,
// so a slow component-coverage recompute or sub-word lookup directly delayed
// the HTTP response. initComponentsAsync must hand the work to a background
// goroutine and return immediately, regardless of how long the store calls take.
func TestInitComponentsAsync_ReturnsBeforeStoreWorkCompletes(t *testing.T) {
	fake := &blockingComponentIniter{unblock: make(chan struct{}), called: make(chan struct{})}

	returned := make(chan struct{})
	go func() {
		initComponentsAsync(fake, 2, 1, "妈妈")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("initComponentsAsync should return immediately without waiting for store work")
	}

	select {
	case <-fake.called:
		t.Fatal("store work must not have completed yet — initComponentsAsync returned before the fake was unblocked")
	default:
	}

	close(fake.unblock)
	WaitForComponentInit()

	select {
	case <-fake.called:
	default:
		t.Fatal("expected store work to have completed after WaitForComponentInit")
	}
}
