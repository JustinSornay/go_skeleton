package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig configures JWTVerifier.
type JWTConfig struct {
	JWKSURL    string
	Issuer     string
	Audience   string
	RolesClaim string        // top-level claim holding roles; default "roles"
	Leeway     time.Duration // clock skew tolerance; default 30s
}

// JWTVerifier validates asymmetric JWTs (RS*, ES*, PS*, EdDSA) whose keys are
// published at a JWKS URL. Keys are cached and refreshed in the background,
// and an unknown "kid" triggers a rate-limited refresh (key rotation).
//
// Firebase Auth example:
//
//	JWKSURL:  https://www.googleapis.com/service_accounts/v1/jwk/securetoken@system.gserviceaccount.com
//	Issuer:   https://securetoken.google.com/<project-id>
//	Audience: <project-id>
type JWTVerifier struct {
	keys   keyfunc.Keyfunc
	parser *jwt.Parser
	roles  string
}

// NewJWTVerifier fetches the JWKS once and keeps it fresh until ctx is done.
func NewJWTVerifier(ctx context.Context, cfg JWTConfig) (*JWTVerifier, error) {
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURL})
	if err != nil {
		return nil, fmt.Errorf("auth: loading JWKS from %s: %w", cfg.JWKSURL, err)
	}
	return newJWTVerifier(keys, cfg), nil
}

func newJWTVerifier(keys keyfunc.Keyfunc, cfg JWTConfig) *JWTVerifier {
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	if cfg.Leeway == 0 {
		cfg.Leeway = 30 * time.Second
	}
	return &JWTVerifier{
		keys:  keys,
		roles: cfg.RolesClaim,
		parser: jwt.NewParser(
			// Never accept "none" or HMAC algorithms with public keys.
			jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "PS256", "PS384", "PS512", "EdDSA"}),
			jwt.WithIssuer(cfg.Issuer),
			jwt.WithAudience(cfg.Audience),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(cfg.Leeway),
		),
	}
}

// Verify implements Verifier.
func (v *JWTVerifier) Verify(ctx context.Context, raw string) (Principal, error) {
	claims := jwt.MapClaims{}
	if _, err := v.parser.ParseWithClaims(raw, claims, v.keys.KeyfuncCtx(ctx)); err != nil {
		return Principal{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return Principal{}, fmt.Errorf("%w: missing sub claim", ErrInvalidToken)
	}
	p := Principal{Subject: sub, Roles: stringList(claims[v.roles])}
	if email, ok := claims["email"].(string); ok {
		p.Email = email
	}
	if at, ok := claims["auth_time"].(float64); ok {
		p.AuthTime = time.Unix(int64(at), 0).UTC()
	}
	return p, nil
}

// stringList accepts ["a","b"] or "a b" (OAuth2 "scope" style).
func stringList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Fields(t)
	default:
		return nil
	}
}

// DevVerifier accepts any non-empty token of the form "subject" or
// "subject:role1,role2". It performs no cryptographic check and must only be
// wired when config allows it (AUTH_MODE=dev with APP_ENV=dev|test).
type DevVerifier struct{}

// Verify implements Verifier.
func (DevVerifier) Verify(_ context.Context, token string) (Principal, error) {
	sub, roles, _ := strings.Cut(strings.TrimSpace(token), ":")
	if sub == "" {
		return Principal{}, fmt.Errorf("%w: empty dev token", ErrInvalidToken)
	}
	p := Principal{Subject: sub, AuthTime: time.Now().UTC()}
	for r := range strings.SplitSeq(roles, ",") {
		if r = strings.TrimSpace(r); r != "" {
			p.Roles = append(p.Roles, r)
		}
	}
	return p, nil
}

// BearerToken extracts the token from an "Authorization: Bearer <token>"
// header value. Tokens in query strings are deliberately not supported: they
// end up in access logs, proxies and browser history.
func BearerToken(header string) (string, error) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", errors.New("authorization header must be 'Bearer <token>'")
	}
	return strings.TrimSpace(token), nil
}
