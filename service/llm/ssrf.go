package llm

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
)

// isPublicIP reports whether ip is a routable public address. Loopback,
// private (RFC1918 / fc00::/7), link-local (incl. 169.254.169.254 cloud
// metadata), multicast, and unspecified addresses are all rejected.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return true
}

// ValidateExternalURL ensures a user-supplied URL is safe to use as an
// outbound HTTP target: it must be http(s) and must not point at an internal
// address. Host names are resolved and every resolved IP is checked, so a
// name that maps to a private address is rejected too. This is the write-time
// guard against SSRF; safeDialControl re-checks at connect time to defeat
// DNS rebinding and redirects.
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
	ips, err := net.LookupIP(host)
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

// safeDialControl is a net.Dialer.Control hook that blocks connections to
// non-public addresses. Because it runs after DNS resolution on the actual
// IP being dialed (including each redirect hop), it stops SSRF via DNS
// rebinding and redirects that a one-time URL check would miss.
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

// safeOutboundClient is an HTTP client for user-configured (untrusted) URLs.
// Its dialer refuses to connect to internal addresses at connect time.
func safeOutboundClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Control: safeDialControl}).DialContext,
		},
	}
}
