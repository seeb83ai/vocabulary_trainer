package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const sampleLine = `{"character":"好","definition":"good","pinyin":["hǎo"],"decomposition":"⿰女子","radical":"女"}` + "\n"

func TestReadSourceDownloadsWithoutFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleLine))
	}))
	defer srv.Close()

	data, err := readSource("", srv.URL)
	if err != nil {
		t.Fatalf("readSource: %v", err)
	}
	if string(data) != sampleLine {
		t.Errorf("got %q, want %q", data, sampleLine)
	}
}

func TestReadSourceFailsOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := readSource("", srv.URL); err == nil {
		t.Error("readSource returned no error for HTTP 404")
	}
}

func TestReadSourceReadsFileWithoutDownload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.txt")
	if err := os.WriteFile(path, []byte(sampleLine), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := readSource(path, "http://127.0.0.1:0/unreachable")
	if err != nil {
		t.Fatalf("readSource: %v", err)
	}
	if string(data) != sampleLine {
		t.Errorf("got %q, want %q", data, sampleLine)
	}
}
