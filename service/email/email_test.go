package email

import (
	"os"
	"testing"
)

func clearSMTPEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_FROM", "SMTP_USER", "SMTP_PASS"} {
		old := os.Getenv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if old != "" {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestNewSenderFromEnv_DisabledWithoutHost(t *testing.T) {
	clearSMTPEnv(t)
	if s := NewSenderFromEnv(); s != nil {
		t.Errorf("expected nil sender when SMTP_HOST is unset")
	}
}

func TestNewSenderFromEnv_DefaultsPort587(t *testing.T) {
	clearSMTPEnv(t)
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_USER", "user@example.com")
	s := NewSenderFromEnv()
	if s == nil {
		t.Fatal("expected a sender when SMTP_HOST is set")
	}
	if s.port != "587" {
		t.Errorf("default port = %q, want 587", s.port)
	}
	// from falls back to user when SMTP_FROM is unset.
	if s.from != "user@example.com" {
		t.Errorf("from = %q, want fallback to SMTP_USER", s.from)
	}
}

func TestNewSenderFromEnv_ExplicitValues(t *testing.T) {
	clearSMTPEnv(t)
	os.Setenv("SMTP_HOST", "mail.example.com")
	os.Setenv("SMTP_PORT", "2525")
	os.Setenv("SMTP_FROM", "noreply@example.com")
	os.Setenv("SMTP_USER", "u")
	os.Setenv("SMTP_PASS", "p")
	s := NewSenderFromEnv()
	if s == nil {
		t.Fatal("expected a sender")
	}
	if s.host != "mail.example.com" || s.port != "2525" || s.from != "noreply@example.com" || s.user != "u" || s.password != "p" {
		t.Errorf("sender fields not populated as expected: %+v", *s)
	}
}
