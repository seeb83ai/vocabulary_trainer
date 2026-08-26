package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// ImagesHandler serves GET /api/words/{id}/image — a stock photo relevant to
// the word's translation, sourced from Unsplash and cached per word.
type ImagesHandler struct {
	Store      imagesStore
	AccessKey  string       // Unsplash API access key; empty disables the feature (503)
	HTTPClient *http.Client // overridable in tests; defaults to http.DefaultClient
	BaseURL    string       // overridable in tests; defaults to https://api.unsplash.com
}

type imageResponse struct {
	ImageURL string `json:"image_url"`
}

// GetImage handles GET /api/words/{id}/image. It returns the cached Unsplash
// image URL for the word if one has already been fetched; otherwise it looks
// up the word's English translation (falling back to German), searches
// Unsplash, caches the first result, and returns it.
func (h *ImagesHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	if h.AccessKey == "" {
		writeError(w, http.StatusServiceUnavailable, "images not configured")
		return
	}

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid word id")
		return
	}

	userID := UserIDFromContext(r.Context())
	cached, err := h.Store.GetWordImageURL(r.Context(), userID, id)
	if err != nil {
		internalError(w, err)
		return
	}
	if cached != nil {
		writeJSON(w, http.StatusOK, imageResponse{ImageURL: *cached})
		return
	}

	word, err := h.Store.GetWordByID(r.Context(), userID, id)
	if err != nil {
		internalError(w, err)
		return
	}
	if word == nil {
		writeError(w, http.StatusNotFound, "word not found")
		return
	}

	query := imageQueryText(word.Translations)
	if query == "" {
		writeError(w, http.StatusNotFound, "word has no translation to search an image for")
		return
	}

	imageURL, err := h.searchPhoto(r.Context(), query)
	if err != nil {
		log.Printf("unsplash search %q: %v", query, err)
		writeError(w, http.StatusBadGateway, "image service unavailable")
		return
	}

	if err := h.Store.SetWordImageURL(r.Context(), userID, id, imageURL); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, imageResponse{ImageURL: imageURL})
}

// imageQueryText picks the search term for a word's image: the first English
// translation, falling back to the first German translation.
func imageQueryText(translations map[string][]string) string {
	if texts, ok := translations["en"]; ok && len(texts) > 0 && texts[0] != "" {
		return texts[0]
	}
	if texts, ok := translations["de"]; ok && len(texts) > 0 && texts[0] != "" {
		return texts[0]
	}
	return ""
}

// searchPhoto queries the Unsplash "search photos" endpoint and returns the
// regular-size URL of the first result.
func (h *ImagesHandler) searchPhoto(ctx context.Context, query string) (string, error) {
	base := h.BaseURL
	if base == "" {
		base = "https://api.unsplash.com"
	}
	client := h.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reqURL := base + "/search/photos?per_page=1&query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Client-ID "+h.AccessKey)
	req.Header.Set("Accept-Version", "v1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		detail := raw
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return "", fmt.Errorf("unsplash returned HTTP %d: %s", resp.StatusCode, detail)
	}

	var result struct {
		Results []struct {
			Urls struct {
				Regular string `json:"regular"`
			} `json:"urls"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Results) == 0 || result.Results[0].Urls.Regular == "" {
		return "", fmt.Errorf("no results for query %q", query)
	}
	return result.Results[0].Urls.Regular, nil
}
