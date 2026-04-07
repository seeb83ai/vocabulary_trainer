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

// Client is the common interface for all LLM providers.
type Client interface {
	// Generate sends prompt to the LLM and returns the text response.
	Generate(ctx context.Context, prompt string) (string, error)
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

// ── OpenAI ────────────────────────────────────────────────────────────────────

type openAIClient struct {
	apiKey     string
	httpClient *http.Client
	BaseURL    string // overridable for tests
}

func (c *openAIClient) Name() string { return "openai" }

func (c *openAIClient) Generate(ctx context.Context, prompt string) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	body, _ := json.Marshal(map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 500,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("openai: decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// ── Anthropic ─────────────────────────────────────────────────────────────────

type anthropicClient struct {
	apiKey     string
	httpClient *http.Client
	BaseURL    string // overridable for tests
}

func (c *anthropicClient) Name() string { return "anthropic" }

func (c *anthropicClient) Generate(ctx context.Context, prompt string) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://api.anthropic.com"
	}
	body, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 500,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
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

func (c *geminiClient) Generate(ctx context.Context, prompt string) (string, error) {
	base := c.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1beta/models/gemini-1.5-flash:generateContent?key=%s", base, c.apiKey)
	body, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
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
