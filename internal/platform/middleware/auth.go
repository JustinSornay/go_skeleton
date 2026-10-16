package middleware

import (
	"log/slog"
	"net/http"

	"github.com/JustinSornay/go_skeleton/internal/platform/actor"
	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/auth"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/platform/logging"
)

var (
	errInvalidToken = apperr.Unauthenticated("auth.invalid_token", "the access token is invalid or expired")
	errAuthRequired = apperr.Unauthenticated("auth.required", "authentication is required")
	errRoleRequired = apperr.Forbidden("auth.forbidden", "you are not allowed to perform this action")
)

// Authenticate verifies the bearer token when one is present and stores the
// principal (and the matching actor) in the context. Requests without an
// Authorization header pass through anonymously: use RequireAuth on routes
// that need a user. A present but invalid token is always rejected.
func Authenticate(v auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}
			token, err := auth.BearerToken(header)
			if err == nil {
				var p auth.Principal
				if p, err = v.Verify(r.Context(), token); err == nil {
					ctx := auth.WithPrincipal(r.Context(), p)
					ctx = actor.With(ctx, actor.User(p.Subject))
					ctx = logging.WithContext(ctx, logging.FromContext(ctx).With(slog.String("user_id", p.Subject)))
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			logging.FromContext(r.Context()).LogAttrs(r.Context(), slog.LevelInfo, "token rejected", slog.Any("error", err))
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			httpx.WriteError(w, r, errInvalidToken.Wrap(err))
		})
	}
}

// RequireAuth rejects anonymous requests with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.FromContext(r.Context()); !ok {
			httpx.WriteError(w, r, errAuthRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole rejects requests whose principal holds none of roles (403),
// or anonymous requests (401).
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.FromContext(r.Context())
			if !ok {
				httpx.WriteError(w, r, errAuthRequired)
				return
			}
			for _, role := range roles {
				if p.HasRole(role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.WriteError(w, r, errRoleRequired)
		})
	}
}
