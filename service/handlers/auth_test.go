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
	authH, err := handlers.NewAuthHandler(s)
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
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
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

func TestMiddleware_NoSession_PageRedirectsToLogin(t *testing.T) {
	r := newAuthRouter(t)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	r.(http.Handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("want 302 redirect for unauthenticated page request, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("want redirect to /login, got %q", loc)
	}
}

func TestMiddleware_LoginPageAccessibleWithoutSession(t *testing.T) {
	r := newAuthRouter(t)
	rec := do(t, r, "GET", "/login", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 on /login without session, got %d", rec.Code)
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
	rec := loginReq(t, r, "me@elygor.de", "I learn zh")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body)
	}
	sessionCookie(t, rec) // asserts cookie is present
}

func TestLogin_WrongPassword(t *testing.T) {
	r := newAuthRouter(t)
	rec := loginReq(t, r, "me@elygor.de", "wrong")
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
	rec := loginReq(t, r, "admin@elygor.de", "I am the admin")
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
	loginRec := loginReq(t, r, "me@elygor.de", "I learn zh")
	cookie := sessionCookie(t, loginRec)

	rec := doWithCookie(t, r, "GET", "/api/protected", cookie)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 with valid session, got %d", rec.Code)
	}
}

func TestSession_TamperedCookieDenied(t *testing.T) {
	r := newAuthRouter(t)
	loginRec := loginReq(t, r, "me@elygor.de", "I learn zh")
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

	loginRec := loginReq(t, r, "me@elygor.de", "I learn zh")
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
