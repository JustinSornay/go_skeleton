package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/platform/config"
)

func baseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
		"AUTH_MODE":    "dev",
	}
}

func with(kv ...string) map[string]string {
	env := baseEnv()
	for i := 0; i < len(kv); i += 2 {
		env[kv[i]] = kv[i+1]
	}
	return env
}

func TestDefaults(t *testing.T) {
	cfg, err := config.LoadFrom(baseEnv())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Env != config.EnvDev || cfg.HTTP.Addr != ":8080" || cfg.HTTP.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.RateLimit.RPS != 10 || cfg.Database.MaxConns != 10 || !cfg.Docs.Enabled {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestListsAreTrimmed(t *testing.T) {
	cfg, err := config.LoadFrom(with(
		"CORS_ALLOWED_ORIGINS", " https://a.example , ,https://b.example",
		"RATE_LIMIT_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.0.0/16",
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.CORS.AllowedOrigins, "|"); got != "https://a.example|https://b.example" {
		t.Errorf("origins = %q", got)
	}
	if len(cfg.RateLimit.TrustedProxies) != 2 {
		t.Errorf("proxies = %v", cfg.RateLimit.TrustedProxies)
	}
}

func TestInvalid(t *testing.T) {
	prod := []string{
		"APP_ENV", "production", "AUTH_MODE", "jwt",
		"AUTH_JWKS_URL", "https://idp.example/jwks", "AUTH_ISSUER", "https://idp.example", "AUTH_AUDIENCE", "api",
		"DOCS_ENABLED", "false",
	}
	tests := map[string]struct {
		env  map[string]string
		want string
	}{
		"missing database":       {map[string]string{"AUTH_MODE": "dev"}, "DATABASE_URL"},
		"bad duration":           {with("HTTP_READ_TIMEOUT", "soon"), "invalid duration"},
		"unknown env":            {with("APP_ENV", "prod"), "APP_ENV"},
		"bad log level":          {with("LOG_LEVEL", "verbose"), "LOG_LEVEL"},
		"bad proxy cidr":         {with("RATE_LIMIT_TRUSTED_PROXIES", "10.0.0.1"), "invalid CIDR"},
		"jwt without settings":   {with("AUTH_MODE", "jwt"), "AUTH_JWKS_URL"},
		"dev auth in staging":    {with("APP_ENV", "staging"), "AUTH_MODE=dev"},
		"cors wildcard in prod":  {with(append(prod, "CORS_ALLOWED_ORIGINS", "*")...), "CORS_ALLOWED_ORIGINS"},
		"open docs in prod":      {with(append(prod, "DOCS_ENABLED", "true")...), "DOCS_USER"},
		"auto-migrate in prod":   {with(append(prod, "DATABASE_AUTO_MIGRATE", "true")...), "DATABASE_AUTO_MIGRATE"},
		"plain http jwks prod":   {with(append(prod, "AUTH_JWKS_URL", "http://idp.example/jwks")...), "https"},
		"min conns > max conns":  {with("DATABASE_MIN_CONNS", "20"), "DATABASE_MIN_CONNS"},
		"non-positive body size": {with("HTTP_MAX_BODY_BYTES", "0"), "HTTP_MAX_BODY_BYTES"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := config.LoadFrom(tt.env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestValidProduction(t *testing.T) {
	_, err := config.LoadFrom(with(
		"APP_ENV", "production", "AUTH_MODE", "jwt",
		"AUTH_JWKS_URL", "https://idp.example/jwks", "AUTH_ISSUER", "https://idp.example", "AUTH_AUDIENCE", "api",
		"DOCS_USER", "ops", "DOCS_PASSWORD", "s3cret",
		"CORS_ALLOWED_ORIGINS", "https://app.example",
	))
	if err != nil {
		t.Fatal(err)
	}
}
