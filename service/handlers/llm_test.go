package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vocabulary_trainer/handlers"
	"vocabulary_trainer/llm"

	"github.com/go-chi/chi/v5"
)

// mockLLMClient implements llm.Client for testing.
type mockLLMClient struct {
	text string
	err  error
}

func (m *mockLLMClient) Generate(_ context.Context, _ string) (string, error) {
	return m.text, m.err
}
func (m *mockLLMClient) Name() string { return "mock" }

// Ensure mockLLMClient satisfies the interface at compile time.
var _ llm.Client = (*mockLLMClient)(nil)

func newLLMRouter(client llm.Client) http.Handler {
	llmH := &handlers.LLMHandler{Client: client}
	r := chi.NewRouter()
	r.Post("/api/llm/scene", llmH.GenerateScene)
	return r
}

func TestLLMGenerateScene_OK(t *testing.T) {
	r := newLLMRouter(&mockLLMClient{text: "Jackie Chan trips over the sword."})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/llm/scene",
		strings.NewReader(`{"prompt":"write a scene"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Text != "Jackie Chan trips over the sword." {
		t.Errorf("text = %q", resp.Text)
	}
}

func TestLLMGenerateScene_EmptyPrompt(t *testing.T) {
	r := newLLMRouter(&mockLLMClient{text: "irrelevant"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/llm/scene",
		strings.NewReader(`{"prompt":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLLMGenerateScene_BadJSON(t *testing.T) {
	r := newLLMRouter(&mockLLMClient{text: "irrelevant"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/llm/scene",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLLMGenerateScene_ClientError(t *testing.T) {
	r := newLLMRouter(&mockLLMClient{err: fmt.Errorf("network error")})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/llm/scene",
		strings.NewReader(`{"prompt":"write a scene"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
