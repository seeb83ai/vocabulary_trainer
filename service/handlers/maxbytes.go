package handlers

import "net/http"

// MaxBytes returns a middleware that caps each request body at n bytes using
// http.MaxBytesReader. Once the limit is exceeded, reads from r.Body return an
// error, so the JSON decoders in the /api handlers fail and respond with 400.
// Applied to the /api route group in main.go to keep unbounded bodies from
// exhausting memory.
func MaxBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
