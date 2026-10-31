package tts

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// googleTTSURL is the unofficial Google Translate TTS endpoint (the speaker
// button on translate.google.com). It needs no key but has no SLA.
var googleTTSURL = "https://translate.google.com/translate_tts"

var googleClient = &http.Client{Timeout: 10 * time.Second}

// synthesizeGoogle fetches a Mandarin MP3 for zhText from Google Translate TTS.
func synthesizeGoogle(zhText string) ([]byte, error) {
	q := url.Values{
		"ie":     {"UTF-8"},
		"client": {"tw-ob"},
		"tl":     {"zh-CN"},
		"q":      {zhText},
	}
	req, err := http.NewRequest(http.MethodGet, googleTTSURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("google tts: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	resp, err := googleClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google tts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google tts: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		return nil, fmt.Errorf("google tts: unexpected content type %q", ct)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google tts: read: %w", err)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("google tts: empty audio")
	}
	return audio, nil
}
