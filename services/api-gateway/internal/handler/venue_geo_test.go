package handler

import (
	"net/url"
	"testing"
)

func Test_geoSearchParams(t *testing.T) {
	tests := []struct {
		name                         string
		query                        string
		wantLat, wantLng, wantRadius float64
	}{
		{"valid point", "lat=55.796&lng=49.106&radius=10", 55.796, 49.106, 10},
		{"zero coords stay valid", "lat=0&lng=0&radius=5", 0, 0, 5},
		{"missing radius drops filter", "lat=55.796&lng=49.106", 0, 0, 0},
		{"missing lng drops filter", "lat=55.796&radius=10", 0, 0, 0},
		{"unparsable lat drops filter", "lat=abc&lng=49.106&radius=10", 0, 0, 0},
		{"lat out of range", "lat=91&lng=49.106&radius=10", 0, 0, 0},
		{"lng out of range", "lat=55.796&lng=181&radius=10", 0, 0, 0},
		{"negative radius drops filter", "lat=55.796&lng=49.106&radius=-1", 0, 0, 0},
		{"huge radius is clamped", "lat=55.796&lng=49.106&radius=100000", 55.796, 49.106, maxSearchRadiusKM},
		{"no geo params at all", "q=баня", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			lat, lng, radius := geoSearchParams(q)
			if lat != tt.wantLat || lng != tt.wantLng || radius != tt.wantRadius {
				t.Errorf("geoSearchParams() = (%v, %v, %v), want (%v, %v, %v)",
					lat, lng, radius, tt.wantLat, tt.wantLng, tt.wantRadius)
			}
		})
	}
}
