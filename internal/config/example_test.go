package config_test

import (
	"fmt"

	"github.com/JustinSornay/go_skeleton/internal/config"
)

// ExampleLoadFrom loads the configuration from a map instead of the process
// environment: unset variables take the defaults declared in config.go and
// lists are split on commas and trimmed.
func ExampleLoadFrom() {
	cfg, err := config.LoadFrom(map[string]string{
		"DATABASE_URL":         "postgres://app:app@localhost:5432/app",
		"AUTH_MODE":            "dev",
		"HTTP_ADDR":            ":9000",
		"CORS_ALLOWED_ORIGINS": "https://app.example.com, https://admin.example.com",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.App.Env, cfg.HTTP.Addr, cfg.HTTP.RequestTimeout, cfg.IsProduction())
	fmt.Println(cfg.CORS.AllowedOrigins)
	// Output:
	// dev :9000 20s false
	// [https://app.example.com https://admin.example.com]
}

// ExampleLoadFrom_productionSafety shows the fail-fast behaviour: every
// unsafe production setting is reported at once and nothing starts.
func ExampleLoadFrom_productionSafety() {
	_, err := config.LoadFrom(map[string]string{
		"DATABASE_URL": "postgres://app:app@db:5432/app",
		"APP_ENV":      "production",
		"AUTH_MODE":    "dev",
	})
	fmt.Println(err)
	// Output:
	// config: AUTH_MODE=dev is only allowed with APP_ENV=dev or APP_ENV=test
	// DOCS_USER and DOCS_PASSWORD are required when DOCS_ENABLED=true in production
}
