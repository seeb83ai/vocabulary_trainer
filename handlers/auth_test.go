package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"vocabulary_trainer/handlers"

	"github.com/go-chi/chi/v5"
)

// newAuthRouter builds a chi router with auth middleware + all auth endpoints
// plus a protected sentinel endpoint GET /api/protected.
func newAuthRouter(t *testing.T, authH *handlers.AuthHandler) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	if authH != nil {
		r.Use(authH.Middleware)
	}
	r.Get("/api/auth/status", handlers.AuthStatus(authH))
	if authH != nil {
		r.Post("/api/login", authH.Login)
		r.Post("/api/logout", authH.Logout)
	}
	r.Get("/api/protected", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

// login performs POST /api/login and returns the response recorder.
func loginReq(t *testing.T, r http.Handler, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, r, "POST", "/api/login", map[string]string{
		"username": username,
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

// ── AuthStatus ────────────────────────────────────────────────────────────────

func TestAuthStatus_AuthDisabled(t *testing.T) {
	r := newAuthRouter(t, nil)
	rec := do(t, r, "GET", "/api/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]bool
	decodeJSON(t, rec, &body)
	if body["auth"] {
		t.Error("auth should be false when no credentials configured")
	}
}

func TestAuthStatus_AuthEnabled(t *testing.T) {
	authH, err := handlers.NewAuthHandler("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	r := newAuthRouter(t, authH)
	rec := do(t, r, "GET", "/api/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body map[string]bool
	decodeJSON(t, rec, &body)
	if !body["auth"] {
		t.Error("auth should be true when credentials are configured")
	}
}

// ── NewAuthHandler ────────────────────────────────────────────────────────────

func TestNewAuthHandler_NilWhenNoCredentials(t *testing.T) {
	cases := [][2]string{{"", ""}, {"user", ""}, {"", "pass"}}
	for _, c := range cases {
		h, err := handlers.NewAuthHandler(c[0], c[1])
		if err != nil {
			t.Fatalf("unexpected error for (%q, %q): %v", c[0], c[1], err)
		}
		if h != nil {
			t.Errorf("expected nil handler for (%q, %q), got non-nil", c[0], c[1])
		}
	}
}

func TestNewAuthHandler_NonNilWhenCredentialsSet(t *testing.T) {
	h, err := handlers.NewAuthHandler("admin", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Error("expected non-nil handler when both credentials are set")
	}
}

// ── Middleware: auth disabled ─────────────────────────────────────────────────

func TestMiddleware_AuthDisabled_AllowsAnything(t *testing.T) {
	r := newAuthRouter(t, nil)
	rec := do(t, r, "GET", "/api/protected", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("auth disabled: want 200 on protected route, got %d", rec.Code)
	}
}

// ── Middleware: auth enabled, no session ──────────────────────────────────────

func TestMiddleware_NoSession_APIReturns401(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
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

func TestMiddleware_NoSession_PageRedirectsToLogin(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("want 302 redirect for unauthenticated page request, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("want redirect to /login, got %q", loc)
	}
}

func TestMiddleware_LoginPageAccessibleWithoutSession(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
	rec := do(t, r, "GET", "/login", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 on /login without session, got %d", rec.Code)
	}
}

func TestMiddleware_AuthStatusAccessibleWithoutSession(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
	rec := do(t, r, "GET", "/api/auth/status", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 on /api/auth/status without session, got %d", rec.Code)
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_CorrectCredentials(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
	rec := loginReq(t, r, "user", "pass")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	sessionCookie(t, rec) // asserts cookie is present
}

func TestLogin_WrongPassword(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
	rec := loginReq(t, r, "user", "wrong")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestLogin_WrongUsername(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
	rec := loginReq(t, r, "admin", "pass")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)
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
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)

	loginRec := loginReq(t, r, "user", "pass")
	cookie := sessionCookie(t, loginRec)

	rec := doWithCookie(t, r, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 with valid session, got %d", rec.Code)
	}
}

func TestSession_TamperedCookieDenied(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)

	loginRec := loginReq(t, r, "user", "pass")
	cookie := sessionCookie(t, loginRec)

	tampered := &http.Cookie{Name: cookie.Name, Value: cookie.Value + "x"}
	rec := doWithCookie(t, r, "GET", "/api/protected", tampered)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for tampered cookie, got %d", rec.Code)
	}
}

func TestSession_GarbageCookieDenied(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)

	rec := doWithCookie(t, r, "GET", "/api/protected", &http.Cookie{
		Name: "vocab_session", Value: "notavalidtoken",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for garbage cookie, got %d", rec.Code)
	}
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestLogout_ClearsSession(t *testing.T) {
	authH, _ := handlers.NewAuthHandler("user", "pass")
	r := newAuthRouter(t, authH)

	// Login to get a session
	loginRec := loginReq(t, r, "user", "pass")
	cookie := sessionCookie(t, loginRec)

	// Verify access works before logout
	rec := doWithCookie(t, r, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 before logout, got %d", rec.Code)
	}

	// Logout
	logoutRec := doWithCookie(t, r, "POST", "/api/logout", cookie)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("want 200 on logout, got %d", logoutRec.Code)
	}

	// The original cookie still has its value, but after logout the server
	// cleared it. The client would discard the cookie; simulate that by using
	// the cleared cookie from the logout response instead.
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
