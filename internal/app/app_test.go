//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/app"
	"github.com/JustinSornay/go_skeleton/internal/platform/auth"
	"github.com/JustinSornay/go_skeleton/internal/platform/cache"
	"github.com/JustinSornay/go_skeleton/internal/platform/database/dbtest"
	"github.com/JustinSornay/go_skeleton/internal/platform/health"
	"github.com/JustinSornay/go_skeleton/internal/platform/logging"
	"github.com/JustinSornay/go_skeleton/internal/platform/requestid"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// TestEndToEnd drives the real router (every middleware, real PostgreSQL)
// through the public HTTP interface.
func TestEndToEnd(t *testing.T) {
	db := dbtest.New(t)
	h := health.New(time.Second)
	h.Add("postgres", db.Ping)
	router, err := app.NewRouter(app.Deps{
		Config:   testConfig(t),
		Logger:   logging.Discard(),
		DB:       db,
		Cache:    cache.NewMemory(),
		Verifier: auth.DevVerifier{},
		Health:   h,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(router)
	defer srv.Close()

	call := func(method, path, token, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequestWithContext(context.Background(), method, srv.URL+path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set(requestid.Header, "e2e-"+method)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	if resp, body := call(http.MethodGet, "/readyz", "", ""); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"postgres":"ok"`) {
		t.Fatalf("readyz: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(http.MethodGet, "/v1/projects", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", resp.StatusCode)
	}

	resp, body := call(http.MethodPost, "/v1/projects", "alice", `{"name":"Orchard"}`)
	if resp.StatusCode != http.StatusCreated || resp.Header.Get(requestid.Header) != "e2e-POST" {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created struct{ ID string }
	_ = json.Unmarshal([]byte(body), &created)

	if resp, body := call(http.MethodPost, "/v1/projects", "alice", `{"name":"orchard"}`); resp.StatusCode != http.StatusConflict || !strings.Contains(body, `"request_id":"e2e-POST"`) {
		t.Fatalf("duplicate: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(http.MethodGet, "/v1/projects/"+created.ID, "bob", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other owner: %d", resp.StatusCode)
	}
	if resp, _ := call(http.MethodDelete, "/v1/projects/"+created.ID, "alice", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}

	// The audit trail records who did what, with the request ID.
	rows, err := db.Q(context.Background()).Query(context.Background(),
		`SELECT actor_id || ' ' || action || ' ' || request_id FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var trail []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		trail = append(trail, s)
	}
	want := []string{"alice project.create e2e-POST", "alice project.delete e2e-DELETE"}
	if strings.Join(trail, "|") != strings.Join(want, "|") {
		t.Fatalf("audit trail = %v, want %v", trail, want)
	}
}
