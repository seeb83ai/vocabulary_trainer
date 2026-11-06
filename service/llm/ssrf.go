package llm

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// blockedNets are non-public ranges that the net.IP helpers in isPublicIP
// do not cover.
var blockedNets = func() []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range []string{
		"0.0.0.0/8",      // "this network"; 0.x may reach the local host
		"100.64.0.0/10",  // carrier-grade NAT (also Tailscale)
		"192.0.0.0/24",   // IETF protocol assignments
		"198.18.0.0/15",  // benchmarking
		"240.0.0.0/4",    // reserved, incl. 255.255.255.255
		"64:ff9b::/96",   // NAT64: can embed any IPv4 address
		"64:ff9b:1::/48", // local-use NAT64
		"2002::/16",      // 6to4: can embed any IPv4 address
		"2001::/32",      // Teredo
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}()

// isPublicIP reports whether ip is a routable public address. Loopback,
// private (RFC1918 / fc00::/7), link-local (incl. 169.254.169.254 cloud
// metadata), multicast, unspecified and the blockedNets ranges are all
// rejected. IPv4-mapped IPv6 addresses are checked as IPv4.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// lookupIP resolves host names for ValidateExternalURL; tests replace it.
var lookupIP = net.LookupIP

// ValidateExternalURL ensures a user-supplied URL is safe to use as an
// outbound HTTP target: it must be http(s) and must not point at an internal
// address. Host names are resolved and every resolved IP is checked, so a
// name that maps to a private address is rejected too. safeDialControl
// re-checks at connect time to defeat DNS rebinding.
func ValidateExternalURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("URL host is not a public address")
		}
		return nil
	}
	ips, err := lookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("cannot resolve URL host")
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("URL host resolves to a non-public address")
		}
	}
	return nil
}

// AllowedHosts is the operator's list of LLM endpoints (LLM_ALLOWED_HOSTS)
// that users may set as their local LLM URL even though they are not
// public, e.g. an LM Studio or Ollama server in the LAN. Keys are
// "host:port" with the host in lower case.
type AllowedHosts map[string]bool

// ParseAllowedHosts parses a comma-separated list of host:port entries.
// The port is required, so allowing a LAN LLM server does not also allow
// other services on the same host.
func ParseAllowedHosts(list string) (AllowedHosts, error) {
	allowed := AllowedHosts{}
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		host, port, err := net.SplitHostPort(entry)
		if err != nil || host == "" {
			return nil, fmt.Errorf("invalid entry %q: want host:port", entry)
		}
		if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid port in entry %q", entry)
		}
		allowed[net.JoinHostPort(strings.ToLower(host), port)] = true
	}
	return allowed, nil
}

// Allows reports whether raw is an http(s) URL whose host and port (the
// scheme's default port if none is given) are on the list.
func (a AllowedHosts) Allows(raw string) bool {
	if len(a) == 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return a[net.JoinHostPort(strings.ToLower(u.Hostname()), port)]
}

// ValidateLocalURL checks a user-supplied local LLM URL: it must be on the
// operator's allowlist or be a public http(s) address.
func ValidateLocalURL(raw string, allowed AllowedHosts) error {
	if allowed.Allows(raw) {
		return nil
	}
	return ValidateExternalURL(raw)
}

// safeDialControl is a net.Dialer.Control hook that blocks connections to
// non-public addresses. Because it runs after DNS resolution on the actual
// IP being dialed, it stops SSRF via DNS rebinding that a one-time URL check
// would miss.
func safeDialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !isPublicIP(ip) {
		return fmt.Errorf("blocked connection to non-public address %q", address)
	}
	return nil
}

// userURLTimeout bounds a whole request to a user-configured LLM URL.
const userURLTimeout = 2 * time.Minute

var errRedirect = errors.New("redirects are not followed")

// userURLClient is an HTTP client for a user-configured (untrusted) URL. It
// does not follow redirects, uses no proxy and has a short timeout. Unless
// the URL is on the operator's allowlist, its dialer also refuses to connect
// to internal addresses.
func userURLClient(allowlisted bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !allowlisted {
		dialer.Control = safeDialControl
	}
	return &http.Client{
		Timeout: userURLTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		},
		Transport: &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}
