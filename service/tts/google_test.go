package tts

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeGoogle starts a fake Google Translate TTS server and points the package at it.
func fakeGoogle(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := googleTTSURL
	googleTTSURL = srv.URL
	t.Cleanup(func() { googleTTSURL = old })
}

func TestSynthesizeUsesGoogleTranslateMandarin(t *testing.T) {
	var gotTL, gotQ, gotClient string
	fakeGoogle(t, func(w http.ResponseWriter, r *http.Request) {
		gotTL = r.URL.Query().Get("tl")
		gotQ = r.URL.Query().Get("q")
		gotClient = r.URL.Query().Get("client")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("google-mp3"))
	})

	got, err := Synthesize("我是学生")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(got) != "google-mp3" {
		t.Errorf("Synthesize = %q, want the Google audio", got)
	}
	if gotTL != "zh-CN" || gotQ != "我是学生" || gotClient != "tw-ob" {
		t.Errorf("request tl=%q q=%q client=%q, want tl=zh-CN q=我是学生 client=tw-ob", gotTL, gotQ, gotClient)
	}
}

// stubEdge replaces the Edge fallback and records the text it was called with.
func stubEdge(t *testing.T) *string {
	t.Helper()
	var got string
	old := edgeSynth
	edgeSynth = func(text string) ([]byte, error) { got = text; return []byte("edge-mp3"), nil }
	t.Cleanup(func() { edgeSynth = old })
	return &got
}

func TestSynthesizeFallsBackToEdgeWhenGoogleFails(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"rate limited": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("slow down"))
		},
		"captcha page": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>sorry</html>"))
		},
		"empty audio": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			fakeGoogle(t, h)
			edgeText := stubEdge(t)

			got, err := Synthesize("你好")
			if err != nil {
				t.Fatalf("Synthesize: %v", err)
			}
			if string(got) != "edge-mp3" || *edgeText != "你好" {
				t.Errorf("Synthesize = %q (edge called with %q), want the Edge audio for 你好", got, *edgeText)
			}
		})
	}
}

func TestSynthesizeFallsBackToEdgeWhenGoogleIsUnreachable(t *testing.T) {
	old := googleTTSURL
	googleTTSURL = "http://127.0.0.1:1/translate_tts"
	t.Cleanup(func() { googleTTSURL = old })
	stubEdge(t)

	got, err := Synthesize("你好")
	if err != nil || string(got) != "edge-mp3" {
		t.Errorf("Synthesize = %q, %v; want the Edge audio", got, err)
	}
}
