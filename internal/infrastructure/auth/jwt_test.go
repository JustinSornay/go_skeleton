package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testKID      = "test-key"
	testIssuer   = "https://idp.example"
	testAudience = "my-api"
)

func newTestVerifier(t *testing.T) (*JWTVerifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks, _ := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": testKID, "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}})
	kf, err := keyfunc.NewJWKSetJSON(jwks)
	if err != nil {
		t.Fatal(err)
	}
	return newJWTVerifier(kf, JWTConfig{Issuer: testIssuer, Audience: testAudience}), key
}

func sign(t *testing.T, key *rsa.PrivateKey, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = testKID
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validClaims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"sub": "user-1", "iss": testIssuer, "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "auth_time": now.Add(-time.Minute).Unix(),
		"email": "u@example.com", "roles": []string{"admin", "editor"},
	}
}

func TestJWTVerifierAcceptsValidToken(t *testing.T) {
	v, key := newTestVerifier(t)
	p, err := v.Verify(context.Background(), sign(t, key, jwt.SigningMethodRS256, validClaims()))
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "user-1" || p.Email != "u@example.com" || !p.HasRole("admin") || p.AuthTime.IsZero() {
		t.Fatalf("unexpected principal %+v", p)
	}
}

func TestJWTVerifierRejects(t *testing.T) {
	v, key := newTestVerifier(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)

	mutate := func(f func(jwt.MapClaims)) jwt.MapClaims {
		c := validClaims()
		f(c)
		return c
	}
	tests := map[string]string{
		"expired":        sign(t, key, jwt.SigningMethodRS256, mutate(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() })),
		"no expiry":      sign(t, key, jwt.SigningMethodRS256, mutate(func(c jwt.MapClaims) { delete(c, "exp") })),
		"wrong issuer":   sign(t, key, jwt.SigningMethodRS256, mutate(func(c jwt.MapClaims) { c["iss"] = "https://evil.example" })),
		"wrong audience": sign(t, key, jwt.SigningMethodRS256, mutate(func(c jwt.MapClaims) { c["aud"] = "other-api" })),
		"no subject":     sign(t, key, jwt.SigningMethodRS256, mutate(func(c jwt.MapClaims) { delete(c, "sub") })),
		"wrong key":      sign(t, other, jwt.SigningMethodRS256, validClaims()),
		"garbage":        "not.a.jwt",
	}
	// HS256 signed with the public modulus: the classic algorithm-confusion attack.
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
	hs.Header["kid"] = testKID
	tests["hmac confusion"], _ = hs.SignedString(key.N.Bytes())
	// alg "none".
	none := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
	tests["alg none"], _ = none.SignedString(jwt.UnsafeAllowNoneSignatureType)

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("want ErrInvalidToken, got %v", err)
			}
		})
	}
}

func TestStringList(t *testing.T) {
	if got := stringList("read write"); len(got) != 2 || got[1] != "write" {
		t.Errorf("space-separated: %v", got)
	}
	if got := stringList([]any{"a", 1, ""}); len(got) != 1 || got[0] != "a" {
		t.Errorf("array: %v", got)
	}
	if got := stringList(nil); got != nil {
		t.Errorf("nil: %v", got)
	}
}

func TestDevVerifier(t *testing.T) {
	p, err := DevVerifier{}.Verify(context.Background(), "alice:admin, ops")
	if err != nil || p.Subject != "alice" || !p.HasRole("admin") || !p.HasRole("ops") {
		t.Fatalf("got %+v, %v", p, err)
	}
	if _, err := (DevVerifier{}).Verify(context.Background(), " "); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty token: %v", err)
	}
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{"Bearer abc": "abc", "bearer  abc ": "abc"} {
		if got, err := BearerToken(header); err != nil || got != want {
			t.Errorf("BearerToken(%q) = %q, %v", header, got, err)
		}
	}
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer   "} {
		if _, err := BearerToken(header); err == nil {
			t.Errorf("BearerToken(%q) accepted", header)
		}
	}
}

func TestFromContext(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty context must be anonymous")
	}
	if _, ok := FromContext(WithPrincipal(context.Background(), Principal{})); ok {
		t.Fatal("principal without subject must be anonymous")
	}
}
