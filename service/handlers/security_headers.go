package handlers

import "net/http"

// SecurityHeaders applies a baseline set of security response headers to
// every request. Keep the policy here rather than inlining in main.go so
// it is unit-testable and reused if other entry points are added.
//
// Notes on CSP:
//   - script-src 'self' — no inline scripts; all JS must be served from
//     same-origin static files. The frontend follows this convention.
//   - style-src 'self' 'unsafe-inline' — Tailwind utility classes are
//     applied inline by the template, and the layout includes a
//     <style> block. 'unsafe-inline' is the standard exception for
//     Tailwind-style apps; it does not weaken script execution.
//   - img-src 'self' data: — allow data: URIs for SVG icons.
//   - connect-src 'self' — XHR/fetch only to same origin.
//
// HSTS is intentionally absent here: it must be issued only over HTTPS,
// which is the reverse proxy's responsibility (see deploy/nginx.conf).
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"font-src 'self' data:; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// SecurityHeaders is a middleware that sets baseline security headers on
// every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-XSS-Protection", "0")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}
