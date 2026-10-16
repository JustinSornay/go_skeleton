// Package config loads the application configuration from environment
// variables (twelve-factor). Every field has a typed default declared next to
// it, so .env.example and this file are the only places to look.
//
// Load fails fast: a missing required variable, an unparsable value or an
// unsafe production setting stops the process at startup instead of
// surfacing later as a runtime bug.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Environment names. Behaviour that differs between environments must be
// driven by explicit settings below, never by checking Env directly in
// business code.
const (
	EnvDev        = "dev"
	EnvTest       = "test"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// Auth modes.
const (
	AuthModeJWT = "jwt" // verify signed JWTs against a JWKS endpoint (OIDC, Firebase, Auth0, Keycloak…)
	AuthModeDev = "dev" // the bearer token is the subject itself; forbidden outside dev/test
)

type Config struct {
	App       App       `envPrefix:"APP_"`
	HTTP      HTTP      `envPrefix:"HTTP_"`
	Admin     Admin     `envPrefix:"ADMIN_"`
	Log       Log       `envPrefix:"LOG_"`
	Database  Database  `envPrefix:"DATABASE_"`
	Redis     Redis     `envPrefix:"REDIS_"`
	Auth      Auth      `envPrefix:"AUTH_"`
	CORS      CORS      `envPrefix:"CORS_"`
	RateLimit RateLimit `envPrefix:"RATE_LIMIT_"`
	Docs      Docs      `envPrefix:"DOCS_"`
}

type App struct {
	Name    string `env:"NAME" envDefault:"go-skeleton"`
	Env     string `env:"ENV" envDefault:"dev"`
	Version string `env:"VERSION" envDefault:"dev"`
}

type HTTP struct {
	Addr              string        `env:"ADDR" envDefault:":8080"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"5s"`
	ReadTimeout       time.Duration `env:"READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout      time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout       time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s"`
	// RequestTimeout bounds the context of every request (DB calls, outgoing HTTP…).
	RequestTimeout time.Duration `env:"REQUEST_TIMEOUT" envDefault:"20s"`
	// ShutdownDelay keeps serving while /readyz reports "draining", so load
	// balancers stop routing traffic before connections are closed.
	ShutdownDelay   time.Duration `env:"SHUTDOWN_DELAY" envDefault:"0s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"20s"`
	MaxBodyBytes    int64         `env:"MAX_BODY_BYTES" envDefault:"1048576"`
	// HSTS enables Strict-Transport-Security; only meaningful behind HTTPS.
	HSTS bool `env:"HSTS" envDefault:"false"`
}

// Admin is a separate, non-public listener for /metrics and /debug/pprof.
type Admin struct {
	Addr string `env:"ADDR" envDefault:"127.0.0.1:9090"` // empty disables the admin server
}

type Log struct {
	Level  string `env:"LEVEL" envDefault:"info"`  // debug|info|warn|error
	Format string `env:"FORMAT" envDefault:"json"` // json|text
}

type Database struct {
	URL             string        `env:"URL,required,unset"`
	MaxConns        int32         `env:"MAX_CONNS" envDefault:"10"`
	MinConns        int32         `env:"MIN_CONNS" envDefault:"0"`
	MaxConnLifetime time.Duration `env:"MAX_CONN_LIFETIME" envDefault:"1h"`
	MaxConnIdleTime time.Duration `env:"MAX_CONN_IDLE_TIME" envDefault:"30m"`
	// AutoMigrate applies pending migrations at startup. Convenient in dev;
	// in production run `migrate up` as a separate release step instead.
	AutoMigrate bool `env:"AUTO_MIGRATE" envDefault:"false"`
}

type Redis struct {
	URL string `env:"URL,unset"` // optional; empty falls back to an in-memory cache
}

type Auth struct {
	Mode     string `env:"MODE" envDefault:"jwt"`
	JWKSURL  string `env:"JWKS_URL"`
	Issuer   string `env:"ISSUER"`
	Audience string `env:"AUDIENCE"`
	// RolesClaim is the top-level claim holding the user's roles
	// (array of strings or space-separated string).
	RolesClaim string `env:"ROLES_CLAIM" envDefault:"roles"`
}

type CORS struct {
	AllowedOrigins []string `env:"ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
}

type RateLimit struct {
	// RPS is the sustained per-client request rate; 0 disables the limiter.
	RPS        float64 `env:"RPS" envDefault:"10"`
	Burst      int     `env:"BURST" envDefault:"30"`
	MaxClients int     `env:"MAX_CLIENTS" envDefault:"100000"`
	// TrustedProxies lists the CIDRs of reverse proxies whose X-Forwarded-For
	// header is honoured. Empty means X-Forwarded-For is ignored.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`
}

type Docs struct {
	Enabled  bool   `env:"ENABLED" envDefault:"true"`
	User     string `env:"USER"`
	Password string `env:"PASSWORD,unset"`
}

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(env.ToMap(os.Environ()))
}

// LoadFrom reads the configuration from the given variables. Tests use it to
// avoid touching the real environment.
func LoadFrom(vars map[string]string) (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: vars}); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	cfg.App.Env = strings.ToLower(strings.TrimSpace(cfg.App.Env))
	cfg.Auth.Mode = strings.ToLower(strings.TrimSpace(cfg.Auth.Mode))
	cfg.CORS.AllowedOrigins = compact(cfg.CORS.AllowedOrigins)
	cfg.RateLimit.TrustedProxies = compact(cfg.RateLimit.TrustedProxies)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

// IsProduction reports whether production safety rules apply.
func (c Config) IsProduction() bool { return c.App.Env == EnvProduction }

// Validate checks consistency and enforces production safety gates.
func (c Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if !slices.Contains([]string{EnvDev, EnvTest, EnvStaging, EnvProduction}, c.App.Env) {
		add("APP_ENV must be one of dev, test, staging, production (got %q)", c.App.Env)
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, strings.ToLower(c.Log.Level)) {
		add("LOG_LEVEL must be one of debug, info, warn, error (got %q)", c.Log.Level)
	}
	if c.Log.Format != "json" && c.Log.Format != "text" {
		add("LOG_FORMAT must be json or text (got %q)", c.Log.Format)
	}
	if c.HTTP.MaxBodyBytes <= 0 {
		add("HTTP_MAX_BODY_BYTES must be positive")
	}
	if c.Database.MaxConns <= 0 || c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		add("DATABASE_MIN_CONNS/DATABASE_MAX_CONNS are inconsistent")
	}
	for _, cidr := range c.RateLimit.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			add("RATE_LIMIT_TRUSTED_PROXIES: invalid CIDR %q", cidr)
		}
	}

	switch c.Auth.Mode {
	case AuthModeJWT:
		if c.Auth.JWKSURL == "" || c.Auth.Issuer == "" || c.Auth.Audience == "" {
			add("AUTH_MODE=jwt requires AUTH_JWKS_URL, AUTH_ISSUER and AUTH_AUDIENCE")
		} else if u, err := url.Parse(c.Auth.JWKSURL); err != nil || u.Host == "" {
			add("AUTH_JWKS_URL is not a valid URL")
		}
	case AuthModeDev:
		// Two independent conditions must hold to bypass token verification:
		// the dev mode itself AND a non-shared environment.
		if c.App.Env != EnvDev && c.App.Env != EnvTest {
			add("AUTH_MODE=dev is only allowed with APP_ENV=dev or APP_ENV=test")
		}
	default:
		add("AUTH_MODE must be jwt or dev (got %q)", c.Auth.Mode)
	}

	if c.IsProduction() {
		if slices.Contains(c.CORS.AllowedOrigins, "*") {
			add("CORS_ALLOWED_ORIGINS must not contain '*' in production")
		}
		if c.Docs.Enabled && (c.Docs.User == "" || c.Docs.Password == "") {
			add("DOCS_USER and DOCS_PASSWORD are required when DOCS_ENABLED=true in production")
		}
		if c.Database.AutoMigrate {
			add("DATABASE_AUTO_MIGRATE must be false in production (run `migrate up` as a release step)")
		}
		if c.Auth.Mode == AuthModeJWT && !strings.HasPrefix(c.Auth.JWKSURL, "https://") {
			add("AUTH_JWKS_URL must use https in production")
		}
	}
	return errors.Join(errs...)
}

func compact(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// LoadDatabase reads only the DATABASE_* variables. Batch tools (migrate,
// retention) use it so they do not require the API's auth/HTTP settings.
func LoadDatabase() (Database, error) {
	db, err := env.ParseAsWithOptions[Database](env.Options{Prefix: "DATABASE_", Environment: env.ToMap(os.Environ())})
	if err != nil {
		return Database{}, fmt.Errorf("config: %w", err)
	}
	return db, nil
}
