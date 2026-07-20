package db

import (
	"context"
	"testing"
)

func TestRecordUsageEvent_InsertsNewRow(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordUsageEvent(ctx, int64(2), "POST /api/quiz/answer"); err != nil {
		t.Fatal(err)
	}

	var count int
	var lastSeen string
	if err := s.db.QueryRowContext(ctx,
		`SELECT count, last_seen FROM usage_events WHERE user_id = ? AND name = ?`,
		int64(2), "POST /api/quiz/answer").Scan(&count, &lastSeen); err != nil {
		t.Fatalf("query usage_events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
	if lastSeen == "" {
		t.Error("expected last_seen to be set")
	}
}

func TestRecordUsageEvent_IncrementsOnRepeat(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordUsageEvent(ctx, int64(2), "GET /train"); err != nil {
		t.Fatal(err)
	}
	firstSeen := usageLastSeen(t, s, int64(2), "GET /train")

	if err := s.RecordUsageEvent(ctx, int64(2), "GET /train"); err != nil {
		t.Fatal(err)
	}

	var count int
	var lastSeen string
	if err := s.db.QueryRowContext(ctx,
		`SELECT count, last_seen FROM usage_events WHERE user_id = ? AND name = ?`,
		int64(2), "GET /train").Scan(&count, &lastSeen); err != nil {
		t.Fatalf("query usage_events: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2 after second hit, got %d", count)
	}
	if lastSeen == "" {
		t.Error("expected last_seen to remain set")
	}
	_ = firstSeen
}

func TestRecordUsageEvent_SeparatesUsersAndNames(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordUsageEvent(ctx, int64(2), "GET /vocab"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, int64(3), "GET /vocab"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, int64(2), "GET /stats"); err != nil {
		t.Fatal(err)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_events`).Scan(&total); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 distinct rows, got %d", total)
	}
}

func TestRecordUsageEvent_AnonymousUsesZeroUserID(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordUsageEvent(ctx, 0, "GET /login"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, 0, "GET /login"); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count FROM usage_events WHERE user_id = 0 AND name = ?`, "GET /login").Scan(&count); err != nil {
		t.Fatalf("query usage_events: %v", err)
	}
	if count != 2 {
		t.Errorf("expected anonymous hits to aggregate into one row with count 2, got %d", count)
	}
}

func usageLastSeen(t *testing.T, s *Store, userID int64, name string) string {
	t.Helper()
	var lastSeen string
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT last_seen FROM usage_events WHERE user_id = ? AND name = ?`, userID, name).Scan(&lastSeen); err != nil {
		t.Fatalf("query last_seen: %v", err)
	}
	return lastSeen
}
