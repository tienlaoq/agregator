package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterPgxPool registers a collector that reads pool.Stat() on every scrape
// (USE: utilization = acquired/max, saturation = empty_acquire growth).
func (m *Metrics) RegisterPgxPool(pool *pgxpool.Pool) {
	m.registry.MustRegister(newPgxPoolCollector(pool, m.ns))
}

type pgxPoolCollector struct {
	pool *pgxpool.Pool

	totalConns      *prometheus.Desc
	idleConns       *prometheus.Desc
	acquiredConns   *prometheus.Desc
	maxConns        *prometheus.Desc
	acquireTotal    *prometheus.Desc
	emptyAcquire    *prometheus.Desc
	canceledAcquire *prometheus.Desc
	acquireSeconds  *prometheus.Desc
}

func newPgxPoolCollector(pool *pgxpool.Pool, ns string) *pgxPoolCollector {
	fq := func(name string) string { return prometheus.BuildFQName(ns, "pgxpool", name) }
	return &pgxPoolCollector{
		pool:            pool,
		totalConns:      prometheus.NewDesc(fq("total_conns"), "Total connections in the pool.", nil, nil),
		idleConns:       prometheus.NewDesc(fq("idle_conns"), "Idle connections in the pool.", nil, nil),
		acquiredConns:   prometheus.NewDesc(fq("acquired_conns"), "Connections currently acquired.", nil, nil),
		maxConns:        prometheus.NewDesc(fq("max_conns"), "Maximum pool size.", nil, nil),
		acquireTotal:    prometheus.NewDesc(fq("acquire_total"), "Cumulative successful acquires.", nil, nil),
		emptyAcquire:    prometheus.NewDesc(fq("empty_acquire_total"), "Cumulative acquires that waited for a free connection (saturation signal).", nil, nil),
		canceledAcquire: prometheus.NewDesc(fq("canceled_acquire_total"), "Cumulative acquires canceled by context.", nil, nil),
		acquireSeconds:  prometheus.NewDesc(fq("acquire_duration_seconds_total"), "Cumulative time spent acquiring connections.", nil, nil),
	}
}

func (c *pgxPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalConns
	ch <- c.idleConns
	ch <- c.acquiredConns
	ch <- c.maxConns
	ch <- c.acquireTotal
	ch <- c.emptyAcquire
	ch <- c.canceledAcquire
	ch <- c.acquireSeconds
}

func (c *pgxPoolCollector) Collect(ch chan<- prometheus.Metric) {
	st := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(st.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(st.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.acquiredConns, prometheus.GaugeValue, float64(st.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.maxConns, prometheus.GaugeValue, float64(st.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.acquireTotal, prometheus.CounterValue, float64(st.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.emptyAcquire, prometheus.CounterValue, float64(st.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.canceledAcquire, prometheus.CounterValue, float64(st.CanceledAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireSeconds, prometheus.CounterValue, st.AcquireDuration().Seconds())
}
