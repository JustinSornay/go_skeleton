package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/health"
)

func ready(t *testing.T, c *health.Checker) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	c.Ready(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func TestReadiness(t *testing.T) {
	c := health.New(50 * time.Millisecond)
	c.Add("db", func(context.Context) error { return nil })
	if code, body := ready(t, c); code != 200 || body["status"] != "ok" {
		t.Fatalf("healthy: %d %v", code, body)
	}

	c.Add("slow", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	c.Add("broken", func(context.Context) error { return errors.New("connection refused to 10.0.0.5") })
	code, body := ready(t, c)
	checks := body["checks"].(map[string]any)
	if code != 503 || checks["db"] != "ok" || checks["slow"] != "fail" || checks["broken"] != "fail" {
		t.Fatalf("unhealthy: %d %v", code, body)
	}
}

func TestDrainingAndLiveness(t *testing.T) {
	c := health.New(time.Second)
	c.SetDraining()
	if code, body := ready(t, c); code != 503 || body["status"] != "draining" {
		t.Fatalf("draining: %d %v", code, body)
	}
	rec := httptest.NewRecorder()
	c.Live(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != 200 {
		t.Fatalf("liveness must not depend on readiness: %d", rec.Code)
	}
}
