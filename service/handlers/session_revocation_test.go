package handlers_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/llm"

	"github.com/go-chi/chi/v5"
)

const testSessionSecret = "a3f1c2e4b5d6a7f8e9c0d1b2a3f4e5d6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2"

// newSessionRouter wires the real auth middleware in front of the settings
// endpoints, the way main.go does, in production mode (Secure cookies).
func newSessionRouter(t *testing.T) http.Handler {
	t.Helper()
	r, _ := newSessionRouterWith(t, nil, nil)
	return r
}

// newSessionRouterWithLLMHosts is newSessionRouter with the local LLM
// provider turned on for users (LLM_USER_LOCAL_ENABLED) and LLM_ALLOWED_HOSTS set.
func newSessionRouterWithLLMHosts(t *testing.T, allowed llm.AllowedHosts) http.Handler {
	t.Helper()
	r, _ := newSessionRouterWith(t, func(h *handlers.SettingsHandler) {
		h.UserLocalLLM = true
		h.LLMAllowedHosts = allowed
	}, nil)
	return r
}

// newSessionRouterWith is newSessionRouter with the settings handler set up by
// configure and the LLM endpoints backed by serverLLM (the operator's LLM).
func newSessionRouterWith(t *testing.T, configure func(*handlers.SettingsHandler), serverLLM llm.Client) (http.Handler, *db.Store) {
	t.Helper()
	s := openTestDB(t)
	authH, err := handlers.NewAuthHandlerWithEnv(s, nil, "https://example.com", testSessionSecret, "")
	if err != nil {
		t.Fatal(err)
	}
	settingsH := handlers.NewSettingsHandler(s, authH.Secret())
	if configure != nil {
		configure(settingsH)
	}
	llmH := &handlers.LLMHandler{Client: serverLLM, Store: s, SettingsHandler: settingsH}
	translateH := &handlers.TranslateHandler{Store: s, SettingsHandler: settingsH}
	r := chi.NewRouter()
	r.Use(authH.Middleware)
	r.Post("/api/login", authH.Login)
	r.Post("/api/register", authH.Register)
	r.Post("/api/logout", authH.Logout)
	r.Post("/api/change-password", authH.ChangePassword)
	r.Get("/api/settings", settingsH.Get)
	r.Put("/api/settings/api-keys", settingsH.PutAPIKeys)
	r.Get("/api/translate/config", translateH.Config(false, serverLLM != nil))
	r.Post("/api/words/{id}/hmm/generate-scene", llmH.GenerateScene)
	return r, s
}

// device is one browser: the cookies it got from its last login.
type device struct {
	session, settingsKey *http.Cookie
}

func loginDevice(t *testing.T, r http.Handler) device {
	t.Helper()
	rec := loginReq(t, r, "me@example.de", "I learn zh")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var d device
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "vocab_session":
			d.session = c
		case "vocab_settings_key":
			d.settingsKey = c
		}
	}
	if d.session == nil || d.settingsKey == nil {
		t.Fatal("login did not set both cookies")
	}
	return d
}

func (d device) do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if d.session != nil {
		req.AddCookie(d.session)
	}
	if d.settingsKey != nil {
		req.AddCookie(d.settingsKey)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLogout_RevokesSessionOnServer(t *testing.T) {
	r := newSessionRouter(t)
	d := loginDevice(t, r)

	if rec := d.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusOK {
		t.Fatalf("before logout: want 200, got %d", rec.Code)
	}
	if rec := d.do(t, r, "POST", "/api/logout", ""); rec.Code != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", rec.Code)
	}
	// Replay the old cookie, as an attacker with a copy of it would.
	if rec := d.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("replayed cookie after logout: want 401, got %d", rec.Code)
	}
}

func TestLogout_KeepsOtherDevicesLoggedIn(t *testing.T) {
	r := newSessionRouter(t)
	phone := loginDevice(t, r)
	laptop := loginDevice(t, r)

	phone.do(t, r, "POST", "/api/logout", "")

	if rec := laptop.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusOK {
		t.Errorf("other device after logout: want 200, got %d", rec.Code)
	}
}

func TestLogout_ClearCookiesUseSameAttributesAsLogin(t *testing.T) {
	r := newSessionRouter(t)
	d := loginDevice(t, r)
	rec := d.do(t, r, "POST", "/api/logout", "")

	seen := map[string]bool{}
	for _, c := range rec.Result().Cookies() {
		seen[c.Name] = true
		if c.MaxAge >= 0 {
			t.Errorf("%s: MaxAge want < 0, got %d", c.Name, c.MaxAge)
		}
		if c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
			t.Errorf("%s: want Path=/ HttpOnly Secure SameSite=Strict, got Path=%q HttpOnly=%v Secure=%v SameSite=%v",
				c.Name, c.Path, c.HttpOnly, c.Secure, c.SameSite)
		}
	}
	if !seen["vocab_session"] || !seen["vocab_settings_key"] {
		t.Errorf("logout must clear both cookies, got %v", seen)
	}
}

func TestChangePassword_LogsOutOtherDevices(t *testing.T) {
	r := newSessionRouter(t)
	phone := loginDevice(t, r)
	laptop := loginDevice(t, r)

	rec := phone.do(t, r, "POST", "/api/change-password",
		`{"current_password":"I learn zh","new_password":"new password 123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change-password: want 200, got %d: %s", rec.Code, rec.Body)
	}

	if rec := laptop.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("other device after password change: want 401, got %d", rec.Code)
	}

	// The device that changed the password gets a new session and stays logged in.
	var fresh device
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "vocab_session":
			fresh.session = c
		case "vocab_settings_key":
			fresh.settingsKey = c
		}
	}
	if fresh.session == nil {
		t.Fatal("change-password must set a new session cookie")
	}
	if rec := fresh.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusOK {
		t.Errorf("new session after password change: want 200, got %d", rec.Code)
	}
}

func TestSession_TamperedTokenDenied(t *testing.T) {
	r := newSessionRouter(t)
	d := loginDevice(t, r)
	v := d.session.Value
	for _, pos := range []int{0, len(v) / 2, len(v) - 1} {
		flipped := byte('A')
		if v[pos] == 'A' {
			flipped = 'B'
		}
		mutated := v[:pos] + string(flipped) + v[pos+1:]
		bad := device{session: &http.Cookie{Name: "vocab_session", Value: mutated}}
		if rec := bad.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("token changed at %d: want 401, got %d", pos, rec.Code)
		}
	}
}

// A token of the old format ("userID:unixNano:HMAC") must no longer work,
// even when it is signed correctly with SESSION_SECRET. This also covers a
// token where an attacker changed the user ID and re-signed it.
func TestSession_OldSignedTokenDenied(t *testing.T) {
	r := newSessionRouter(t)
	secret, _ := hex.DecodeString(testSessionSecret)
	payload := "2:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	old := payload + ":" + hex.EncodeToString(mac.Sum(nil))

	d := device{session: &http.Cookie{Name: "vocab_session", Value: old}}
	if rec := d.do(t, r, "GET", "/api/settings", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("old-format signed token: want 401, got %d", rec.Code)
	}
}

func TestSession_TokenIsRandomAndLongEnough(t *testing.T) {
	r := newSessionRouter(t)
	a := loginDevice(t, r).session.Value
	b := loginDevice(t, r).session.Value
	if a == b {
		t.Error("two logins returned the same session token")
	}
	// ≥ 128 bit: 32 random bytes base64url-encoded are 43 characters.
	if len(a) < 43 {
		t.Errorf("session token too short: %d chars", len(a))
	}
	if strings.Contains(a, ":") {
		t.Errorf("session token must not carry the user ID: %q", a)
	}
}

// The settings-key cookie is bound to its session: after logout it cannot
// be used with another session to decrypt the stored API keys.
func TestSettingsKey_BoundToSession(t *testing.T) {
	r := newSessionRouter(t)
	a := loginDevice(t, r)
	b := loginDevice(t, r)

	mixed := device{session: b.session, settingsKey: a.settingsKey}
	rec := mixed.do(t, r, "PUT", "/api/settings/api-keys", `{"deepl_key":"secret-key"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("settings key of another session: want 400, got %d: %s", rec.Code, rec.Body)
	}
	if rec := b.do(t, r, "PUT", "/api/settings/api-keys", `{"deepl_key":"secret-key"}`); rec.Code != http.StatusOK {
		t.Errorf("own settings key: want 200, got %d: %s", rec.Code, rec.Body)
	}
}
