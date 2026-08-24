package db

import (
	"context"
	"testing"
)

func seedFunnelUser(t *testing.T, s *Store, email string, verified int) int64 {
	t.Helper()
	res, err := s.db.Exec(
		`INSERT INTO users (email, password_hash, email_verified) VALUES (?, 'x', ?)`,
		email, verified)
	if err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

func seedTrainingDay(t *testing.T, s *Store, userID int64, date string, attempts int) {
	t.Helper()
	if _, err := s.db.Exec(
		`INSERT INTO daily_stats (user_id, date, attempts) VALUES (?, ?, ?)`,
		userID, date, attempts); err != nil {
		t.Fatalf("insert daily_stats %d %s: %v", userID, date, err)
	}
}

func TestGetFunnelReport_CountsEachStage(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	base, err := s.GetFunnelReport(ctx, 20)
	if err != nil {
		t.Fatalf("baseline funnel report: %v", err)
	}

	// User A: verified, trained two consecutive days, 25 attempts total.
	a := seedFunnelUser(t, s, "a@example.com", 1)
	seedTrainingDay(t, s, a, "2026-03-01", 15)
	seedTrainingDay(t, s, a, "2026-03-02", 10)

	// User B: unverified, trained once with 3 attempts, never returned.
	b := seedFunnelUser(t, s, "b@example.com", 0)
	seedTrainingDay(t, s, b, "2026-03-01", 3)

	// User C: verified, registered but never trained.
	seedFunnelUser(t, s, "c@example.com", 1)

	// User D: verified, trained twice but with a gap (no day-1 return).
	d := seedFunnelUser(t, s, "d@example.com", 1)
	seedTrainingDay(t, s, d, "2026-03-01", 5)
	seedTrainingDay(t, s, d, "2026-03-05", 5)

	got, err := s.GetFunnelReport(ctx, 20)
	if err != nil {
		t.Fatalf("funnel report: %v", err)
	}

	if diff := got.Registered - base.Registered; diff != 4 {
		t.Errorf("Registered: expected +4, got +%d", diff)
	}
	if diff := got.Verified - base.Verified; diff != 3 {
		t.Errorf("Verified: expected +3, got +%d", diff)
	}
	if diff := got.Activated - base.Activated; diff != 3 {
		t.Errorf("Activated: expected +3, got +%d", diff)
	}
	if diff := got.Engaged - base.Engaged; diff != 1 {
		t.Errorf("Engaged: expected +1, got +%d", diff)
	}
	if diff := got.Returned - base.Returned; diff != 2 {
		t.Errorf("Returned: expected +2, got +%d", diff)
	}
	if diff := got.ReturnedDay1 - base.ReturnedDay1; diff != 1 {
		t.Errorf("ReturnedDay1: expected +1, got +%d", diff)
	}
}

func TestGetFunnelReport_ExcludesLibraryUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	base, err := s.GetFunnelReport(ctx, 20)
	if err != nil {
		t.Fatalf("baseline funnel report: %v", err)
	}

	// Training activity for the shared library user (id=1) must not count.
	seedTrainingDay(t, s, 1, "2026-03-01", 50)
	seedTrainingDay(t, s, 1, "2026-03-02", 50)

	got, err := s.GetFunnelReport(ctx, 20)
	if err != nil {
		t.Fatalf("funnel report: %v", err)
	}
	if got != base {
		t.Errorf("library user activity changed the funnel: base=%+v got=%+v", base, got)
	}
}

func TestGetFunnelReport_MinAttemptsThreshold(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	u := seedFunnelUser(t, s, "threshold@example.com", 1)
	seedTrainingDay(t, s, u, "2026-03-01", 7)

	base, err := s.GetFunnelReport(ctx, 100)
	if err != nil {
		t.Fatalf("funnel report (100): %v", err)
	}
	low, err := s.GetFunnelReport(ctx, 5)
	if err != nil {
		t.Fatalf("funnel report (5): %v", err)
	}
	if low.Engaged <= base.Engaged {
		t.Errorf("expected lower threshold to count more engaged users: high=%d low=%d",
			base.Engaged, low.Engaged)
	}
}
