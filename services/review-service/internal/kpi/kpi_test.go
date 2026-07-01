package kpi

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestReview_RatingLabels(t *testing.T) {
	reviews.Reset()

	for _, r := range []int32{1, 2, 3, 4, 5, 5} {
		Review(r)
	}

	if got := testutil.ToFloat64(reviews.WithLabelValues("5")); got != 2 {
		t.Errorf("rating 5 = %v, want 2", got)
	}
	for _, r := range []string{"1", "2", "3", "4"} {
		if got := testutil.ToFloat64(reviews.WithLabelValues(r)); got != 1 {
			t.Errorf("rating %s = %v, want 1", r, got)
		}
	}
}

func TestReview_OutOfRangeClampedToOther(t *testing.T) {
	reviews.Reset()

	Review(0)
	Review(6)
	Review(-3)

	if got := testutil.ToFloat64(reviews.WithLabelValues("other")); got != 3 {
		t.Errorf("other = %v, want 3 (ratings outside 1..5)", got)
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
		if mf.GetName() == "review_service_reviews_total" {
			found = true
		}
	}
	if !found {
		t.Error("review_service_reviews_total not registered")
	}
}
