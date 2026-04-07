package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── OpenAI ────────────────────────────────────────────────────────────────────

func TestOpenAIClient_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing or wrong Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Jackie Chan storms in."}},
			},
		})
	}))
	defer srv.Close()

	c := &openAIClient{apiKey: "test-key", httpClient: srv.Client(), BaseURL: srv.URL}
	got, err := c.Generate(context.Background(), "write a scene")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "Jackie Chan storms in." {
		t.Errorf("got %q, want %q", got, "Jackie Chan storms in.")
	}
}

func TestOpenAIClient_Generate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &openAIClient{apiKey: "bad", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestOpenAIClient_Generate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer srv.Close()

	c := &openAIClient{apiKey: "k", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

func TestAnthropicClient_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing or wrong x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": "Scene: she drops the sword."},
			},
		})
	}))
	defer srv.Close()

	c := &anthropicClient{apiKey: "test-key", httpClient: srv.Client(), BaseURL: srv.URL}
	got, err := c.Generate(context.Background(), "write a scene")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "Scene: she drops the sword." {
		t.Errorf("got %q", got)
	}
}

func TestAnthropicClient_Generate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &anthropicClient{apiKey: "bad", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestAnthropicClient_Generate_NoTextBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{
				{"type": "tool_use", "text": ""},
			},
		})
	}))
	defer srv.Close()

	c := &anthropicClient{apiKey: "k", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error when no text block")
	}
}

// ── Gemini ────────────────────────────────────────────────────────────────────

func TestGeminiClient_Generate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("missing or wrong key query param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{
					"parts": []map[string]string{{"text": "Dragon appears."}},
				}},
			},
		})
	}))
	defer srv.Close()

	c := &geminiClient{apiKey: "test-key", httpClient: srv.Client(), BaseURL: srv.URL}
	got, err := c.Generate(context.Background(), "write a scene")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "Dragon appears." {
		t.Errorf("got %q", got)
	}
}

func TestGeminiClient_Generate_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := &geminiClient{apiKey: "k", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestGeminiClient_Generate_NoCandidates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"candidates": []any{}})
	}))
	defer srv.Close()

	c := &geminiClient{apiKey: "k", httpClient: srv.Client(), BaseURL: srv.URL}
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
}

// ── Name ──────────────────────────────────────────────────────────────────────

func TestClientNames(t *testing.T) {
	if (&openAIClient{}).Name() != "openai" {
		t.Error("openAIClient.Name() != openai")
	}
	if (&anthropicClient{}).Name() != "anthropic" {
		t.Error("anthropicClient.Name() != anthropic")
	}
	if (&geminiClient{}).Name() != "gemini" {
		t.Error("geminiClient.Name() != gemini")
	}
}
