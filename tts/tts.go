// Package tts generates MP3 audio for Chinese text using the Microsoft Edge TTS
// WebSocket API (the same backend used by edge-tts / edge-playback).
package tts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	voice              = "zh-CN-XiaoxiaoNeural"
	wsURL              = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"

	// Windows FILETIME epoch offset from Unix epoch, in seconds.
	winEpoch = 11644473600
)

// Synthesize synthesizes zhText to MP3 and returns the raw bytes.
func Synthesize(zhText string) ([]byte, error) {
	connID := newUUID()
	secGEC, secGECVer := generateSecMsGEC()

	url := fmt.Sprintf("%s?TrustedClientToken=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		wsURL, trustedClientToken, connID, secGEC, secGECVer)

	headers := http.Header{
		"Pragma":          {"no-cache"},
		"Cache-Control":   {"no-cache"},
		"Origin":          {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"},
		"Accept-Encoding": {"gzip, deflate, br"},
		"Accept-Language": {"en-US,en;q=0.9"},
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return nil, fmt.Errorf("tts: dial: %w", err)
	}
	defer conn.Close()

	// 1. Send speech.config
	ts := jsTimestamp()
	configMsg := fmt.Sprintf(
		"X-Timestamp:%s\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n"+
			`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`,
		ts,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(configMsg)); err != nil {
		return nil, fmt.Errorf("tts: send config: %w", err)
	}

	// 2. Send SSML request
	reqID := newUUID()
	ssml := buildSSML(zhText)
	ssmlMsg := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:%sZ\r\nPath:ssml\r\n\r\n%s",
		reqID, ts, ssml,
	)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("tts: send ssml: %w", err)
	}

	// 3. Collect audio frames until turn.end
	var audio []byte
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("tts: read: %w", err)
		}

		switch msgType {
		case websocket.BinaryMessage:
			chunk, err := extractAudio(data)
			if err != nil {
				return nil, err
			}
			audio = append(audio, chunk...)

		case websocket.TextMessage:
			if isTurnEnd(data) {
				return audio, nil
			}
			// other text frames (audio.metadata, turn.start) — ignore
		}
	}
}

// extractAudio pulls the MP3 bytes out of a binary WebSocket frame.
// Frame layout: [2-byte big-endian header length][headers][audio bytes]
func extractAudio(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, nil
	}
	headerLen := int(binary.BigEndian.Uint16(data[:2]))
	if 2+headerLen > len(data) {
		return nil, fmt.Errorf("tts: malformed binary frame")
	}
	return data[2+headerLen:], nil
}

// isTurnEnd returns true when a text frame signals end-of-stream.
func isTurnEnd(data []byte) bool {
	return strings.Contains(string(data), "Path:turn.end")
}

// buildSSML wraps zhText in the SSML envelope expected by the Edge TTS service.
func buildSSML(text string) string {
	escaped := xmlEscape(text)
	return fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'>"+
			"<voice name='%s'><prosody pitch='+0Hz' rate='+0%%' volume='+0%%'>%s</prosody></voice></speak>",
		voice, escaped,
	)
}

// xmlEscape escapes the five XML special characters.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// jsTimestamp returns a JavaScript-style UTC date string, e.g.
// "Mon Feb 25 2026 14:30:45 GMT+0000 (Coordinated Universal Time)"
func jsTimestamp() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
}

// generateSecMsGEC returns the Sec-MS-GEC token and version string.
// The token is SHA-256( roundedWindowsFileTime + TrustedClientToken ) in uppercase hex.
func generateSecMsGEC() (token, version string) {
	// Seconds since Unix epoch → Windows FILETIME epoch
	unixSec := float64(time.Now().UTC().Unix())
	winSec := unixSec + winEpoch

	// Round down to nearest 5-minute window
	winSec -= float64(int64(winSec) % 300)

	// Convert to 100-nanosecond intervals (Windows FILETIME unit)
	ticks := int64(winSec * 1e7)

	input := fmt.Sprintf("%d%s", ticks, trustedClientToken)
	sum := sha256.Sum256([]byte(input))
	token = fmt.Sprintf("%X", sum[:])
	version = "1-143.0.3650.75"
	return
}

// newUUID returns a random 32-hex-char ID without dashes, suitable for X-RequestId / ConnectionId.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: time-based (should never happen in practice)
		t := time.Now().UnixNano()
		return fmt.Sprintf("%016x%016x", t, ^t)
	}
	return hex.EncodeToString(b)
}
