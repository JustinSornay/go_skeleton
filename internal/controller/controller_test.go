package controller_test

// Helpers shared by every controller test of the package. Reuse them in a new
// <entity>_controller_test.go; only the <entity>-prefixed helpers belong to
// one feature.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
)

// mount serves routes under prefix like internal/app does, with a fake
// authentication that trusts the X-User header (tests only).
func mount(t *testing.T, prefix string, routes http.Handler) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if u := r.Header.Get("X-User"); u != "" {
				r = r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{Subject: u}))
			}
			next.ServeHTTP(w, r)
		})
	})
	r.Mount(prefix, routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// client sends requests to a test server as user (anonymous when empty).
type client struct {
	t    *testing.T
	base string
	user string
}

func (c client) do(method, path, body string) (*http.Response, []byte) {
	c.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if c.user != "" {
		req.Header.Set("X-User", c.user)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp, b
}

// decode unmarshals a JSON response body into a T.
func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decoding %s: %v", b, err)
	}
	return v
}
