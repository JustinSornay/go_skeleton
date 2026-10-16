package app_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.yaml.in/yaml/v3"

	"github.com/JustinSornay/go_skeleton/api"
	"github.com/JustinSornay/go_skeleton/internal/app"
	"github.com/JustinSornay/go_skeleton/internal/config"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/health"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.LoadFrom(map[string]string{
		"DATABASE_URL": "postgres://unused@localhost/unused",
		"AUTH_MODE":    "dev",
		"LOG_LEVEL":    "error",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// undocumented lists routes deliberately absent from the OpenAPI document.
var undocumented = []string{"GET /openapi.yaml", "GET /docs", "GET /docs/init.js"}

// TestRoutesMatchOpenAPI fails when a route is added without documenting it
// in api/openapi.yaml, or when the document describes a route that does not
// exist. It keeps the hand-written contract honest at zero tooling cost.
func TestRoutesMatchOpenAPI(t *testing.T) {
	router, err := app.NewRouter(app.Deps{
		Config:   testConfig(t),
		Logger:   logging.Discard(),
		Verifier: auth.DevVerifier{},
		Health:   health.New(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}

	var routes []string
	err = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(strings.ReplaceAll(route, "/*/", "/"), "/")
		if route == "" {
			route = "/"
		}
		if method == http.MethodHead || slices.Contains(undocumented, method+" "+route) {
			return nil
		}
		routes = append(routes, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(api.Spec(), &doc); err != nil {
		t.Fatalf("api/openapi.yaml: %v", err)
	}
	var documented []string
	for path, item := range doc.Paths {
		for method := range item {
			if method == "parameters" || method == "summary" || method == "description" {
				continue
			}
			documented = append(documented, strings.ToUpper(method)+" "+path)
		}
	}

	slices.Sort(routes)
	slices.Sort(documented)
	for _, r := range routes {
		if !slices.Contains(documented, r) {
			t.Errorf("route %q is not documented in api/openapi.yaml", r)
		}
	}
	for _, d := range documented {
		if !slices.Contains(routes, d) {
			t.Errorf("api/openapi.yaml documents %q but no such route exists", d)
		}
	}
}
