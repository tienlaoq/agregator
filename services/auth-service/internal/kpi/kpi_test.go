package kpi

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestRegistration_IncrementsByMethod(t *testing.T) {
	registrations.Reset()

	Registration("password")
	Registration("password")
	Registration("oauth_google")

	if got := testutil.ToFloat64(registrations.WithLabelValues("password")); got != 2 {
		t.Errorf("password registrations = %v, want 2", got)
	}
	if got := testutil.ToFloat64(registrations.WithLabelValues("oauth_google")); got != 1 {
		t.Errorf("oauth_google registrations = %v, want 1", got)
	}
}

func TestLogin_RecordsResultLabel(t *testing.T) {
	logins.Reset()

	Login("password", true)
	Login("password", false)
	Login("password", false)

	if got := testutil.ToFloat64(logins.WithLabelValues("password", "ok")); got != 1 {
		t.Errorf("ok logins = %v, want 1", got)
	}
	if got := testutil.ToFloat64(logins.WithLabelValues("password", "fail")); got != 2 {
		t.Errorf("fail logins = %v, want 2", got)
	}
}

func TestRegister_RegistersBothCounters(t *testing.T) {
	reg := prometheus.NewRegistry()

	Register(reg)

	// MustRegister panics on a duplicate; a fresh registry must accept both.
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	want := map[string]bool{
		"auth_service_registrations_total": false,
		"auth_service_logins_total":        false,
	}
	for _, mf := range mfs {
		if _, ok := want[mf.GetName()]; ok {
			want[mf.GetName()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("counter %q not registered (gathered: %s)", name, names(mfs))
		}
	}
}

func names(mfs []*dto.MetricFamily) string {
	var b strings.Builder
	for i, mf := range mfs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(mf.GetName())
	}
	return b.String()
}
