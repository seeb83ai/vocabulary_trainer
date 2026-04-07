package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Request holds the prompt for an LLM call.
// System is an optional high-trust instruction; User is the (potentially
// user-influenced) message. Keeping them separate lets each provider place
// them in the correct API field rather than concatenating into a single string.
type Request struct {
	System string
	User   string
}

// Client is the common interface for all LLM providers.
type Client interface {
	// Generate sends req to the LLM and returns the text response.
	Generate(ctx context.Context, req Request) (string, error)
	// Name returns a human-readable provider name for logging.
	Name() string
}

// NewClientFromEnv checks OPENAI_API_KEY, ANTHROPIC_API_KEY, and GEMINI_API_KEY
// in that order and returns the first available client. Returns nil if none are set.
func NewClientFromEnv() Client {
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return &openAIClient{apiKey: key, httpClient: http.DefaultClient}
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return &anthropicClient{apiKey: key, httpClient: http.DefaultClient}
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return &geminiClient{apiKey: key, httpClient: http.DefaultClient}
	}
	return nil
}

// ── OpenAI (Responses API) ────────────────────────────────────────────────────

type openAIClient struct {
	apiKey     string
	httpClient *http.Client
	BaseURL    string // overridable for tests
}

func (c *openAIClient) Name() string { return "openai" }

func (c *openAIClient) Generate(ctx context.Context, req Request) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	payload := map[string]any{
		"model": "gpt-5.4-nano",
		"input": req.User,
	}
	if req.System != "" {
		payload["instructions"] = req.System
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("openai: decode: %w", err)
	}
	if len(result.Output) == 0 || len(result.Output[0].Content) == 0 {
		return "", fmt.Errorf("openai: no output in response")
	}
	return result.Output[0].Content[0].Text, nil
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

type anthropicClient struct {
	apiKey     string
	httpClient *http.Client
	BaseURL    string // overridable for tests
}

func (c *anthropicClient) Name() string { return "anthropic" }

func (c *anthropicClient) Generate(ctx context.Context, req Request) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	payload := map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 2000,
		"messages":   []map[string]string{{"role": "user", "content": req.User}},
	}
	if req.System != "" {
		payload["system"] = req.System
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("anthropic: decode: %w", err)
	}
	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content in response")
}

// ── Gemini ────────────────────────────────────────────────────────────────────

type geminiClient struct {
	apiKey     string
	httpClient *http.Client
	BaseURL    string // overridable for tests
}

func (c *geminiClient) Name() string { return "gemini" }

func (c *geminiClient) Generate(ctx context.Context, req Request) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/gemini-1.5-flash:generateContent?key=%s", base, c.apiKey)

	payload := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": req.User}}},
		},
	}
	if req.System != "" {
		payload["system_instruction"] = map[string]any{
			"parts": []map[string]string{{"text": req.System}},
		}
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("gemini: decode: %w", err)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: no content in response")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}
