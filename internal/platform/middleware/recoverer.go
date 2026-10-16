package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/platform/logging"
)

// Recoverer turns a panic — always a bug, since errors are returned — into a
// logged stack trace and a 500 problem response.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity, as net/http does
				panic(rec) // let net/http abort the connection silently
			}
			logging.FromContext(r.Context()).LogAttrs(r.Context(), slog.LevelError, "panic recovered",
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())),
			)
			if ww.Status() == 0 {
				httpx.WriteError(ww, r, apperr.Internal(fmt.Errorf("panic: %v", rec)))
			}
		}()
		next.ServeHTTP(ww, r)
	})
}
