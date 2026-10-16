package middleware

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/JustinSornay/go_skeleton/internal/platform/apperr"
	"github.com/JustinSornay/go_skeleton/internal/platform/httpx"
)

var errRateLimited = apperr.New(apperr.KindRateLimited, "rate_limited", "too many requests, retry later")

// RateLimiter is an in-memory token bucket per client IP (see ClientIP).
//
// It protects a single instance against bursts and scripts. Limits are per
// process: with N replicas a client may get up to N times the rate. Use a
// shared store (e.g. Redis) when an exact global limit is required.
type RateLimiter struct {
	rps        rate.Limit
	burst      int
	maxClients int
	idleTTL    time.Duration

	mu       sync.Mutex
	clients  map[string]*client
	overflow *rate.Limiter // shared bucket once maxClients is reached
}

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter starts a janitor goroutine that evicts idle clients until
// ctx is cancelled.
func NewRateLimiter(ctx context.Context, rps float64, burst, maxClients int) *RateLimiter {
	rl := &RateLimiter{
		rps:        rate.Limit(rps),
		burst:      burst,
		maxClients: maxClients,
		idleTTL:    3 * time.Minute,
		clients:    make(map[string]*client),
		// New clients beyond capacity share one bucket instead of being
		// rejected outright, so an IP-rotation flood cannot lock everyone out.
		overflow: rate.NewLimiter(rate.Limit(rps), burst),
	}
	go rl.janitor(ctx)
	return rl
}

// Middleware rejects requests over the limit with 429 and Retry-After.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lim := rl.limiter(ClientIPFromContext(r.Context()))
		res := lim.Reserve()
		if !res.OK() {
			// Burst of 0: the limiter can never grant a token.
			w.Header().Set("Retry-After", "60")
			httpx.WriteError(w, r, errRateLimited)
			return
		}
		if delay := res.Delay(); delay > 0 {
			res.Cancel()
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(delay.Seconds()))))
			httpx.WriteError(w, r, errRateLimited)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) limiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if c, ok := rl.clients[key]; ok {
		c.lastSeen = time.Now()
		return c.limiter
	}
	if len(rl.clients) >= rl.maxClients {
		return rl.overflow
	}
	c := &client{limiter: rate.NewLimiter(rl.rps, rl.burst), lastSeen: time.Now()}
	rl.clients[key] = c
	return c.limiter
}

func (rl *RateLimiter) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			rl.evictIdle(now)
		}
	}
}

func (rl *RateLimiter) evictIdle(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for k, c := range rl.clients {
		if now.Sub(c.lastSeen) > rl.idleTTL {
			delete(rl.clients, k)
		}
	}
}
