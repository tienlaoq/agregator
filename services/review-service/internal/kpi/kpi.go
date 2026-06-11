// Package kpi exposes business-level counters (real-time KPIs) consumed by
// the business-kpi Grafana dashboard. The average rating is derived in
// PromQL: sum(rating × increase) / sum(increase) — no extra metric needed
// (docs/metrics-design.md §5).
package kpi

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var reviews = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "review_service",
		Name:      "reviews_total",
		Help:      "Reviews created, by rating (1..5).",
	},
	[]string{"rating"},
)

// Register exposes the KPI counters via the service metrics registry.
// Call once from main.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(reviews)
}

// Review records one created review. Ratings outside 1..5 are clamped to
// "other" (the usecase validates the range, so this is a safety net).
func Review(rating int32) {
	label := "other"
	if rating >= 1 && rating <= 5 {
		label = strconv.Itoa(int(rating))
	}
	reviews.WithLabelValues(label).Inc()
}
