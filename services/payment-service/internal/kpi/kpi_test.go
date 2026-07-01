package kpi

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPayment_KnownStatuses(t *testing.T) {
	payments.Reset()

	Payment("succeeded")
	Payment("succeeded")
	Payment("cancelled")
	Payment("refunded")

	if got := testutil.ToFloat64(payments.WithLabelValues("succeeded")); got != 2 {
		t.Errorf("succeeded = %v, want 2", got)
	}
	if got := testutil.ToFloat64(payments.WithLabelValues("cancelled")); got != 1 {
		t.Errorf("cancelled = %v, want 1", got)
	}
	if got := testutil.ToFloat64(payments.WithLabelValues("refunded")); got != 1 {
		t.Errorf("refunded = %v, want 1", got)
	}
}

func TestPayment_UnknownStatusClampedToOther(t *testing.T) {
	payments.Reset()

	Payment("weird")
	Payment("")

	if got := testutil.ToFloat64(payments.WithLabelValues("other")); got != 2 {
		t.Errorf("other = %v, want 2 (unknown statuses clamped)", got)
	}
}

func TestRegister(t *testing.T) {
	reg := prometheus.NewRegistry()
	Register(reg)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var found bool
	for _, mf := range mfs {
		if mf.GetName() == "payment_service_payments_total" {
			found = true
		}
	}
	if !found {
		t.Error("payment_service_payments_total not registered")
	}
}
