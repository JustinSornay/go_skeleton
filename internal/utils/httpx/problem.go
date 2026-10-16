package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/JustinSornay/go_skeleton/internal/utils/apperr"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
	"github.com/JustinSornay/go_skeleton/internal/utils/requestid"
)

// ProblemContentType is the media type defined by RFC 9457.
const ProblemContentType = "application/problem+json"

// Problem is an RFC 9457 "Problem Details" body, extended with a stable
// machine-readable code, the request ID and field-level validation errors.
type Problem struct {
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Status    int                 `json:"status"`
	Detail    string              `json:"detail,omitempty"`
	Instance  string              `json:"instance,omitempty"`
	Code      string              `json:"code"`
	RequestID string              `json:"request_id,omitempty"`
	Errors    []apperr.FieldError `json:"errors,omitempty"`
}

// StatusFor maps an error kind to an HTTP status. This is the only place
// where the application error model meets HTTP.
func StatusFor(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindTooLarge:
		return http.StatusRequestEntityTooLarge
	case apperr.KindRateLimited:
		return http.StatusTooManyRequests
	case apperr.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// WriteError renders err as a problem response.
//
// Client-facing fields come from *apperr.Error only; any other error becomes
// an opaque 500 so internal details never leak. Server errors (5xx) are
// logged with their full cause and the request-scoped logger.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	ae, ok := apperr.As(err)
	if !ok {
		kind := apperr.KindOf(err)
		if kind == apperr.KindUnavailable {
			ae = apperr.Unavailable("unavailable", "the request could not be completed in time", err)
		} else {
			ae = apperr.Internal(err)
		}
	}
	status := StatusFor(ae.Kind)

	if status >= http.StatusInternalServerError {
		logging.FromContext(r.Context()).LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("code", ae.Code),
			slog.Any("error", err),
		)
	}

	p := Problem{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    ae.Message,
		Instance:  r.URL.Path,
		Code:      ae.Code,
		RequestID: requestid.From(r.Context()),
		Errors:    ae.Fields,
	}
	if ae.Kind == apperr.KindUnauthenticated && w.Header().Get("WWW-Authenticate") == "" {
		w.Header().Set("WWW-Authenticate", "Bearer") // RFC 6750 §3: no error code when credentials are missing
	}
	writeProblem(w, p)
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", ProblemContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// NotFound and MethodNotAllowed render router-level misses as problems
// instead of the plain-text defaults.
func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, apperr.NotFound("route.not_found", "no route matches this path"))
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	p := Problem{
		Type:      "about:blank",
		Title:     http.StatusText(http.StatusMethodNotAllowed),
		Status:    http.StatusMethodNotAllowed,
		Detail:    "method not allowed on this route",
		Instance:  r.URL.Path,
		Code:      "route.method_not_allowed",
		RequestID: requestid.From(r.Context()),
	}
	writeProblem(w, p)
}
