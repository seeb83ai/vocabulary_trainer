package db

import (
	"context"
	"testing"
)

func TestRecordAuditLog_AppendsRow(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	if err := s.RecordAuditLog(ctx, 2, "login_success", "203.0.113.1", ""); err != nil {
		t.Fatalf("RecordAuditLog: %v", err)
	}
	if err := s.RecordAuditLog(ctx, 2, "password_change", "203.0.113.1", ""); err != nil {
		t.Fatalf("RecordAuditLog: %v", err)
	}

	rows, err := s.GetAuditLogForUser(ctx, 2, 10)
	if err != nil {
		t.Fatalf("GetAuditLogForUser: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// Most recent first.
	if rows[0].Action != "password_change" {
		t.Errorf("row[0].Action: want password_change, got %q", rows[0].Action)
	}
	if rows[1].Action != "login_success" {
		t.Errorf("row[1].Action: want login_success, got %q", rows[1].Action)
	}
	if rows[0].IPAddress != "203.0.113.1" {
		t.Errorf("ip_address: want 203.0.113.1, got %q", rows[0].IPAddress)
	}
	if rows[0].CreatedAt.IsZero() {
		t.Error("created_at should be populated")
	}
}

func TestRecordAuditLog_NoUserID_StillRecorded(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	// user_id = 0 → record without an associated account (used for failed
	// logins where the email did not match any user).
	if err := s.RecordAuditLog(ctx, 0, "login_failure", "203.0.113.2", "nobody@example.com"); err != nil {
		t.Fatalf("RecordAuditLog: %v", err)
	}

	rows, err := s.GetAuditLogByIP(ctx, "203.0.113.2", 10)
	if err != nil {
		t.Fatalf("GetAuditLogByIP: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Action != "login_failure" {
		t.Errorf("action: %q", rows[0].Action)
	}
	if rows[0].Detail != "nobody@example.com" {
		t.Errorf("detail: %q", rows[0].Detail)
	}
}

func TestGetAuditLogForUser_RespectsLimit(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.RecordAuditLog(ctx, 2, "login_success", "203.0.113.1", ""); err != nil {
			t.Fatalf("RecordAuditLog: %v", err)
		}
	}

	rows, err := s.GetAuditLogForUser(ctx, 2, 3)
	if err != nil {
		t.Fatalf("GetAuditLogForUser: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("limit=3: want 3 rows, got %d", len(rows))
	}
}
