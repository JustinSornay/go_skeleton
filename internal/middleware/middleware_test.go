package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
	mw "github.com/JustinSornay/go_skeleton/internal/middleware"
	"github.com/JustinSornay/go_skeleton/internal/utils/actor"
	"github.com/JustinSornay/go_skeleton/internal/utils/httpx"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
	"github.com/JustinSornay/go_skeleton/internal/utils/requestid"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p httpx.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body %q is not a problem: %v", rec.Body.String(), err)
	}
	return p.Code
}

func TestRequestID(t *testing.T) {
	var seen string
	h := mw.RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = requestid.From(r.Context()) }))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestid.Header, "upstream-123")
	rec := do(h, req)
	if seen != "upstream-123" || rec.Header().Get(requestid.Header) != "upstream-123" {
		t.Errorf("valid incoming id not reused: %q", seen)
	}

	req.Header.Set(requestid.Header, "bad id\r\nX-Injected: 1")
	rec = do(h, req)
	if seen == "" || strings.Contains(seen, " ") || rec.Header().Get(requestid.Header) != seen {
		t.Errorf("invalid incoming id not replaced: %q", seen)
	}
}

func TestClientIP(t *testing.T) {
	clientIP, err := mw.ClientIP([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	h := clientIP(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = mw.ClientIPFromContext(r.Context()) }))

	tests := []struct{ name, remote, xff, want string }{
		{"direct client ignores xff", "203.0.113.9:1234", "1.2.3.4", "203.0.113.9"},
		{"trusted proxy", "10.0.0.2:80", "198.51.100.7", "198.51.100.7"},
		{"spoofed left-most value is ignored", "10.0.0.2:80", "6.6.6.6, 198.51.100.7, 10.0.0.3", "198.51.100.7"},
		{"only proxies", "10.0.0.2:80", "10.0.0.3", "10.0.0.2"},
		{"garbage hop skipped", "10.0.0.2:80", "198.51.100.7, nonsense", "198.51.100.7"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remote
		req.Header.Set("X-Forwarded-For", tt.xff)
		do(h, req)
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}

	if _, err := mw.ClientIP([]string{"not-a-cidr"}); err == nil {
		t.Error("invalid CIDR accepted")
	}
}

func TestRateLimiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientIP, _ := mw.ClientIP(nil)
	h := clientIP(mw.NewRateLimiter(ctx, 1, 2, 1).Middleware(ok))

	req := func(ip string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip + ":1"
		return r
	}
	for i := range 2 {
		if rec := do(h, req("192.0.2.1")); rec.Code != 200 {
			t.Fatalf("request %d within burst: %d", i, rec.Code)
		}
	}
	rec := do(h, req("192.0.2.1"))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || problemCode(t, rec) != "rate_limited" {
		t.Fatalf("over limit: %d %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// maxClients=1: a new IP falls back to the shared overflow bucket
	// instead of being rejected.
	if rec := do(h, req("192.0.2.2")); rec.Code != 200 {
		t.Fatalf("overflow client: %d", rec.Code)
	}
}

type stubVerifier struct{}

func (stubVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token == "good" {
		return auth.Principal{Subject: "u1", Roles: []string{"editor"}}, nil
	}
	return auth.Principal{}, auth.ErrInvalidToken
}

func TestAuthenticate(t *testing.T) {
	var p auth.Principal
	var a actor.Actor
	h := mw.Authenticate(stubVerifier{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ = auth.FromContext(r.Context())
		a = actor.From(r.Context())
	}))

	// Anonymous requests pass through.
	if rec := do(h, httptest.NewRequest(http.MethodGet, "/", nil)); rec.Code != 200 || p.Subject != "" || a != actor.System {
		t.Fatalf("anonymous: %d %+v %+v", rec.Code, p, a)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	if rec := do(h, req); rec.Code != 200 || p.Subject != "u1" || a != actor.User("u1") {
		t.Fatalf("valid token: %d %+v %+v", rec.Code, p, a)
	}

	for _, header := range []string{"Bearer bad", "Basic dXNlcjpwYXNz"} {
		req.Header.Set("Authorization", header)
		rec := do(h, req)
		if rec.Code != 401 || problemCode(t, rec) != "auth.invalid_token" ||
			rec.Header().Get("WWW-Authenticate") != `Bearer error="invalid_token"` {
			t.Errorf("%q: %d %q", header, rec.Code, rec.Header().Get("WWW-Authenticate"))
		}
	}
}

func TestRequireAuthAndRole(t *testing.T) {
	anon := httptest.NewRequest(http.MethodGet, "/", nil)
	editor := anon.WithContext(auth.WithPrincipal(context.Background(), auth.Principal{Subject: "u1", Roles: []string{"editor"}}))

	if rec := do(mw.RequireAuth(ok), anon); rec.Code != 401 || problemCode(t, rec) != "auth.required" {
		t.Errorf("RequireAuth anonymous: %d", rec.Code)
	}
	if rec := do(mw.RequireAuth(ok), editor); rec.Code != 200 {
		t.Errorf("RequireAuth authenticated: %d", rec.Code)
	}
	if rec := do(mw.RequireRole("admin", "editor")(ok), editor); rec.Code != 200 {
		t.Errorf("RequireRole matching: %d", rec.Code)
	}
	if rec := do(mw.RequireRole("admin")(ok), editor); rec.Code != 403 || problemCode(t, rec) != "auth.forbidden" {
		t.Errorf("RequireRole missing: %d", rec.Code)
	}
	if rec := do(mw.RequireRole("admin")(ok), anon); rec.Code != 401 {
		t.Errorf("RequireRole anonymous: %d", rec.Code)
	}
}

func TestRecoverer(t *testing.T) {
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(logging.WithContext(req.Context(), logging.Discard()))
	rec := do(mw.Recoverer(boom), req)
	if rec.Code != 500 || problemCode(t, rec) != "internal" || strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}

	defer func() {
		if r := recover(); !errors.Is(r.(error), http.ErrAbortHandler) {
			t.Fatalf("ErrAbortHandler must be re-panicked, got %v", r)
		}
	}()
	abort := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	do(mw.Recoverer(abort), req)
}

func TestLoggerProvidesRequestScopedLogger(t *testing.T) {
	var buf strings.Builder
	base := logging.New(&buf, logging.Options{Format: "json"})
	h := mw.RequestID(mw.Logger(base, "/livez")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging.FromContext(r.Context()).Info("inside")
		w.WriteHeader(http.StatusTeapot)
	})))

	do(h, httptest.NewRequest(http.MethodGet, "/brew?token=secret", nil))
	out := buf.String()
	if strings.Count(out, `"request_id"`) != 2 || !strings.Contains(out, `"status":418`) {
		t.Errorf("unexpected log output:\n%s", out)
	}
	if strings.Contains(out, "secret") {
		t.Error("query string must never be logged")
	}

	buf.Reset()
	do(h, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if strings.Contains(buf.String(), "http request") {
		t.Error("quiet path logged")
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := do(mw.SecurityHeaders(true)(ok), httptest.NewRequest(http.MethodGet, "/", nil))
	for _, h := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Cache-Control", "Strict-Transport-Security"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("missing %s", h)
		}
	}
	rec = do(mw.SecurityHeaders(false)(ok), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must be opt-in")
	}
}

func TestBasicAuth(t *testing.T) {
	h := mw.BasicAuth("docs", "ops", "s3cret")(ok)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if rec := do(h, req); rec.Code != 401 || rec.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("no credentials: %d", rec.Code)
	}
	req.SetBasicAuth("ops", "wrong")
	if rec := do(h, req); rec.Code != 401 {
		t.Errorf("wrong password: %d", rec.Code)
	}
	req.SetBasicAuth("ops", "s3cret")
	if rec := do(h, req); rec.Code != 200 {
		t.Errorf("valid credentials: %d", rec.Code)
	}
}

func TestBodyLimitAndTimeout(t *testing.T) {
	h := mw.BodyLimit(4)(mw.Timeout(0)(httpx.Handle(func(w http.ResponseWriter, r *http.Request) error {
		if err := r.Context().Err(); err == nil {
			t.Error("Timeout(0) must expire the context")
		}
		_, err := httpx.DecodeJSON[map[string]string](r)
		return err
	})))
	rec := do(h, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"long"}`)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", rec.Code)
	}
}
