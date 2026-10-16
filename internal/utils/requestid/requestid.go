// Package requestid carries the request correlation ID in the context.
package requestid

import (
	"context"
	"uuid"
)

// Header is the HTTP header used to propagate the request ID.
const Header = "X-Request-ID"

const maxLen = 128

type ctxKey struct{}

// With returns a copy of ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the request ID stored in ctx, or "".
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// New generates a new, time-ordered request ID.
func New() string { return uuid.NewV7().String() }

// Valid reports whether an incoming ID is safe to reuse: bounded length and
// a conservative charset, so a client cannot inject arbitrary content into
// logs or response headers.
func Valid(id string) bool {
	if id == "" || len(id) > maxLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == ':':
		default:
			return false
		}
	}
	return true
}
