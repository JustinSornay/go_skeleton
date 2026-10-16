// Package auth authenticates bearer tokens behind a provider-agnostic
// Verifier interface.
//
// Production uses JWTVerifier, which validates signed JWTs against a JWKS
// endpoint: it works with any OIDC provider (Firebase Auth, Auth0, Keycloak,
// Cognito, Entra ID…) without pulling a vendor SDK. DevVerifier exists for
// local development and tests only; config forbids it outside dev/test.
package auth

import (
	"context"
	"errors"
	"slices"
	"time"
)

// ErrInvalidToken is returned by verifiers for any rejected token. The
// precise reason is wrapped for logs but must not be sent to clients.
var ErrInvalidToken = errors.New("invalid token")

// Principal is the authenticated caller.
type Principal struct {
	Subject  string
	Email    string
	Roles    []string
	AuthTime time.Time // when the user actually authenticated (zero if unknown)
}

// HasRole reports whether the principal holds role.
func (p Principal) HasRole(role string) bool { return slices.Contains(p.Roles, role) }

// Verifier turns a raw bearer token into a Principal.
type Verifier interface {
	Verify(ctx context.Context, token string) (Principal, error)
}

type ctxKey struct{}

// WithPrincipal returns a copy of ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the authenticated principal, if any.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok && p.Subject != ""
}
