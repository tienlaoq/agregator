package handler

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func Test_geoSearchParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLat    float64
		wantLng    float64
		wantRadius float64
		wantOK     bool
	}{
		{name: "valid point", query: "lat=55.796&lng=49.106&radius=10", wantLat: 55.796, wantLng: 49.106, wantRadius: 10, wantOK: true},
		{name: "zero coords stay valid", query: "lat=0&lng=0&radius=5", wantRadius: 5, wantOK: true},
		{name: "huge radius is clamped", query: "lat=55.796&lng=49.106&radius=100000", wantLat: 55.796, wantLng: 49.106, wantRadius: maxSearchRadiusKM, wantOK: true},
		{name: "no geo params at all", query: "q=баня", wantOK: true},

		{name: "missing radius is rejected", query: "lat=55.796&lng=49.106"},
		{name: "missing lng is rejected", query: "lat=55.796&radius=10"},
		{name: "unparsable lat is rejected", query: "lat=abc&lng=49.106&radius=10"},
		{name: "lat out of range", query: "lat=91&lng=49.106&radius=10"},
		{name: "lng out of range", query: "lat=55.796&lng=181&radius=10"},
		{name: "negative radius is rejected", query: "lat=55.796&lng=49.106&radius=-1"},
		{name: "zero radius is rejected", query: "lat=55.796&lng=49.106&radius=0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			w := httptest.NewRecorder()

			geo, ok := geoSearchParams(w, q)

			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			want := geoFilter{Lat: tt.wantLat, Lng: tt.wantLng, RadiusKM: tt.wantRadius}
			if geo != want {
				t.Errorf("geoSearchParams() = %+v, want %+v", geo, want)
			}
			// Ошибку пишет сама функция — контракт тот же, что у httpx.QueryInt.
			if !tt.wantOK && w.Code != 400 {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if tt.wantOK && w.Body.Len() != 0 {
				t.Errorf("valid input must not write a response, got %q", w.Body.String())
			}
		})
	}
}
