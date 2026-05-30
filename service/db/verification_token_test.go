package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

// TestCreateUser_HashesVerificationToken verifies the DB row stores the
// SHA-256 hash of the verification token, never the plaintext. A DB
// breach should not yield directly-usable verification links.
func TestCreateUser_HashesVerificationToken(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	plainToken := "this-is-the-plaintext-token-1234567890abcdef"
	_, err := s.CreateUser(ctx, "hashtest@example.com", "irrelevant-hash", plainToken, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	var stored string
	if err := s.db.QueryRowContext(ctx,
		`SELECT verification_token FROM users WHERE email = ?`, "hashtest@example.com").
		Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored == plainToken {
		t.Fatalf("stored token equals plaintext (must be hashed)")
	}
	wantHash := sha256.Sum256([]byte(plainToken))
	if stored != hex.EncodeToString(wantHash[:]) {
		t.Errorf("stored token is not SHA-256(plain). got=%q", stored)
	}
}

// TestSetUserEmailVerified_AcceptsPlaintextToken verifies that the
// verification flow continues to accept the plaintext token from the URL
// — the DB read side hashes the incoming value to look up the row.
func TestSetUserEmailVerified_AcceptsPlaintextToken(t *testing.T) {
	s := openTestDB(t)
	ctx := context.Background()

	plain := "another-plaintext-token-abcdef0123456789"
	if _, err := s.CreateUser(ctx, "verify@example.com", "hash", plain, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := s.SetUserEmailVerified(ctx, plain)
	if err != nil {
		t.Fatalf("SetUserEmailVerified: %v", err)
	}
	if user == nil {
		t.Fatal("expected non-nil user for valid plaintext token")
	}
	if !user.EmailVerified {
		t.Error("user.EmailVerified should be true")
	}

	// Replay with the same plaintext should now fail (token cleared).
	user2, err := s.SetUserEmailVerified(ctx, plain)
	if err != nil {
		t.Fatalf("SetUserEmailVerified replay: %v", err)
	}
	if user2 != nil {
		t.Error("token must not be reusable after verification")
	}
}
