package main

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestNewServer_HasTimeouts(t *testing.T) {
	srv := newServer(":8080", http.NewServeMux())
	if srv.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", srv.Addr)
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout must be set")
	}
	if srv.ReadTimeout <= 0 {
		t.Error("ReadTimeout must be set")
	}
	if srv.WriteTimeout <= 0 {
		t.Error("WriteTimeout must be set")
	}
	if srv.IdleTimeout < time.Second {
		t.Errorf("IdleTimeout = %v, want >= 1s", srv.IdleTimeout)
	}
	if srv.ReadHeaderTimeout > srv.ReadTimeout {
		t.Errorf("ReadHeaderTimeout (%v) must not exceed ReadTimeout (%v)", srv.ReadHeaderTimeout, srv.ReadTimeout)
	}
}

func setupTemplates(t *testing.T) {
	t.Helper()
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	initTemplates(sub)
}

func TestRenderTemplate_EachPageTitle(t *testing.T) {
	setupTemplates(t)
	cases := []struct {
		name  string
		data  PageData
		title string
	}{
		{"train", PageData{Title: "Train — Vocab Trainer", ActiveNav: "train", PageScripts: []string{"hmm-builder.js", "train.js"}}, "Train — Vocab Trainer"},
		{"vocab", PageData{Title: "Vocabulary — Vocab Trainer", ActiveNav: "vocab", PageScripts: []string{"hmm-builder.js", "vocab.js"}}, "Vocabulary — Vocab Trainer"},
		{"stats", PageData{Title: "Stats — Vocab Trainer", ActiveNav: "stats", PageScripts: []string{"stats.js"}}, "Stats — Vocab Trainer"},
		{"mnemonics", PageData{Title: "Mnemonics — Vocab Trainer", ActiveNav: "mnemonics", PageScripts: []string{"mnemonics.js"}}, "Mnemonics — Vocab Trainer"},
		{"mismatches", PageData{Title: "Mismatches — Vocab Trainer", ActiveNav: "mismatches", PageScripts: []string{"mismatches.js"}}, "Mismatches — Vocab Trainer"},
		{"pinyin", PageData{Title: "Pinyin Listening · Vocab Trainer", ActiveNav: "pinyin", PageScripts: []string{"pinyin.js"}}, "Pinyin Listening · Vocab Trainer"},
		{"settings", PageData{Title: "Settings — Vocab Trainer", ActiveNav: "settings", PageScripts: []string{"settings.js"}}, "Settings — Vocab Trainer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			renderTemplate(rr, tc.name, tc.data)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "<title>"+tc.title+"</title>") {
				t.Errorf("missing title %q in body", tc.title)
			}
		})
	}
}

func TestRenderTemplate_ActiveNavLink(t *testing.T) {
	setupTemplates(t)
	cases := []struct {
		page    string
		href    string
		wantSub string
	}{
		{"train", "/train", `href="/train"`},
		{"vocab", "/vocab", `href="/vocab"`},
		{"stats", "/stats", `href="/stats"`},
		{"pinyin", "/pinyin", `href="/pinyin"`},
		{"mnemonics", "/mnemonics", `href="/mnemonics"`},
		{"mismatches", "/mismatches", `href="/mismatches"`},
	}
	for _, tc := range cases {
		t.Run(tc.page, func(t *testing.T) {
			rr := httptest.NewRecorder()
			renderTemplate(rr, tc.page, PageData{ActiveNav: tc.page})
			body := rr.Body.String()
			// Find the active link: must contain both the href and the active class
			if !strings.Contains(body, tc.wantSub) {
				t.Errorf("nav link not found: %q", tc.wantSub)
			}
			// The sidebar marks the active link with aria-current="page".
			if !strings.Contains(body, tc.wantSub+` class="app-nav-item" aria-current="page"`) {
				t.Errorf("active sidebar link not marked for %s", tc.page)
			}
		})
	}
}

func TestRenderTemplate_SettingsNavLink(t *testing.T) {
	setupTemplates(t)
	// Settings link is always visible in the nav (not hidden on non-settings pages)
	for _, page := range []string{"settings", "train", "vocab", "stats"} {
		rr := httptest.NewRecorder()
		renderTemplate(rr, page, PageData{ActiveNav: page})
		if !strings.Contains(rr.Body.String(), `href="/settings"`) {
			t.Errorf("settings nav link missing on %s page", page)
		}
	}
}

func TestRenderTemplate_PageScriptsOrder(t *testing.T) {
	setupTemplates(t)
	rr := httptest.NewRecorder()
	renderTemplate(rr, "train", PageData{
		ActiveNav:   "train",
		PageScripts: []string{"hmm-builder.js", "train.js"},
	})
	body := rr.Body.String()
	appIdx := strings.Index(body, `src="app.js"`)
	hmmIdx := strings.Index(body, `src="hmm-builder.js"`)
	trainIdx := strings.Index(body, `src="train.js"`)
	if appIdx < 0 || hmmIdx < 0 || trainIdx < 0 {
		t.Fatal("missing script tags")
	}
	if !(appIdx < hmmIdx && hmmIdx < trainIdx) {
		t.Error("script tags not in expected order: app.js < hmm-builder.js < train.js")
	}
}

func TestRenderTemplate_ExtraHeadUnescaped(t *testing.T) {
	setupTemplates(t)
	rr := httptest.NewRecorder()
	renderTemplate(rr, "stats", PageData{
		ActiveNav:   "stats",
		ExtraHead:   template.HTML(`<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>`),
		PageScripts: []string{"stats.js"},
	})
	body := rr.Body.String()
	if strings.Contains(body, "&lt;script") {
		t.Error("ExtraHead was HTML-escaped; expected raw <script> tag")
	}
	if !strings.Contains(body, `<script src="https://cdn.jsdelivr.net/npm/chart.js">`) {
		t.Error("ExtraHead chart.js script tag not found in rendered output")
	}
}

func TestRenderTemplate_UnknownName(t *testing.T) {
	setupTemplates(t)
	rr := httptest.NewRecorder()
	renderTemplate(rr, "nonexistent", PageData{})
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

// Every license link on /credits must point to a license text that is
// embedded in the binary: the text is part of the attribution we owe.
func TestCreditsLicenseLinksAreEmbedded(t *testing.T) {
	page, err := fs.ReadFile(frontendFS, "frontend/credits.html")
	if err != nil {
		t.Fatalf("read credits.html: %v", err)
	}
	links := regexp.MustCompile(`href="/licenses/([^"]+)"`).FindAllStringSubmatch(string(page), -1)
	if len(links) == 0 {
		t.Fatal("credits.html has no /licenses/ links")
	}
	for _, m := range links {
		if _, err := fs.Stat(frontendFS, "frontend/licenses/"+m[1]); err != nil {
			t.Errorf("credits.html links /licenses/%s, but the file is not embedded: %v", m[1], err)
		}
	}
}

// Directory listings expose every file; static directories without an
// index.html must answer 404.
func TestStaticFileServer_NoDirectoryListing(t *testing.T) {
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		t.Fatal(err)
	}
	srv := http.FileServer(http.FS(noDirListingFS{sub}))
	for _, p := range []string{"/landing/", "/landing/teasers/", "/landing/teasers", "/landing/teasers/010-training/", "/licenses/"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: want 404, got %d", p, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "<a href=") {
			t.Errorf("GET %s: response contains a directory listing", p)
		}
	}
	for _, p := range []string{"/landing/teasers/010-training/10-chinese_pinyin_visible.png", "/app.css"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: want 200, got %d", p, rec.Code)
		}
	}
}

// chi adds the same method twice to Allow when a static route and the "/*"
// catch-all both match the path.
func TestMethodNotAllowed_AllowHeaderHasNoDuplicates(t *testing.T) {
	r := chi.NewRouter()
	r.MethodNotAllowed(methodNotAllowed(r))
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {})
	r.Post("/api/thing", func(w http.ResponseWriter, r *http.Request) {})
	r.Get("/api/thing", func(w http.ResponseWriter, r *http.Request) {})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {})

	cases := map[string][]string{
		"/settings":  {"GET"},
		"/api/thing": {"GET", "POST"},
		"/app.css":   {"GET"},
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("DELETE", path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s: want 405, got %d", path, rec.Code)
		}
		got := rec.Header().Values("Allow")
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("DELETE %s: Allow = %v, want %v", path, got, want)
		}
	}
}
