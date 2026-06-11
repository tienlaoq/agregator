// Package metrics provides per-service Prometheus instrumentation:
// RED metrics for gRPC servers, NATS consumer counters, pgxpool stats and an
// internal HTTP listener serving /metrics, /healthz and /readyz.
//
// Label cardinality contract: labels never carry user/venue/booking IDs or raw
// request paths — only finite sets (gRPC method, gRPC code, NATS subject,
// result). Per-entity analytics belong to the SQL layer, not Prometheus
// (docs/metrics-design.md §8).
package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// NATS handler results accepted by ObserveNATS.
const (
	NATSOk    = "ok"
	NATSError = "error"
	NATSDLQ   = "dlq"
)

// Metrics holds a dedicated (non-global) registry and the standard instrument
// set for one service. Create once in main with New and share by reference.
type Metrics struct {
	ns       string
	registry *prometheus.Registry

	grpcHandled  *prometheus.CounterVec
	grpcDuration *prometheus.HistogramVec

	natsMessages *prometheus.CounterVec
	natsDuration *prometheus.HistogramVec
}

// New creates a registry for the given service. The service name becomes the
// metric namespace ("booking-service" → booking_service_*). Go runtime and
// process collectors are registered automatically.
func New(service string) *Metrics {
	ns := strings.ReplaceAll(strings.TrimSpace(service), "-", "_")

	m := &Metrics{
		ns:       ns,
		registry: prometheus.NewRegistry(),
		grpcHandled: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "grpc_handled_total",
				Help:      "gRPC requests handled, by short method (Service/Method) and canonical status code.",
			},
			[]string{"method", "code"},
		),
		grpcDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "grpc_handling_seconds",
				Help:      "gRPC request duration in seconds, by short method.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method"},
		),
		natsMessages: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: ns,
				Name:      "nats_messages_total",
				Help:      "NATS messages processed, by subject and result (ok|error|dlq).",
			},
			[]string{"subject", "result"},
		),
		natsDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: ns,
				Name:      "nats_handling_seconds",
				Help:      "NATS message handling duration in seconds, by subject.",
				// Handlers may run up to their 30s timeout — extend default buckets.
				Buckets: append(prometheus.DefBuckets, 30),
			},
			[]string{"subject"},
		),
	}

	m.registry.MustRegister(
		m.grpcHandled,
		m.grpcDuration,
		m.natsMessages,
		m.natsDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Registry returns the service-local registry for additional registrations
// (domain counters, custom collectors).
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// ObserveNATS records one processed NATS message. result must be one of
// NATSOk, NATSError, NATSDLQ; anything else is clamped to NATSError so the
// label set stays finite.
func (m *Metrics) ObserveNATS(subject, result string, d time.Duration) {
	switch result {
	case NATSOk, NATSError, NATSDLQ:
	default:
		result = NATSError
	}
	m.natsMessages.WithLabelValues(subject, result).Inc()
	m.natsDuration.WithLabelValues(subject).Observe(d.Seconds())
}
