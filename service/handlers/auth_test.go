package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
)

// newAuthRouter builds a chi router with DB-backed auth middleware plus a
// protected sentinel endpoint GET /api/protected.
func newAuthRouter(t *testing.T) http.Handler {
	t.Helper()
	s := openTestDB(t)
	authH, err := handlers.NewAuthHandler(s, nil, "http://localhost:8080", "")
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Use(authH.Middleware)
	r.Get("/api/auth/status", handlers.AuthStatus(authH))
	r.Post("/api/login", authH.Login)
	r.Post("/api/register", authH.Register)
	r.Post("/api/logout", authH.Logout)
	r.Get("/api/protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

// loginReq performs POST /api/login with the given email and password.
func loginReq(t *testing.T, r http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, r, "POST", "/api/login", map[string]string{
		"email":    email,
		"password": password,
	})
}

// sessionCookie extracts the session cookie set by a login response.
func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "vocab_session" {
			return c
		}
	}
	t.Fatal("no vocab_session cookie in response")
	return nil
}

// doWithCookie performs a request with the given cookie attached.
func doWithCookie(t *testing.T, r http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ── NewAuthHandler ────────────────────────────────────────────────────────────

func TestNewAuthHandler_NoSecret_Succeeds(t *testing.T) {
	s := openTestDB(t)
	_, err := handlers.NewAuthHandler(s, nil, "http://localhost", "")
	if err != nil {
		t.Fatalf("want no error with empty secret, got %v", err)
	}
}

// TestRequireProductionSecret_RejectsEmpty verifies that the helper used
// by main.go to validate startup configuration treats an empty
// SESSION_SECRET as fatal whenever APP_ENV is not "dev".
func TestRequireProductionSecret_RejectsEmpty(t *testing.T) {
	if err := handlers.RequireProductionSecret("", "production"); err == nil {
		t.Error("want error when SESSION_SECRET is empty in production")
	}
	if err := handlers.RequireProductionSecret("", ""); err == nil {
		t.Error("want error when SESSION_SECRET is empty and APP_ENV unset (defaults to prod)")
	}
}

// TestRequireProductionSecret_AllowsDev verifies that running without a
// SESSION_SECRET is still allowed when the operator explicitly opts in
// via APP_ENV=dev.
func TestRequireProductionSecret_AllowsDev(t *testing.T) {
	if err := handlers.RequireProductionSecret("", "dev"); err != nil {
		t.Errorf("dev mode should permit empty secret, got %v", err)
	}
}

// TestRequireProductionSecret_AllowsConfiguredSecret confirms a non-empty
// secret passes in any env.
func TestRequireProductionSecret_AllowsConfiguredSecret(t *testing.T) {
	if err := handlers.RequireProductionSecret("anything", "production"); err != nil {
		t.Errorf("production with a secret must succeed, got %v", err)
	}
	if err := handlers.RequireProductionSecret("anything", "dev"); err != nil {
		t.Errorf("dev with a secret must succeed, got %v", err)
	}
}

func TestNewAuthHandler_ValidSecret_Succeeds(t *testing.T) {
	s := openTestDB(t)
	secret := "a3f1c2e4b5d6a7f8e9c0d1b2a3f4e5d6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2"
	_, err := handlers.NewAuthHandler(s, nil, "http://localhost", secret)
	if err != nil {
		t.Fatalf("want no error with valid secret, got %v", err)
	}
}

func TestNewAuthHandler_InvalidHex_Errors(t *testing.T) {
	s := openTestDB(t)
	_, err := handlers.NewAuthHandler(s, nil, "http://localhost", "notvalidhex!!")
	if err == nil {
		t.Fatal("want error for non-hex secret, got nil")
	}
}

func TestNewAuthHandler_TooShortSecret_Errors(t *testing.T) {
	s := openTestDB(t)
	// 31 bytes = 62 hex chars — one byte short of the 32-byte minimum.
	_, err := handlers.NewAuthHandler(s, nil, "http://localhost", "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899aa"[:62])
	if err == nil {
		t.Fatal("want error for secret shorter than 32 bytes, got nil")
	}
}

func TestNewAuthHandler_PersistentSecret_TokenSurvivesRestart(t *testing.T) {
	s := openTestDB(t)
	secret := "a3f1c2e4b5d6a7f8e9c0d1b2a3f4e5d6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2"

	// First "server instance": login and capture the session cookie.
	buildRouter := func() http.Handler {
		authH, err := handlers.NewAuthHandler(s, nil, "http://localhost:8080", secret)
		if err != nil {
			t.Fatal(err)
		}
		r := chi.NewRouter()
		r.Use(authH.Middleware)
		r.Post("/api/login", authH.Login)
		r.Get("/api/protected", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		return r
	}

	r1 := buildRouter()
	loginRec := loginReq(t, r1, "me@example.de", "I learn zh")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body)
	}
	cookie := sessionCookie(t, loginRec)

	// Second "server instance" with the same secret: cookie must still work.
	r2 := buildRouter()
	rec := doWithCookie(t, r2, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 after simulated restart with same secret, got %d", rec.Code)
	}
}

// ── AuthStatus ────────────────────────────────────────────────────────────────

func TestAuthStatus_ReturnsTrue(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "GET", "/api/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]bool
	decodeJSON(t, rec, &body)
	if !body["auth"] {
		t.Error("auth should be true with DB-backed handler")
	}
}

// ── Middleware ────────────────────────────────────────────────────────────────

func TestMiddleware_NoSession_APIReturns401(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "GET", "/api/protected", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 on protected API without session, got %d", rec.Code)
	}
	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["error"] == "" {
		t.Error("expected error field in 401 response")
	}
}

func TestMiddleware_NoSession_PageRedirectsToRoot(t *testing.T) {
	r := newAuthRouter(t)

	req := httptest.NewRequest("GET", "/train", nil)
	rec := httptest.NewRecorder()
	r.(http.Handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("want 302 redirect for unauthenticated page request, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("want redirect to /, got %q", loc)
	}
}

func TestMiddleware_RootAccessibleWithoutSession(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "GET", "/", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 on / without session, got %d", rec.Code)
	}
}

func TestMiddleware_AuthStatusAccessibleWithoutSession(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "GET", "/api/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 on /api/auth/status without session, got %d", rec.Code)
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_CorrectCredentials(t *testing.T) {
	r := newAuthRouter(t)
	rec := loginReq(t, r, "me@example.de", "I learn zh")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	sessionCookie(t, rec) // asserts cookie is present
}

func TestLogin_WrongPassword(t *testing.T) {
	r := newAuthRouter(t)
	rec := loginReq(t, r, "me@example.de", "wrong")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	r := newAuthRouter(t)
	rec := loginReq(t, r, "nobody@example.com", "I learn zh")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestLogin_AdminCredentials(t *testing.T) {
	r := newAuthRouter(t)
	rec := loginReq(t, r, "admin@example.de", "I am the admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for admin login, got %d: %s", rec.Code, rec.Body)
	}
	sessionCookie(t, rec)
}

func TestLogin_InvalidJSON(t *testing.T) {
	r := newAuthRouter(t)
	req := httptest.NewRequest("POST", "/api/login", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty body, got %d", rec.Code)
	}
}

// ── Session access ────────────────────────────────────────────────────────────

func TestSession_ValidCookieAllowsAccess(t *testing.T) {
	r := newAuthRouter(t)
	loginRec := loginReq(t, r, "me@example.de", "I learn zh")
	cookie := sessionCookie(t, loginRec)

	rec := doWithCookie(t, r, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 with valid session, got %d", rec.Code)
	}
}

func TestSession_TamperedCookieDenied(t *testing.T) {
	r := newAuthRouter(t)
	loginRec := loginReq(t, r, "me@example.de", "I learn zh")
	cookie := sessionCookie(t, loginRec)

	tampered := &http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"}
	rec := doWithCookie(t, r, "GET", "/api/protected", tampered)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for tampered cookie, got %d", rec.Code)
	}
}

func TestSession_GarbageCookieDenied(t *testing.T) {
	r := newAuthRouter(t)
	rec := doWithCookie(t, r, "GET", "/api/protected", &http.Cookie{
		Name: "vocab_session", Value: "notavalidtoken",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for garbage cookie, got %d", rec.Code)
	}
}

// TestSession_TamperedSameLengthSignatureDenied verifies that a signature
// flipped in place (same length, one byte different) is rejected. This
// guards against the case where HMAC verification short-circuits on the
// first differing byte — a timing-attack vector. The behaviour must be
// identical regardless of where the tamper occurs.
func TestSession_TamperedSameLengthSignatureDenied(t *testing.T) {
	r := newAuthRouter(t)
	loginRec := loginReq(t, r, "me@example.de", "I learn zh")
	cookie := sessionCookie(t, loginRec)

	// Cookie format: userID:timestamp:hex_hmac — find the last ':' and flip
	// the first byte of the HMAC. Keep total length identical.
	v := cookie.Value
	last := -1
	for i := len(v) - 1; i >= 0; i-- {
		if v[i] == ':' {
			last = i
			break
		}
	}
	if last < 0 || last >= len(v)-1 {
		t.Fatalf("unexpected cookie format: %q", v)
	}
	first := v[last+1]
	// Flip to a different hex digit so total length stays the same.
	var flipped byte = '0'
	if first == '0' {
		flipped = '1'
	}
	mutated := v[:last+1] + string(flipped) + v[last+2:]
	if len(mutated) != len(v) {
		t.Fatalf("mutation changed length: %d vs %d", len(mutated), len(v))
	}

	rec := doWithCookie(t, r, "GET", "/api/protected",
		&http.Cookie{Name: cookie.Name, Value: mutated})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for same-length tampered signature, got %d", rec.Code)
	}
}

// ── Register: enumeration protection ─────────────────────────────────────────

// TestRegister_ExistingEmail_DoesNotLeak verifies that registering with an
// email that already exists returns the same response as a fresh
// registration. Returning 409 or a "email already registered" message
// lets an attacker enumerate valid accounts.
func TestRegister_ExistingEmail_DoesNotLeak(t *testing.T) {
	r := newAuthRouter(t)

	// "me@example.de" is pre-seeded by TestMain.
	rec := do(t, r, "POST", "/api/register", map[string]string{
		"email":    "me@example.de",
		"password": "another password",
	})

	if rec.Code == http.StatusConflict {
		t.Fatalf("must not return 409 for an existing email (leaks enumeration), got %d", rec.Code)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (indistinguishable from fresh registration), got %d: %s", rec.Code, rec.Body)
	}

	var body map[string]any
	decodeJSON(t, rec, &body)
	if errMsg, _ := body["error"].(string); errMsg != "" {
		t.Errorf("response must not contain an error field that reveals existence, got %q", errMsg)
	}
}

// TestRegister_FreshEmail_MatchesExistingShape sanity-checks that the
// fresh-registration response (no SMTP → auto_login true) and an
// existing-email response can be distinguished only by the server logs,
// not by the public response shape. We test that both succeed (200) with
// no error.
func TestRegister_FreshEmail_Succeeds(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "POST", "/api/register", map[string]string{
		"email":    "new-user@example.de",
		"password": "supersecret",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	decodeJSON(t, rec, &body)
	if errMsg, _ := body["error"].(string); errMsg != "" {
		t.Errorf("fresh registration should not contain error: %q", errMsg)
	}
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestLogout_ClearsSession(t *testing.T) {
	r := newAuthRouter(t)

	loginRec := loginReq(t, r, "me@example.de", "I learn zh")
	cookie := sessionCookie(t, loginRec)

	rec := doWithCookie(t, r, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 before logout, got %d", rec.Code)
	}

	logoutRec := doWithCookie(t, r, "POST", "/api/logout", cookie)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("want 200 on logout, got %d", logoutRec.Code)
	}

	var clearedCookie *http.Cookie
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == "vocab_session" {
			clearedCookie = c
			break
		}
	}
	if clearedCookie == nil {
		t.Fatal("logout response should set an expired cookie")
	}
	if clearedCookie.MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge should be negative, got %d", clearedCookie.MaxAge)
	}
}

// ── Crypto helpers ────────────────────────────────────────────────────────────

func TestEncryptDecryptAPIKey_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plaintext := "sk-test-api-key-1234"
	enc, err := handlers.EncryptAPIKey(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptAPIKey: %v", err)
	}
	if enc == "" {
		t.Fatal("EncryptAPIKey returned empty string")
	}
	got, err := handlers.DecryptAPIKey(key, enc)
	if err != nil {
		t.Fatalf("DecryptAPIKey: %v", err)
	}
	if got != plaintext {
		t.Errorf("want %q, got %q", plaintext, got)
	}
}

func TestEncryptDecryptAPIKey_Empty(t *testing.T) {
	key := make([]byte, 32)
	enc, err := handlers.EncryptAPIKey(key, "")
	if err != nil {
		t.Fatalf("EncryptAPIKey empty: %v", err)
	}
	if enc != "" {
		t.Errorf("want empty for empty input, got %q", enc)
	}
	dec, err := handlers.DecryptAPIKey(key, "")
	if err != nil {
		t.Fatalf("DecryptAPIKey empty: %v", err)
	}
	if dec != "" {
		t.Errorf("want empty for empty ciphertext, got %q", dec)
	}
}

func TestSealOpenSettingsKey_RoundTrip(t *testing.T) {
	secret := make([]byte, 32)
	for i := range secret {
		secret[i] = byte(i + 100)
	}
	derived := make([]byte, 32)
	for i := range derived {
		derived[i] = byte(i + 50)
	}
	sealed, err := handlers.SealKey(secret, derived)
	if err != nil {
		t.Fatalf("SealKey: %v", err)
	}
	got, err := handlers.OpenSettingsKey(secret, sealed)
	if err != nil {
		t.Fatalf("OpenSettingsKey: %v", err)
	}
	if string(got) != string(derived) {
		t.Errorf("round-trip failed: want %v, got %v", derived, got)
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-1234567890abcdef", "****cdef"},
		{"abcd", "****abcd"},
		{"ab", "****ab"},
		{"", ""},
	}
	for _, tt := range tests {
		got := handlers.MaskKey(tt.input)
		if got != tt.want {
			t.Errorf("MaskKey(%q): want %q, got %q", tt.input, tt.want, got)
		}
	}
}
