package db

import (
	"context"
	"testing"
	"time"
)

func TestGetAdminOverview_UserStats(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// Template DB seeds id=1 (admin, verified) and id=2 (plus, verified).
	// Add a third, unverified free user.
	if _, err := s.CreateUser(ctx, "extra@example.de", "hash", "tok", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create extra user: %v", err)
	}

	ov, err := s.GetAdminOverview(ctx)
	if err != nil {
		t.Fatalf("GetAdminOverview: %v", err)
	}

	if ov.Users.Total != 3 {
		t.Errorf("expected 3 total users, got %d", ov.Users.Total)
	}
	if ov.Users.Admins != 1 {
		t.Errorf("expected 1 admin, got %d", ov.Users.Admins)
	}
	if ov.Users.Plus != 1 {
		t.Errorf("expected 1 plus user, got %d", ov.Users.Plus)
	}
	if ov.Users.Free != 1 {
		t.Errorf("expected 1 free user, got %d", ov.Users.Free)
	}
	if ov.Users.Verified != 2 {
		t.Errorf("expected 2 verified users, got %d", ov.Users.Verified)
	}
	if ov.Users.Unverified != 1 {
		t.Errorf("expected 1 unverified user, got %d", ov.Users.Unverified)
	}
}

func TestGetAdminOverview_QuizVolumeAndActivity(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if _, err := s.RecordDailyStat(ctx, 2, true); err != nil {
		t.Fatalf("record daily stat: %v", err)
	}
	if _, err := s.RecordDailyStat(ctx, 2, false); err != nil {
		t.Fatalf("record daily stat: %v", err)
	}

	ov, err := s.GetAdminOverview(ctx)
	if err != nil {
		t.Fatalf("GetAdminOverview: %v", err)
	}

	if ov.Activity.ActiveLast7Days != 1 {
		t.Errorf("expected 1 active user in last 7 days, got %d", ov.Activity.ActiveLast7Days)
	}
	if ov.Activity.ActiveLast30Days != 1 {
		t.Errorf("expected 1 active user in last 30 days, got %d", ov.Activity.ActiveLast30Days)
	}
	if ov.Activity.Dormant != ov.Users.Total-1 {
		t.Errorf("expected dormant = total-1 (%d), got %d", ov.Users.Total-1, ov.Activity.Dormant)
	}

	if len(ov.QuizVolume) != 1 {
		t.Fatalf("expected 1 day of quiz volume, got %d", len(ov.QuizVolume))
	}
	if ov.QuizVolume[0].Attempts != 2 {
		t.Errorf("expected 2 attempts today, got %d", ov.QuizVolume[0].Attempts)
	}
	if ov.QuizVolume[0].Mistakes != 1 {
		t.Errorf("expected 1 mistake today, got %d", ov.QuizVolume[0].Mistakes)
	}
}

func TestGetAdminOverview_GuestActivityFromAuditLog(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordAuditLog(ctx, 0, AuditLoginFailure, "203.0.113.9", "nobody@example.com"); err != nil {
		t.Fatalf("record audit log: %v", err)
	}
	if err := s.RecordAuditLog(ctx, 0, AuditLoginFailure, "203.0.113.9", "nobody@example.com"); err != nil {
		t.Fatalf("record audit log: %v", err)
	}
	// A failed login against a known account (user_id > 0) should not count
	// as guest activity.
	if err := s.RecordAuditLog(ctx, 2, AuditLoginFailure, "203.0.113.9", ""); err != nil {
		t.Fatalf("record audit log: %v", err)
	}

	ov, err := s.GetAdminOverview(ctx)
	if err != nil {
		t.Fatalf("GetAdminOverview: %v", err)
	}

	if len(ov.GuestActivity) != 1 {
		t.Fatalf("expected 1 day of guest activity, got %d", len(ov.GuestActivity))
	}
	if ov.GuestActivity[0].Count != 2 {
		t.Errorf("expected 2 guest login failures today, got %d", ov.GuestActivity[0].Count)
	}
}

func TestGetAdminOverview_FeatureUsageSplitsPagesAndAPIs(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordUsageEvent(ctx, 2, "GET /train"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, 0, "GET /train"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, 2, "POST /api/translate"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, 2, "POST /api/words/{id}/hmm/generate-scene"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUsageEvent(ctx, 1, "POST /api/components/{char}/hmm/generate-scene"); err != nil {
		t.Fatal(err)
	}

	ov, err := s.GetAdminOverview(ctx)
	if err != nil {
		t.Fatalf("GetAdminOverview: %v", err)
	}

	found := false
	for _, pv := range ov.PageViews {
		if pv.Name == "GET /train" {
			found = true
			if pv.TotalCount != 2 {
				t.Errorf("expected GET /train total_count 2, got %d", pv.TotalCount)
			}
			if pv.UniqueUsers != 2 {
				t.Errorf("expected GET /train unique_users 2, got %d", pv.UniqueUsers)
			}
		}
		if pv.Name == "POST /api/translate" {
			t.Errorf("API route %q should not appear in page views", pv.Name)
		}
	}
	if !found {
		t.Error("expected GET /train in page views")
	}

	for _, fu := range ov.FeatureUsage {
		if fu.Name == "GET /train" {
			t.Errorf("page route %q should not appear in feature usage", fu.Name)
		}
	}

	if ov.DeepLUsage.TotalCalls != 1 || ov.DeepLUsage.UniqueUsers != 1 {
		t.Errorf("expected DeepL usage {1,1}, got %+v", ov.DeepLUsage)
	}
	if ov.LLMUsage.TotalCalls != 2 || ov.LLMUsage.UniqueUsers != 2 {
		t.Errorf("expected LLM usage {2,2}, got %+v", ov.LLMUsage)
	}
}
