// Package kpi exposes business-level counters (real-time KPIs) consumed by
// the business-kpi Grafana dashboard and severity:business alerts.
// Increments live in the usecase layer next to domain-event publication
// (docs/metrics-design.md §5). Labels never carry IDs — only the finite
// event set below.
package kpi

import "github.com/prometheus/client_golang/prometheus"

var bookings = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "booking_service",
		Name:      "bookings_total",
		Help:      "Booking lifecycle transitions by event (created|confirmed|cancelled|completed).",
	},
	[]string{"event"},
)

var knownEvents = map[string]bool{
	"created":   true,
	"confirmed": true,
	"cancelled": true,
	"completed": true,
}

// Register exposes the KPI counters via the service metrics registry.
// Call once from main; without it the counters still increment but are
// never scraped.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(bookings)
}

// BookingEvent records one lifecycle transition. Unknown values are clamped
// to "other" so label cardinality stays finite.
func BookingEvent(event string) {
	BookingEvents(event, 1)
}

// BookingEvents records n transitions at once (batch paths like the
// auto-complete poller).
func BookingEvents(event string, n int) {
	if n <= 0 {
		return
	}
	if !knownEvents[event] {
		event = "other"
	}
	bookings.WithLabelValues(event).Add(float64(n))
}
