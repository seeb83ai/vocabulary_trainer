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
