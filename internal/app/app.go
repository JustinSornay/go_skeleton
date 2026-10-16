// Package app is the composition root: it opens the infrastructure, wires
// every module with plain constructor injection (no container, no globals)
// and runs the servers. It is the only package that knows every other one.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/config"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/auth"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/cache"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/health"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/metrics"
	"github.com/JustinSornay/go_skeleton/internal/infrastructure/server"
	mw "github.com/JustinSornay/go_skeleton/internal/middleware"
	"github.com/JustinSornay/go_skeleton/migrations"
)

// App holds the running application.
type App struct {
	cfg     config.Config
	log     *slog.Logger
	health  *health.Checker
	api     http.Handler
	admin   http.Handler
	stop    context.CancelFunc // stops background goroutines (JWKS refresh, limiter janitor)
	closers []func()
}

// New opens the infrastructure and builds the handlers. Call Close when done,
// even if Run was not called.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (_ *App, err error) {
	bg, stop := context.WithCancel(context.WithoutCancel(ctx))
	a := &App{cfg: cfg, log: log, stop: stop}
	defer func() {
		if err != nil {
			a.Close()
		}
	}()

	db, err := database.Open(ctx, database.Options{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	})
	if err != nil {
		return nil, err
	}
	a.closers = append(a.closers, db.Close)

	if cfg.Database.AutoMigrate {
		if err := migrateUp(ctx, db, log); err != nil {
			return nil, err
		}
	}

	a.health = health.New(2 * time.Second)
	a.health.Add("postgres", db.Ping)

	var store cache.Store = cache.NewMemory()
	if cfg.Redis.URL != "" {
		rs, err := cache.OpenRedis(ctx, cfg.Redis.URL, cfg.App.Name+":")
		if err != nil {
			return nil, err
		}
		a.closers = append(a.closers, func() { _ = rs.Close() })
		a.health.Add("redis", rs.Ping)
		store = rs
	}

	verifier, err := newVerifier(bg, cfg.Auth)
	if err != nil {
		return nil, err
	}

	reg := metrics.NewRegistry()
	var limiter *mw.RateLimiter
	if cfg.RateLimit.RPS > 0 {
		limiter = mw.NewRateLimiter(bg, cfg.RateLimit.RPS, cfg.RateLimit.Burst, cfg.RateLimit.MaxClients)
	}

	router, err := NewRouter(Deps{
		Config:   cfg,
		Logger:   log,
		DB:       db,
		Cache:    store,
		Verifier: verifier,
		Health:   a.health,
		Metrics:  metrics.NewHTTP(reg),
		Limiter:  limiter,
	})
	if err != nil {
		return nil, err
	}
	a.api = router
	a.admin = adminHandler(reg)
	return a, nil
}

// Handler returns the public HTTP handler (tests).
func (a *App) Handler() http.Handler { return a.api }

// Run serves the API (and the admin endpoints when enabled) until ctx is
// cancelled, then drains and shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	opts := server.Options{
		ReadHeaderTimeout: a.cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       a.cfg.HTTP.ReadTimeout,
		WriteTimeout:      a.cfg.HTTP.WriteTimeout,
		IdleTimeout:       a.cfg.HTTP.IdleTimeout,
	}
	servers := []*http.Server{server.New(a.cfg.HTTP.Addr, a.api, opts, a.log)}
	if a.cfg.Admin.Addr != "" {
		servers = append(servers, server.New(a.cfg.Admin.Addr, a.admin, opts, a.log))
	}
	return server.Run(ctx, a.log, server.Shutdown{
		Delay:   a.cfg.HTTP.ShutdownDelay,
		Timeout: a.cfg.HTTP.ShutdownTimeout,
		OnDrain: a.health.SetDraining,
	}, servers...)
}

// Close releases resources in reverse order of acquisition.
func (a *App) Close() {
	a.stop()
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

func newVerifier(ctx context.Context, cfg config.Auth) (auth.Verifier, error) {
	switch cfg.Mode {
	case config.AuthModeDev:
		return auth.DevVerifier{}, nil
	case config.AuthModeJWT:
		return auth.NewJWTVerifier(ctx, auth.JWTConfig{
			JWKSURL:    cfg.JWKSURL,
			Issuer:     cfg.Issuer,
			Audience:   cfg.Audience,
			RolesClaim: cfg.RolesClaim,
		})
	default:
		return nil, fmt.Errorf("app: unsupported auth mode %q", cfg.Mode)
	}
}

func migrateUp(ctx context.Context, db *database.DB, log *slog.Logger) error {
	m, err := database.NewMigrator(db, migrations.FS)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	n, err := m.Up(ctx)
	if err != nil {
		return fmt.Errorf("app: applying migrations: %w", err)
	}
	log.Info("migrations applied", slog.Int("count", n))
	return nil
}

// adminHandler serves operational endpoints on the private admin listener.
func adminHandler(reg *metrics.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", reg.Handler())
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}
