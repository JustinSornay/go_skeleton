package app

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/JustinSornay/go_skeleton/api"
	"github.com/JustinSornay/go_skeleton/internal/platform/audit"
	"github.com/JustinSornay/go_skeleton/internal/platform/auth"
	"github.com/JustinSornay/go_skeleton/internal/platform/cache"
	"github.com/JustinSornay/go_skeleton/internal/platform/config"
	"github.com/JustinSornay/go_skeleton/internal/platform/database"
	"github.com/JustinSornay/go_skeleton/internal/platform/health"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
	"github.com/JustinSornay/go_skeleton/internal/platform/metrics"
	mw "github.com/JustinSornay/go_skeleton/internal/platform/middleware"
	"github.com/JustinSornay/go_skeleton/internal/project"
	projecthttp "github.com/JustinSornay/go_skeleton/internal/project/httpapi"
	projectpg "github.com/JustinSornay/go_skeleton/internal/project/postgres"
)

// Deps are the infrastructure handles the router is built from. Tests build
// them directly (e.g. with a throwaway database and auth.DevVerifier).
type Deps struct {
	Config   config.Config
	Logger   *slog.Logger
	DB       *database.DB
	Cache    cache.Store // shared cache for modules that need one (see docs/architecture.md)
	Verifier auth.Verifier
	Health   *health.Checker
	Metrics  *metrics.HTTP   // optional
	Limiter  *mw.RateLimiter // optional; nil disables rate limiting
}

// NewRouter wires every module and returns the public HTTP handler.
//
// Middleware order matters (outermost first):
//
//	RequestID → ClientIP → Logger → Metrics → Recoverer → SecurityHeaders → GetHead
//	  ├─ /livez, /readyz           (no rate limit, no auth: probes)
//	  └─ CORS → RateLimit → BodyLimit → Timeout → Authenticate
//	       ├─ /openapi.yaml, /docs (basic auth when configured)
//	       └─ /v1/...              (RequireAuth per group)
//
// Logger wraps Recoverer so a panic is logged as a 500, and RequestID comes
// first so every log line and problem response carries the ID.
func NewRouter(d Deps) (*chi.Mux, error) {
	cfg := d.Config
	clientIP, err := mw.ClientIP(cfg.RateLimit.TrustedProxies)
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.MethodNotAllowed(httpx.MethodNotAllowed)

	r.Use(mw.RequestID, clientIP, mw.Logger(d.Logger, "/livez", "/readyz"))
	if d.Metrics != nil {
		r.Use(d.Metrics.Middleware)
	}
	r.Use(mw.Recoverer, mw.SecurityHeaders(cfg.HTTP.HSTS), chimw.GetHead)

	r.Get("/livez", d.Health.Live)
	r.Get("/readyz", d.Health.Ready)

	r.Group(func(r chi.Router) {
		r.Use(mw.CORS(cfg.CORS.AllowedOrigins))
		if d.Limiter != nil {
			r.Use(d.Limiter.Middleware)
		}
		r.Use(
			mw.BodyLimit(cfg.HTTP.MaxBodyBytes),
			mw.Timeout(cfg.HTTP.RequestTimeout),
			mw.Authenticate(d.Verifier),
		)

		if cfg.Docs.Enabled {
			r.Group(func(r chi.Router) {
				if cfg.Docs.User != "" {
					r.Use(mw.BasicAuth("api-docs", cfg.Docs.User, cfg.Docs.Password))
				}
				r.Get("/openapi.yaml", api.SpecHandler)
				r.Get("/docs", api.DocsHandler)
				r.Get("/docs/init.js", api.DocsHandler)
			})
		}

		r.Route("/v1", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireAuth)
				r.Mount("/projects", projectModule(d).Routes())
			})
		})
	})
	return r, nil
}

// One constructor per module keeps NewRouter readable as modules grow.
func projectModule(d Deps) *projecthttp.Handler {
	repo := projectpg.NewRepository(d.DB)
	svc := project.NewService(repo, d.DB, audit.NewPostgresRecorder(d.DB))
	return projecthttp.NewHandler(svc)
}
