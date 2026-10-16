// Package middleware contains the cross-cutting HTTP middlewares. The order
// in which they are mounted is defined (and documented) in internal/app.
//
// Middlewares never panic to report errors: they write RFC 9457 problems
// through httpx.WriteError, exactly like handlers.
package middleware

import (
	"net/http"

	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

// RequestID reuses a well-formed incoming X-Request-ID (so IDs propagate
// across services) or generates one, stores it in the context and echoes it
// in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestid.Header)
		if !requestid.Valid(id) {
			id = requestid.New()
		}
		w.Header().Set(requestid.Header, id)
		next.ServeHTTP(w, r.WithContext(requestid.With(r.Context(), id)))
	})
}
