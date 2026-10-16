// Package metrics exposes Prometheus metrics. The registry is served on the
// admin listener, never on the public API port.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is a Prometheus registry pre-loaded with Go runtime and process
// collectors.
type Registry struct {
	*prometheus.Registry
}

// NewRegistry returns a registry with the default runtime collectors.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Registry{reg}
}

// Handler serves the registry in the Prometheus exposition format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.Registry, promhttp.HandlerOpts{Registry: r.Registry})
}

// HTTP records RED metrics (rate, errors, duration) per route.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewHTTP registers the HTTP metrics on reg.
func NewHTTP(reg *Registry) *HTTP {
	m := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

// Middleware observes every request. Routes are labelled with their chi
// pattern ("/v1/projects/{id}"), never the raw path, to keep cardinality low.
func (m *HTTP) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}
