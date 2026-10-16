package middleware

import (
	"net/http"

	"github.com/rs/cors"

	"github.com/JustinSornay/go_skeleton/internal/utils/requestid"
)

// CORS allows browser calls from the configured origins only. "*" allows any
// origin (refused by config in production). Credentials (cookies) are not
// allowed: the API authenticates with bearer tokens.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", requestid.Header},
		ExposedHeaders: []string{requestid.Header, "Retry-After", "Location"},
		MaxAge:         600,
	}).Handler
}
