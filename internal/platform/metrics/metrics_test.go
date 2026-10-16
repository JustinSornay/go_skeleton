package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/JustinSornay/go_skeleton/internal/platform/metrics"
)

func TestMiddlewareLabelsByRoutePattern(t *testing.T) {
	reg := metrics.NewRegistry()
	m := metrics.NewHTTP(reg)

	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/things/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })

	for _, path := range []string{"/things/1", "/things/2", "/nope"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`http_requests_total{method="GET",route="/things/{id}",status="202"} 2`,
		`http_requests_total{method="GET",route="unmatched",status="404"} 1`,
		`go_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output lacks %q", want)
		}
	}
}
