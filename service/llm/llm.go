package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

// NewClientFromEnv collects every configured LLM provider in priority order
// (local first, then OpenAI, Anthropic, Gemini) and returns them wrapped in a
// fallbackClient so that if the primary provider fails the next one is tried
// automatically. Returns nil if no provider is configured.
func NewClientFromEnv() Client {
	var clients []Client
	if url := os.Getenv("LOCAL_LLM_URL"); url != "" {
		if model := os.Getenv("LOCAL_LLM_MODEL"); model != "" {
			clients = append(clients, &localClient{
				baseURL:    url,
				model:      model,
				apiKey:     os.Getenv("LOCAL_LLM_API_KEY"),
				httpClient: http.DefaultClient,
			})
		}
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		clients = append(clients, &openAIClient{apiKey: key, httpClient: http.DefaultClient})
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		clients = append(clients, &anthropicClient{apiKey: key, httpClient: http.DefaultClient})
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		clients = append(clients, &geminiClient{apiKey: key, httpClient: http.DefaultClient})
	}
	switch len(clients) {
	case 0:
		return nil
	case 1:
		return clients[0]
	default:
		return &fallbackClient{clients: clients}
	}
}

// ── Fallback chain ─────────────────────────────────────────────────────────────

// fallbackClient tries each provider in order. If a provider returns an error
// (network failure, HTTP error, etc.) the next one is attempted automatically.
type fallbackClient struct {
	clients []Client
}

func (c *fallbackClient) Name() string {
	names := make([]string, len(c.clients))
	for i, cl := range c.clients {
		names[i] = cl.Name()
	}
	return strings.Join(names, " → ")
}

func (c *fallbackClient) Generate(ctx context.Context, req Request) (string, error) {
	var lastErr error
	for _, cl := range c.clients {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		result, err := cl.Generate(ctx, req)
		if err == nil {
			return result, nil
		}
		lastErr = err
		log.Printf("LLM provider %s failed (%v); trying next provider", cl.Name(), err)
	}
	return "", lastErr
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

// ── Local / private model (OpenAI-compatible chat completions) ────────────────
//
// Compatible with Ollama, LM Studio, LocalAI, vLLM, and any server that
// implements the OpenAI chat completions API at POST /v1/chat/completions.
//
// Required env vars: LOCAL_LLM_URL, LOCAL_LLM_MODEL
// Optional env var:  LOCAL_LLM_API_KEY  (bearer token; some setups require it)

type localClient struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

func (c *localClient) Name() string {
	return fmt.Sprintf("local(%s @ %s)", c.model, c.baseURL)
}

func (c *localClient) Generate(ctx context.Context, req Request) (string, error) {
	messages := []map[string]string{}
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.User})

	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("local: status %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("local: decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("local: no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}
