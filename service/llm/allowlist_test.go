package llm

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseAllowedHosts(t *testing.T) {
	a, err := ParseAllowedHosts(" 192.168.0.50:1234 , LMStudio.lan:443,[fd00::1]:11434 ")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"http://192.168.0.50:1234",
		"http://192.168.0.50:1234/some/path",
		"https://lmstudio.lan",      // default port 443
		"https://LMSTUDIO.lan:443/", // host is case-insensitive
		"http://[fd00::1]:11434",
	} {
		if !a.Allows(raw) {
			t.Errorf("Allows(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{
		"http://192.168.0.50",      // port 80, not 1234
		"http://192.168.0.50:8332", // Bitcoin RPC on the same host
		"http://lmstudio.lan",      // port 80, not 443
		"http://192.168.0.51:1234",
		"ftp://192.168.0.50:1234",
	} {
		if a.Allows(raw) {
			t.Errorf("Allows(%q) = true, want false", raw)
		}
	}
}

func TestParseAllowedHosts_EmptyAllowsNothing(t *testing.T) {
	a, err := ParseAllowedHosts("")
	if err != nil {
		t.Fatal(err)
	}
	if a.Allows("http://127.0.0.1:1234") {
		t.Error("empty allowlist must allow nothing")
	}
}

func TestParseAllowedHosts_RejectsEntriesWithoutPort(t *testing.T) {
	for _, bad := range []string{"192.168.0.50", "http://192.168.0.50:1234", "host:notaport"} {
		if _, err := ParseAllowedHosts(bad); err == nil {
			t.Errorf("ParseAllowedHosts(%q) = nil error, want error", bad)
		}
	}
}

// withFakeDNS makes host names resolve through the given map for the test.
func withFakeDNS(t *testing.T, names map[string]string) {
	t.Helper()
	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if ip, ok := names[host]; ok {
			return []net.IP{net.ParseIP(ip)}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	t.Cleanup(func() { lookupIP = orig })
}

func TestValidateLocalURL_RejectsInternalTargets(t *testing.T) {
	withFakeDNS(t, map[string]string{
		"localhost":         "127.0.0.1",
		"internal.evil.com": "192.168.0.242",
		"rebind.evil.com":   "10.0.0.1",
	})
	allowed, _ := ParseAllowedHosts("192.168.0.50:1234")
	bad := []string{
		"http://127.0.0.1:8332",
		"http://localhost",
		"http://localhost:1234",
		"http://192.168.0.242",
		"http://[::1]:8080",
		"http://[::ffff:127.0.0.1]:8080", // IPv4-mapped IPv6
		"http://[::ffff:192.168.0.1]",
		"http://0.0.0.0:8080",
		"http://0.1.2.3",
		"http://100.64.0.1", // CGNAT / Tailscale
		"http://169.254.169.254",
		"http://[fe80::1]",
		"http://[fd00::1]",
		"http://[64:ff9b::c0a8:1]", // NAT64 of 192.168.0.1
		"http://[2002:c0a8:1::1]",  // 6to4 of 192.168.0.1
		"http://internal.evil.com",
		"http://rebind.evil.com:1234",
		"http://192.168.0.50:8332", // allowlisted host, other port
		"file:///etc/passwd",
	}
	for _, raw := range bad {
		if err := ValidateLocalURL(raw, allowed); err == nil {
			t.Errorf("ValidateLocalURL(%q) = nil, want error", raw)
		}
	}
}

func TestValidateLocalURL_AllowsPublicAndAllowlisted(t *testing.T) {
	withFakeDNS(t, map[string]string{"llm.example.com": "93.184.216.34"})
	allowed, _ := ParseAllowedHosts("192.168.0.50:1234")
	for _, raw := range []string{
		"https://llm.example.com/v1",
		"http://192.168.0.50:1234",
	} {
		if err := ValidateLocalURL(raw, allowed); err != nil {
			t.Errorf("ValidateLocalURL(%q) = %v, want nil", raw, err)
		}
	}
}

func TestSafeDialControl_BlocksNewRanges(t *testing.T) {
	for _, addr := range []string{"0.1.2.3:80", "100.64.0.1:80", "[64:ff9b::7f00:1]:80", "[2002:7f00:1::1]:80"} {
		if err := safeDialControl("tcp", addr, nil); err == nil {
			t.Errorf("safeDialControl(%q) = nil, want error", addr)
		}
	}
	if err := safeDialControl("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("public address blocked: %v", err)
	}
}

// fakeLocalLLM is an OpenAI-responses-compatible server on 127.0.0.1.
func fakeLocalLLM(t *testing.T, h http.HandlerFunc) (*httptest.Server, AllowedHosts) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	allowed, err := ParseAllowedHosts(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	return srv, allowed
}

func TestUserLocalURL_AllowlistedLANServerWorks(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "llama3")
	srv, allowed := fakeLocalLLM(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(localOKResponse("hello from LAN"))
	})
	c := NewClientFromConfig("local", "", srv.URL, allowed)
	if c == nil {
		t.Fatal("allowlisted local URL: want a client, got nil")
	}
	got, err := c.Generate(context.Background(), Request{User: "hi"})
	if err != nil || got != "hello from LAN" {
		t.Fatalf("Generate = %q, %v", got, err)
	}
}

func TestUserLocalURL_NotAllowlistedLoopbackRejected(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "llama3")
	srv, _ := fakeLocalLLM(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be contacted")
	})
	if c := NewClientFromConfig("local", "", srv.URL, nil); c != nil {
		t.Fatal("non-allowlisted loopback URL: want nil client")
	}
}

func TestUserLocalURL_RedirectToPrivateTargetNotFollowed(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "llama3")
	var hit atomic.Bool
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
	}))
	defer internal.Close()
	srv, allowed := fakeLocalLLM(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/v1/responses", http.StatusTemporaryRedirect)
	})
	c := NewClientFromConfig("local", "", srv.URL, allowed)
	if c == nil {
		t.Fatal("want client")
	}
	if _, err := c.Generate(context.Background(), Request{User: "hi"}); err == nil {
		t.Error("redirect must make the request fail")
	}
	if hit.Load() {
		t.Error("redirect target was contacted")
	}
}

func TestUserLocalURL_ResponseSizeLimited(t *testing.T) {
	t.Setenv("LOCAL_LLM_MODEL", "llama3")
	srv, allowed := fakeLocalLLM(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"`))
		w.Write([]byte(strings.Repeat("x", 2<<20)))
		w.Write([]byte(`"}]}]}`))
	})
	c := NewClientFromConfig("local", "", srv.URL, allowed)
	if c == nil {
		t.Fatal("want client")
	}
	if _, err := c.Generate(context.Background(), Request{User: "hi"}); err == nil {
		t.Error("response over the size limit must fail")
	}
}
