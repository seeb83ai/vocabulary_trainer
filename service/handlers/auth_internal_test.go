package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestSession_ExpiredSessionDenied verifies that the server rejects a session
// after sessionTTL, even when the browser keeps sending the cookie.
func TestSession_ExpiredSessionDenied(t *testing.T) {
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateUserWithSettings(context.Background(), "exp@example.com", mustBcrypt(t, "password123"), "tok-exp", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetUserEmailVerified(context.Background(), "tok-exp"); err != nil {
		t.Fatal(err)
	}
	a, err := NewAuthHandlerWithEnv(s, nil, "http://localhost:8080", "", "dev")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	a.now = func() time.Time { return start }

	body, _ := json.Marshal(map[string]string{"email": "exp@example.com", "password": "password123"})
	rec := httptest.NewRecorder()
	a.Login(rec, httptest.NewRequest("POST", "/api/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session cookie")
	}

	protected := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	get := func() int {
		req := httptest.NewRequest("GET", "/api/settings", nil)
		req.AddCookie(session)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("fresh session: want 200, got %d", code)
	}
	a.now = func() time.Time { return start.Add(sessionTTL - time.Second) }
	if code := get(); code != http.StatusOK {
		t.Fatalf("just before expiry: want 200, got %d", code)
	}
	a.now = func() time.Time { return start.Add(sessionTTL + time.Second) }
	if code := get(); code != http.StatusUnauthorized {
		t.Errorf("expired session: want 401, got %d", code)
	}
}

func mustBcrypt(t *testing.T, pw string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}
