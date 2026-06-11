// Package kpi exposes business-level counters (real-time KPIs) consumed by
// the business-kpi Grafana dashboard and severity:business alerts.
// Increments live in the usecase layer at terminal status transitions
// (docs/metrics-design.md §5). No amounts and no IDs in labels — the
// webhook PII policy (CLAUDE.md) stays untouched; revenue truth is SQL.
package kpi

import "github.com/prometheus/client_golang/prometheus"

var payments = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "payment_service",
		Name:      "payments_total",
		Help:      "Terminal payment transitions by domain status (succeeded|cancelled|refunded).",
	},
	[]string{"status"},
)

var knownStatuses = map[string]bool{
	"succeeded": true,
	"cancelled": true,
	"refunded":  true,
}

// Register exposes the KPI counters via the service metrics registry.
// Call once from main.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(payments)
}

// Payment records one terminal transition. Unknown values are clamped to
// "other" so label cardinality stays finite.
func Payment(status string) {
	if !knownStatuses[status] {
		status = "other"
	}
	payments.WithLabelValues(status).Inc()
}
