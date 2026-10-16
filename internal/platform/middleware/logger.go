package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/JustinSornay/go_skeleton/internal/platform/logging"
	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

// Logger stores a request-scoped logger (tagged with the request ID) in the
// context and writes one access-log line per request.
//
// Only the path is logged, never the query string: it may carry secrets.
// Paths listed in quiet (e.g. probes) are only logged, as warnings, when
// they fail.
func Logger(base *slog.Logger, quiet ...string) func(http.Handler) http.Handler {
	skip := make(map[string]bool, len(quiet))
	for _, p := range quiet {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			l := base.With(slog.String("request_id", requestid.From(r.Context())))
			ctx := logging.WithContext(r.Context(), l)

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			switch {
			case skip[r.URL.Path] && status < http.StatusInternalServerError:
				return
			case skip[r.URL.Path]:
				level = slog.LevelWarn // failing probe: worth seeing, not paging (e.g. draining)
			case status >= http.StatusInternalServerError:
				level = slog.LevelError
			}
			l.LogAttrs(ctx, level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", routePattern(r)),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("client_ip", ClientIPFromContext(r.Context())),
			)
		})
	}
}

// routePattern returns the matched chi pattern (low cardinality), or "" when
// no route matched.
func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		return rc.RoutePattern()
	}
	return ""
}
