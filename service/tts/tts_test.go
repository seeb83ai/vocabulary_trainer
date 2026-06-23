package tts

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestXMLEscape(t *testing.T) {
	got := xmlEscape(`a&b<c>d"e'f`)
	want := `a&amp;b&lt;c&gt;d&quot;e&apos;f`
	if got != want {
		t.Errorf("xmlEscape = %q, want %q", got, want)
	}
	if xmlEscape("你好") != "你好" {
		t.Errorf("non-special chars should pass through unchanged")
	}
}

func TestBuildSSML(t *testing.T) {
	out := buildSSML("a<b")
	if !strings.Contains(out, "a&lt;b") {
		t.Errorf("buildSSML must escape the text, got %q", out)
	}
	if !strings.Contains(out, voice) {
		t.Errorf("buildSSML must reference the configured voice")
	}
	if !strings.HasPrefix(out, "<speak") || !strings.HasSuffix(out, "</speak>") {
		t.Errorf("buildSSML must be a complete <speak> envelope")
	}
}

func TestExtractAudio(t *testing.T) {
	payload := []byte("AUDIO-BYTES")
	header := []byte("Path:audio\r\n")
	frame := make([]byte, 2+len(header)+len(payload))
	binary.BigEndian.PutUint16(frame[:2], uint16(len(header)))
	copy(frame[2:], header)
	copy(frame[2+len(header):], payload)

	got, err := extractAudio(frame)
	if err != nil {
		t.Fatalf("extractAudio: %v", err)
	}
	if string(got) != "AUDIO-BYTES" {
		t.Errorf("extractAudio = %q, want the payload after the header", got)
	}

	if _, err := extractAudio([]byte{0xFF, 0xFF, 0x00}); err == nil {
		t.Errorf("expected an error for a malformed frame (header longer than data)")
	}
	if b, err := extractAudio([]byte{0x00}); err != nil || b != nil {
		t.Errorf("a too-short frame should return nil,nil; got %v,%v", b, err)
	}
}

func TestIsTurnEnd(t *testing.T) {
	if !isTurnEnd([]byte("X-Path:turn.end\r\n")) {
		t.Errorf("isTurnEnd should detect the turn.end path")
	}
	if isTurnEnd([]byte("Path:audio")) {
		t.Errorf("isTurnEnd should be false for non-turn.end frames")
	}
}

func TestNewUUID(t *testing.T) {
	id := newUUID()
	if len(id) != 32 {
		t.Errorf("newUUID length = %d, want 32 hex chars", len(id))
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("newUUID has non-hex char %q", c)
		}
	}
	if newUUID() == id {
		t.Errorf("newUUID should produce distinct values")
	}
}
