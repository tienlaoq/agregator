package geo

import (
	"math"
	"testing"
)

func TestHaversineKm_samePoint(t *testing.T) {
	if d := HaversineKm(55.75, 37.62, 55.75, 37.62); d > 1e-9 {
		t.Fatalf("expected 0, got %v", d)
	}
}

func TestHaversineKm_moscowSpbOrderOfMagnitude(t *testing.T) {
	// ~635 km — проверяем только порядок величины (точное значение зависит от точек).
	d := HaversineKm(55.7558, 37.6173, 59.9343, 30.3351)
	if d < 500 || d > 800 {
		t.Fatalf("unexpected distance km: %v", d)
	}
}

func TestHaversineKm_shortChord(t *testing.T) {
	// ~1° широты на экваторе ≈ 111 км
	d := HaversineKm(0, 0, 1, 0)
	if math.Abs(d-111.0) > 2 {
		t.Fatalf("expected ~111 km, got %v", d)
	}
}
