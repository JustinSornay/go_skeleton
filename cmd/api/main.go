// Command api runs the HTTP API.
//
// main only translates the process environment (signals, env vars, exit
// code) into a call to run, which is testable. All wiring lives in
// internal/app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JustinSornay/go_skeleton/internal/app"
	"github.com/JustinSornay/go_skeleton/internal/config"
	"github.com/JustinSornay/go_skeleton/internal/utils/logging"
)

// version is set at build time: -ldflags "-X main.version=$(git describe)".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	healthcheck := fs.Bool("healthcheck", false, "probe /livez on HTTP_ADDR and exit (container HEALTHCHECK)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *healthcheck {
		return probe(ctx, cfg.HTTP.Addr)
	}
	if cfg.App.Version == "dev" {
		cfg.App.Version = version
	}

	log := logging.New(stdout, logging.Options{
		Level: cfg.Log.Level, Format: cfg.Log.Format,
		Service: cfg.App.Name, Version: cfg.App.Version, Env: cfg.App.Env,
	})
	log.Info("starting")

	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	if err := a.Run(ctx); err != nil {
		return err
	}
	log.Info("stopped")
	return nil
}

// probe lets distroless images (no shell, no curl) run a health check.
func probe(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/livez", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("unhealthy: " + resp.Status)
	}
	return nil
}
