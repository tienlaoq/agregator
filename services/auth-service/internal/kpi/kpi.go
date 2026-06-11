// Package kpi exposes business-level counters (real-time KPIs) consumed by
// the business-kpi Grafana dashboard (docs/metrics-design.md §5).
// method values are server-defined ("password", "oauth_<provider>") — never
// raw user input, so cardinality is bounded by configured providers.
package kpi

import "github.com/prometheus/client_golang/prometheus"

var (
	registrations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "auth_service",
			Name:      "registrations_total",
			Help:      "Accounts created, by method (password|oauth_<provider>).",
		},
		[]string{"method"},
	)

	logins = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "auth_service",
			Name:      "logins_total",
			Help:      "Login attempts, by method and result (ok|fail).",
		},
		[]string{"method", "result"},
	)
)

// Register exposes the KPI counters via the service metrics registry.
// Call once from main.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(registrations, logins)
}

// Registration records one created account.
func Registration(method string) {
	registrations.WithLabelValues(method).Inc()
}

// Login records one login attempt outcome.
func Login(method string, ok bool) {
	result := "fail"
	if ok {
		result = "ok"
	}
	logins.WithLabelValues(method, result).Inc()
}
