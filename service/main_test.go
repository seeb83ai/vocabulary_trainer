package main

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
			if !strings.Contains(body, "border-b-2 border-blue-600") {
				t.Errorf("active nav style not found for %s", tc.page)
			}
		})
	}
}

func TestRenderTemplate_SettingsNavLink(t *testing.T) {
	setupTemplates(t)
	rr := httptest.NewRecorder()
	renderTemplate(rr, "settings", PageData{ActiveNav: "settings"})
	body := rr.Body.String()
	if !strings.Contains(body, `href="/settings"`) {
		t.Error("settings nav link missing on settings page")
	}

	rr2 := httptest.NewRecorder()
	renderTemplate(rr2, "train", PageData{ActiveNav: "train"})
	if strings.Contains(rr2.Body.String(), `href="/settings"`) {
		t.Error("settings nav link should not appear on non-settings pages")
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

