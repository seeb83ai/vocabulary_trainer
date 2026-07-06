package db

import (
	"context"
	"fmt"
	"time"
)

// AuditEntry is one row in the audit_log table.
type AuditEntry struct {
	ID        int64
	UserID    int64
	Action    string
	IPAddress string
	Detail    string
	CreatedAt time.Time
}

// Standard audit action names. Use these constants from handlers so the
// log stays normalised (the table also accepts free-form strings).
const (
	AuditLogin             = "login_success"
	AuditLoginFailure      = "login_failure"
	AuditLogout            = "logout"
	AuditRegister          = "register"
	AuditRegisterDuplicate = "register_duplicate"
	AuditPasswordChange    = "password_change"
	AuditEmailVerified     = "email_verified"
	AuditAccountLocked     = "account_locked"
	AuditAPIKeyUpdate      = "api_key_update"
	AuditGitHubIssue       = "github_issue_created"
)

// RecordAuditLog appends a row to audit_log. userID may be 0 when the
// event is not yet tied to an account (e.g. login attempts for unknown
// emails). detail is a free-form short string — keep secrets out of it.
func (s *Store) RecordAuditLog(ctx context.Context, userID int64, action, ipAddress, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, action, ip_address, detail) VALUES (?, ?, ?, ?)`,
		userID, action, ipAddress, detail)
	if err != nil {
		return fmt.Errorf("record audit log: %w", err)
	}
	return nil
}

// GetAuditLogForUser returns the most recent N entries for a user,
// newest first.
func (s *Store) GetAuditLogForUser(ctx context.Context, userID int64, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, action, ip_address, detail, created_at
		 FROM audit_log
		 WHERE user_id = ?
		 ORDER BY id DESC
		 LIMIT ?`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get audit log for user: %w", err)
	}
	defer rows.Close()
	return scanAuditEntries(rows)
}

// GetAuditLogByIP returns the most recent N entries for an IP, newest
// first. Useful for monitoring brute-force attempts that span multiple
// usernames.
func (s *Store) GetAuditLogByIP(ctx context.Context, ip string, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, action, ip_address, detail, created_at
		 FROM audit_log
		 WHERE ip_address = ?
		 ORDER BY id DESC
		 LIMIT ?`, ip, limit)
	if err != nil {
		return nil, fmt.Errorf("get audit log by ip: %w", err)
	}
	defer rows.Close()
	return scanAuditEntries(rows)
}

func scanAuditEntries(rows sqlRows) ([]AuditEntry, error) {
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.IPAddress, &e.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		e.CreatedAt = parseDateTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// sqlRows is the minimal subset of *sql.Rows that scanAuditEntries needs.
// Declared here to keep the function easy to test without a real DB.
type sqlRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}
