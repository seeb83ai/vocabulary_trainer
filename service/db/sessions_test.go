package db

import (
	"context"
	"testing"
	"time"
)

func newSessionTestUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	id, err := s.CreateUser(context.Background(), email, "hash", "tok-"+email, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSession_CreateThenLookup(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess1@example.com")
	now := time.Now().UTC()

	if err := s.CreateSession(ctx, uid, "token-a", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, expires, err := s.GetSessionUserID(ctx, "token-a", now)
	if err != nil {
		t.Fatal(err)
	}
	if got != uid {
		t.Errorf("user id: want %d, got %d", uid, got)
	}
	if d := expires.Sub(now.Add(time.Hour)); d > time.Second || d < -time.Second {
		t.Errorf("expires: want %v, got %v", now.Add(time.Hour), expires)
	}
}

func TestSession_UnknownTokenNotFound(t *testing.T) {
	s := openTestDB(t)
	got, _, err := s.GetSessionUserID(context.Background(), "no-such-token", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("want 0 for unknown token, got %d", got)
	}
}

func TestSession_ExpiredNotFound(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess2@example.com")
	now := time.Now().UTC()
	if err := s.CreateSession(ctx, uid, "token-old", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.GetSessionUserID(ctx, "token-old", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("want 0 for expired session, got %d", got)
	}
}

// The DB must never hold the raw token: a leaked database must not give
// an attacker live session cookies.
func TestSession_TokenNotStoredInPlaintext(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess3@example.com")
	if err := s.CreateSession(ctx, uid, "plain-token-value", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token_hash = ?`, "plain-token-value").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("raw token found in sessions table")
	}
}

func TestSession_DeleteRevokesOnlyThatSession(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess4@example.com")
	now := time.Now().UTC()
	for _, tok := range []string{"phone", "laptop"} {
		if err := s.CreateSession(ctx, uid, tok, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteSession(ctx, "phone"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.GetSessionUserID(ctx, "phone", now); got != 0 {
		t.Errorf("deleted session still valid (user %d)", got)
	}
	if got, _, _ := s.GetSessionUserID(ctx, "laptop", now); got != uid {
		t.Errorf("other session: want user %d, got %d", uid, got)
	}
}

func TestUpdateUserPassword_RevokesAllSessionsOfUser(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess5@example.com")
	other := newSessionTestUser(t, s, "sess6@example.com")
	now := time.Now().UTC()
	for _, tok := range []string{"a", "b"} {
		if err := s.CreateSession(ctx, uid, tok, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateSession(ctx, other, "c", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUserPassword(ctx, uid, "newhash"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"a", "b"} {
		if got, _, _ := s.GetSessionUserID(ctx, tok, now); got != 0 {
			t.Errorf("session %q survived password change", tok)
		}
	}
	if got, _, _ := s.GetSessionUserID(ctx, "c", now); got != other {
		t.Errorf("other user's session must survive, got user %d", got)
	}
}

func TestCreateSession_PrunesExpiredRows(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()
	uid := newSessionTestUser(t, s, "sess7@example.com")
	if err := s.CreateSession(ctx, uid, "stale", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, uid, "fresh", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("want 1 session row after pruning, got %d", n)
	}
}
