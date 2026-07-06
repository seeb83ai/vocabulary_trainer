package llm

import "testing"

func TestValidateExternalURL_RejectsInternalAndBadSchemes(t *testing.T) {
	bad := []string{
		"",
		"http://127.0.0.1/v1",
		"http://localhost:8080",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.0.0.5",
		"http://192.168.1.10",
		"http://172.16.0.1",
		"http://[::1]/v1",
		"http://0.0.0.0",
		"file:///etc/passwd",
		"ftp://example.com",
		"gopher://example.com",
		"not a url",
	}
	for _, raw := range bad {
		if err := ValidateExternalURL(raw); err == nil {
			t.Errorf("ValidateExternalURL(%q) = nil, want error", raw)
		}
	}
}

func TestValidateExternalURL_AllowsPublic(t *testing.T) {
	// IP literals avoid a DNS dependency; the hostname-resolution path is
	// exercised in production (no resolver available in the test sandbox).
	ok := []string{
		"https://8.8.8.8",
		"http://1.1.1.1:8443/v1",
		"https://[2001:4860:4860::8888]/v1",
	}
	for _, raw := range ok {
		if err := ValidateExternalURL(raw); err != nil {
			t.Errorf("ValidateExternalURL(%q) = %v, want nil", raw, err)
		}
	}
}
