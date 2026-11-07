package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"vocabulary_trainer/db"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/llm"
	"vocabulary_trainer/models"
)

// registerFreeDevice registers a new (free, non-plus) user, which logs it in
// at once because no SMTP is configured, and returns its cookies and ID.
func registerFreeDevice(t *testing.T, r http.Handler, s *db.Store, email string) (device, int64) {
	t.Helper()
	rec := do(t, r, http.MethodPost, "/api/register", map[string]string{"email": email, "password": "free user pw 1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
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
	u, err := s.GetUserByEmail(context.Background(), email)
	if err != nil || u == nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	return d, u.ID
}

func sceneRequest(t *testing.T, r http.Handler, s *db.Store, d device, userID int64) *httptest.ResponseRecorder {
	t.Helper()
	id, err := s.CreateWord(context.Background(), userID, models.CreateWordRequest{
		ZhText: "好", Pinyin: "hǎo", Translations: map[string][]string{"en": {"good"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"actor":"Jackie Chan","location":"library","room":"study","props":["sword"]}`
	return d.do(t, r, http.MethodPost, "/api/words/"+itoa(id)+"/hmm/generate-scene", body)
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func putLocalLLM(t *testing.T, r http.Handler, d device, localURL, key string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"llm_provider": "local", "llm_local_url": localURL, "llm_key": key})
	return d.do(t, r, http.MethodPut, "/api/settings/api-keys", string(body))
}

func TestSettings_LocalLLMAllowedFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		r, _ := newSessionRouterWith(t, func(h *handlers.SettingsHandler) { h.UserLocalLLM = enabled }, nil)
		d := loginDevice(t, r)
		var st models.UserSettings
		decodeJSON(t, d.do(t, r, http.MethodGet, "/api/settings", ""), &st)
		if st.LLMLocalAllowed != enabled {
			t.Errorf("UserLocalLLM=%v: llm_local_allowed = %v", enabled, st.LLMLocalAllowed)
		}
	}
}

// Off by default: no user may select the local provider, not even with a
// public or allowlisted URL, and not with the operator's default URL.
func TestPutAPIKeys_LocalProviderRejectedWhenDisabled(t *testing.T) {
	allowed, _ := llm.ParseAllowedHosts("192.168.0.50:1234")
	r, _ := newSessionRouterWith(t, func(h *handlers.SettingsHandler) { h.LLMAllowedHosts = allowed }, nil)
	d := loginDevice(t, r)
	for _, u := range []string{"", "https://8.8.8.8/v1", "http://192.168.0.50:1234"} {
		if rec := putLocalLLM(t, r, d, u, "k"); rec.Code != http.StatusBadRequest {
			t.Errorf("url %q: want 400, got %d: %s", u, rec.Code, rec.Body)
		}
	}
	body, _ := json.Marshal(map[string]string{"llm_provider": "openai", "llm_key": "sk-x"})
	if rec := d.do(t, r, http.MethodPut, "/api/settings/api-keys", string(body)); rec.Code != http.StatusOK {
		t.Errorf("other providers stay available: want 200, got %d", rec.Code)
	}
}

// Bypass fix: "local" with no URL of the user's own means the operator's
// LOCAL_LLM_URL. A dummy key must not turn that into a personal key that
// skips the plus/admin check.
func TestLLM_LocalWithoutOwnURLIsNotAPersonalKey(t *testing.T) {
	t.Setenv("LOCAL_LLM_URL", "http://127.0.0.1:1")
	t.Setenv("LOCAL_LLM_MODEL", "m")
	r, s := newSessionRouterWith(t, func(h *handlers.SettingsHandler) { h.UserLocalLLM = true },
		&mockLLMClient{text: "server scene"})
	d, uid := registerFreeDevice(t, r, s, "free-bypass@example.com")
	if rec := putLocalLLM(t, r, d, "", "dummy"); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := sceneRequest(t, r, s, d, uid); rec.Code != http.StatusForbidden {
		t.Errorf("free user, local without own URL: want 403, got %d: %s", rec.Code, rec.Body)
	}
	var cfg map[string]bool
	decodeJSON(t, d.do(t, r, http.MethodGet, "/api/translate/config", ""), &cfg)
	if cfg["llm_available"] || cfg["user_llm_key_set"] {
		t.Errorf("translate config must not report a personal LLM: %v", cfg)
	}
}

// fakeUserLLM is the user's own OpenAI-responses-compatible server.
func fakeUserLLM(t *testing.T, hits *atomic.Int32) (string, llm.AllowedHosts) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"output": []map[string]any{{
			"type": "message", "role": "assistant",
			"content": []map[string]string{{"type": "output_text", "text": "user scene"}},
		}}})
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	allowed, err := llm.ParseAllowedHosts(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, allowed
}

func TestLLM_OwnLocalServerUsedWhenEnabled(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "m")
	var hits atomic.Int32
	userURL, allowed := fakeUserLLM(t, &hits)
	r, s := newSessionRouterWith(t, func(h *handlers.SettingsHandler) {
		h.UserLocalLLM = true
		h.LLMAllowedHosts = allowed
	}, &mockLLMClient{text: "server scene"})
	d, uid := registerFreeDevice(t, r, s, "free-own@example.com")
	if rec := putLocalLLM(t, r, d, userURL, ""); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	rec := sceneRequest(t, r, s, d, uid)
	if rec.Code != http.StatusOK {
		t.Fatalf("own local server: want 200, got %d: %s", rec.Code, rec.Body)
	}
	var out map[string]string
	decodeJSON(t, rec, &out)
	if out["text"] != "user scene" || hits.Load() != 1 {
		t.Errorf("want the user's server to answer once, got %q (%d hits)", out["text"], hits.Load())
	}
}

// Settings saved while the switch was on are ignored after the operator
// turns it off: the user's server is not contacted.
func TestLLM_SavedLocalSettingsIgnoredWhenDisabled(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "m")
	var hits atomic.Int32
	userURL, allowed := fakeUserLLM(t, &hits)
	r, s := newSessionRouterWith(t, func(h *handlers.SettingsHandler) { h.LLMAllowedHosts = allowed },
		&mockLLMClient{text: "server scene"})
	d, uid := registerFreeDevice(t, r, s, "free-off@example.com")
	if err := s.UpdateUserAPIKeys(context.Background(), uid, "", "local", "", userURL); err != nil {
		t.Fatal(err)
	}
	if rec := sceneRequest(t, r, s, d, uid); rec.Code != http.StatusForbidden {
		t.Errorf("free user with saved local settings, switch off: want 403, got %d: %s", rec.Code, rec.Body)
	}
	if hits.Load() != 0 {
		t.Errorf("user's server was contacted %d times", hits.Load())
	}
}
