// Package health serves liveness and readiness probes.
//
//   - /livez answers 200 as long as the process can serve HTTP. It never
//     checks dependencies: restarting the API would not fix the database.
//   - /readyz runs every registered check in parallel and answers 503 when
//     one fails or when the server is draining before shutdown, so the load
//     balancer stops sending traffic.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// CheckFunc reports whether a dependency is usable.
type CheckFunc func(ctx context.Context) error

// Checker aggregates readiness checks.
type Checker struct {
	timeout  time.Duration
	draining atomic.Bool

	mu     sync.RWMutex
	checks map[string]CheckFunc
}

// New returns a Checker whose checks share the given timeout.
func New(timeout time.Duration) *Checker {
	return &Checker{timeout: timeout, checks: make(map[string]CheckFunc)}
}

// Add registers a named check.
func (c *Checker) Add(name string, fn CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[name] = fn
}

// SetDraining makes readiness fail from now on (graceful shutdown).
func (c *Checker) SetDraining() { c.draining.Store(true) }

type report struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Live is the liveness handler.
func (c *Checker) Live(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, report{Status: "ok"})
}

// Ready is the readiness handler.
func (c *Checker) Ready(w http.ResponseWriter, r *http.Request) {
	if c.draining.Load() {
		write(w, http.StatusServiceUnavailable, report{Status: "draining"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), c.timeout)
	defer cancel()

	c.mu.RLock()
	checks := make(map[string]CheckFunc, len(c.checks))
	for k, v := range c.checks {
		checks[k] = v
	}
	c.mu.RUnlock()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make(map[string]string, len(checks))
		failed  bool
	)
	for name, fn := range checks {
		wg.Go(func() {
			status := "ok"
			if err := fn(ctx); err != nil {
				status = "fail" // error details stay in logs: probes may be public
			}
			mu.Lock()
			defer mu.Unlock()
			results[name] = status
			failed = failed || status != "ok"
		})
	}
	wg.Wait()

	if failed {
		write(w, http.StatusServiceUnavailable, report{Status: "fail", Checks: results})
		return
	}
	write(w, http.StatusOK, report{Status: "ok", Checks: results})
}

func write(w http.ResponseWriter, status int, rep report) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(rep)
}
