package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vocabulary_trainer/db"

	"golang.org/x/crypto/bcrypt"
)

// TestLogin_NoUserStillComparesBcrypt verifies that a login attempt for an
// email with no matching account still performs a bcrypt comparison. This
// keeps the response timing of the "no such user" path indistinguishable
// from the "wrong password" path, defeating user-enumeration via timing.
func TestLogin_NoUserStillComparesBcrypt(t *testing.T) {
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	a, err := NewAuthHandlerWithEnv(s, nil, "http://localhost:8080", "", "dev")
	if err != nil {
		t.Fatal(err)
	}

	orig := bcryptCompare
	defer func() { bcryptCompare = orig }()
	var compares int
	bcryptCompare = func(hash, password []byte) error {
		compares++
		return orig(hash, password)
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "nobody@example.com",
		"password": "whatever",
	})
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	a.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unknown email, got %d", rec.Code)
	}
	if compares != 1 {
		t.Errorf("no-user path must perform exactly one bcrypt compare, got %d", compares)
	}
}

// TestDummyBcryptHashIsValid guards that the constant used for the dummy
// comparison is a real bcrypt hash (so the compare does real work).
func TestDummyBcryptHashIsValid(t *testing.T) {
	if _, err := bcrypt.Cost([]byte(dummyBcryptHash)); err != nil {
		t.Fatalf("dummyBcryptHash is not a valid bcrypt hash: %v", err)
	}
}
