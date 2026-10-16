// Package server runs HTTP servers with production timeouts and a graceful
// shutdown sequence.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// Options holds the server timeouts.
type Options struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// New returns an http.Server with every timeout set: the zero values of
// http.Server mean "no timeout" and expose the process to slow-client attacks.
func New(addr string, h http.Handler, o Options, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: o.ReadHeaderTimeout,
		ReadTimeout:       o.ReadTimeout,
		WriteTimeout:      o.WriteTimeout,
		IdleTimeout:       o.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// Shutdown configures the graceful stop.
type Shutdown struct {
	// Delay keeps serving after the stop signal while readiness reports
	// "draining", giving load balancers time to deregister the instance.
	Delay time.Duration
	// Timeout bounds the wait for in-flight requests.
	Timeout time.Duration
	// OnDrain is called first (e.g. health.Checker.SetDraining).
	OnDrain func()
}

// Run serves on every server until ctx is cancelled or one of them fails,
// then shuts them all down gracefully.
func Run(ctx context.Context, log *slog.Logger, sd Shutdown, servers ...*http.Server) error {
	// Bind every port before serving anything, so a busy port fails fast
	// without leaving another server running.
	var lc net.ListenConfig
	listeners := make([]net.Listener, 0, len(servers))
	for _, srv := range servers {
		ln, err := lc.Listen(ctx, "tcp", srv.Addr)
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return fmt.Errorf("server: listen %s: %w", srv.Addr, err)
		}
		listeners = append(listeners, ln)
	}

	g, gctx := errgroup.WithContext(ctx)
	for i, srv := range servers {
		ln := listeners[i]
		log.Info("listening", slog.String("addr", ln.Addr().String()))
		g.Go(func() error {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server %s: %w", srv.Addr, err)
			}
			return nil
		})
	}

	g.Go(func() error {
		<-gctx.Done()
		log.Info("shutting down", slog.Duration("drain_delay", sd.Delay))
		if sd.OnDrain != nil {
			sd.OnDrain()
		}
		if sd.Delay > 0 {
			time.Sleep(sd.Delay)
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(gctx), sd.Timeout)
		defer cancel()
		var errs []error
		for _, srv := range servers {
			if err := srv.Shutdown(sctx); err != nil {
				errs = append(errs, fmt.Errorf("server %s: shutdown: %w", srv.Addr, err))
			}
		}
		return errors.Join(errs...)
	})
	return g.Wait()
}
