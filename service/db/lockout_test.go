package db

import (
	"context"
	"testing"
	"time"
)

func TestLockout_IncrementFailedLogins(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	n, err := s.IncrementFailedLogins(ctx, 2)
	if err != nil {
		t.Fatalf("IncrementFailedLogins: %v", err)
	}
	if n != 1 {
		t.Errorf("after 1 increment: want 1, got %d", n)
	}

	for i := 0; i < 3; i++ {
		if _, err := s.IncrementFailedLogins(ctx, 2); err != nil {
			t.Fatalf("IncrementFailedLogins (loop %d): %v", i, err)
		}
	}

	count, locked, err := s.GetFailedLogins(ctx, 2)
	if err != nil {
		t.Fatalf("GetFailedLogins: %v", err)
	}
	if count != 4 {
		t.Errorf("count: want 4, got %d", count)
	}
	if !locked.IsZero() {
		t.Errorf("lockout_until should be zero before LockAccount, got %v", locked)
	}
}

func TestLockout_LockAndCheck(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	until := time.Now().UTC().Add(15 * time.Minute)
	if err := s.LockAccountUntil(ctx, 2, until); err != nil {
		t.Fatalf("LockAccountUntil: %v", err)
	}

	locked, lockedUntil, err := s.IsAccountLocked(ctx, 2)
	if err != nil {
		t.Fatalf("IsAccountLocked: %v", err)
	}
	if !locked {
		t.Error("account should be locked")
	}
	if lockedUntil.Before(time.Now().UTC()) {
		t.Errorf("lockedUntil should be in the future, got %v", lockedUntil)
	}
}

func TestLockout_ExpiredLockNotEnforced(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	past := time.Now().UTC().Add(-1 * time.Hour)
	if err := s.LockAccountUntil(ctx, 2, past); err != nil {
		t.Fatalf("LockAccountUntil: %v", err)
	}

	locked, _, err := s.IsAccountLocked(ctx, 2)
	if err != nil {
		t.Fatalf("IsAccountLocked: %v", err)
	}
	if locked {
		t.Error("expired lockout should not be enforced")
	}
}

func TestLockout_ResetClearsCounter(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, _ = s.IncrementFailedLogins(ctx, 2)
	}
	if err := s.ResetFailedLogins(ctx, 2); err != nil {
		t.Fatalf("ResetFailedLogins: %v", err)
	}
	count, locked, err := s.GetFailedLogins(ctx, 2)
	if err != nil {
		t.Fatalf("GetFailedLogins: %v", err)
	}
	if count != 0 {
		t.Errorf("count after reset: want 0, got %d", count)
	}
	if !locked.IsZero() {
		t.Errorf("lockout_until after reset: want zero, got %v", locked)
	}
}
