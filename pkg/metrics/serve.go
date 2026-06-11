package metrics

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AddrFromEnv returns the metrics listen address from METRICS_ADDR
// (default ":9100"). The listener must stay inside the docker network —
// never publish this port on the host (docs/metrics-design.md §8).
func AddrFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("METRICS_ADDR")); v != "" {
		return v
	}
	return ":9100"
}

// Serve runs the internal observability listener until ctx is cancelled:
//
//	/metrics — Prometheus exposition for the service registry
//	/healthz — liveness, always 200
//	/readyz  — readiness; 503 while ready() returns an error (nil ready → 200)
//
// Blocks; run in a goroutine. Returns nil on graceful shutdown.
func (m *Metrics) Serve(ctx context.Context, addr string, ready func(context.Context) error) error {
	return ServeRegistry(ctx, addr, m.registry, ready)
}

// ServeRegistry is Serve for callers that maintain their own registry
// (api-gateway keeps its HTTP RED metrics in a service-local package).
func ServeRegistry(ctx context.Context, addr string, reg *prometheus.Registry, ready func(context.Context) error) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler(reg, ready),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func handler(reg *prometheus.Registry, ready func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			if err := ready(r.Context()); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
