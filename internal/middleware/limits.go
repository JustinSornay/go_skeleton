package middleware

import (
	"context"
	"net/http"
	"time"
)

// BodyLimit caps the request body size. Decoding a larger body fails with a
// 413 problem (see httpx.DecodeJSON).
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout bounds the request context. It does not write any response
// itself: handlers and the database driver observe the deadline, and the
// resulting context.DeadlineExceeded is rendered as a 503 problem.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
