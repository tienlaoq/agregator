package geocode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Geocode(t *testing.T) {
	const kazanBaumana = `{"response":{"GeoObjectCollection":{"featureMember":[
		{"GeoObject":{"Point":{"pos":"49.106414 55.796127"},
		 "metaDataProperty":{"GeocoderMetaData":{"precision":"exact"}}}}]}}}`

	tests := []struct {
		name    string
		status  int
		body    string
		want    Point
		wantErr error
	}{
		{
			name:   "exact match returns lat/lng in the right order",
			status: http.StatusOK,
			body:   kazanBaumana,
			// Yandex sends "lng lat"; a swap here would land in Kazakhstan.
			want: Point{Lat: 55.796127, Lng: 49.106414},
		},
		{
			name:    "no match",
			status:  http.StatusOK,
			body:    `{"response":{"GeoObjectCollection":{"featureMember":[]}}}`,
			wantErr: ErrNotFound,
		},
		{
			name:   "city-level precision is rejected",
			status: http.StatusOK,
			body: `{"response":{"GeoObjectCollection":{"featureMember":[
				{"GeoObject":{"Point":{"pos":"49.106414 55.796127"},
				 "metaDataProperty":{"GeocoderMetaData":{"precision":"other"}}}}]}}}`,
			wantErr: ErrNotFound,
		},
		{
			name:   "street precision is accepted",
			status: http.StatusOK,
			body: `{"response":{"GeoObjectCollection":{"featureMember":[
				{"GeoObject":{"Point":{"pos":"49.1 55.8"},
				 "metaDataProperty":{"GeocoderMetaData":{"precision":"street"}}}}]}}}`,
			want: Point{Lat: 55.8, Lng: 49.1},
		},
		{
			name:    "out of range coordinates",
			status:  http.StatusOK,
			body:    `{"response":{"GeoObjectCollection":{"featureMember":[{"GeoObject":{"Point":{"pos":"49.1 91.5"}}}]}}}`,
			wantErr: ErrNotFound,
		},
		{
			name:    "zero island is not a match",
			status:  http.StatusOK,
			body:    `{"response":{"GeoObjectCollection":{"featureMember":[{"GeoObject":{"Point":{"pos":"0 0"}}}]}}}`,
			wantErr: ErrNotFound,
		},
		{
			name:    "malformed pos",
			status:  http.StatusOK,
			body:    `{"response":{"GeoObjectCollection":{"featureMember":[{"GeoObject":{"Point":{"pos":"nope"}}}]}}}`,
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			c := NewClient("test-key")
			c.baseURL = srv.URL

			got, err := c.Geocode(context.Background(), "Казань, ул. Баумана, 5")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Geocode() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClient_Geocode_sendsAddressAndKey(t *testing.T) {
	var gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("geocode")
		gotKey = r.URL.Query().Get("apikey")
		_, _ = w.Write([]byte(`{"response":{"GeoObjectCollection":{"featureMember":[]}}}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient("secret")
	c.baseURL = srv.URL
	_, _ = c.Geocode(context.Background(), "Казань, ул. Баумана, 5")

	if gotQuery != "Казань, ул. Баумана, 5" {
		t.Errorf("geocode param = %q", gotQuery)
	}
	if gotKey != "secret" {
		t.Errorf("apikey param = %q", gotKey)
	}
}

func TestClient_Geocode_disabledWithoutKey(t *testing.T) {
	// No key must not mean "call the provider anonymously": it means the feature
	// is off, so local dev and CI create venues without a network round trip.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("provider must not be called without an API key")
	}))
	t.Cleanup(srv.Close)

	c := NewClient("   ")
	c.baseURL = srv.URL

	if c.Enabled() {
		t.Fatal("Enabled() = true for a blank key")
	}
	if _, err := c.Geocode(context.Background(), "Казань"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestClient_Geocode_providerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"apikey secret is invalid"}`, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	c := NewClient("secret")
	c.baseURL = srv.URL

	_, err := c.Geocode(context.Background(), "Казань")
	if err == nil {
		t.Fatal("expected an error on HTTP 403")
	}
	// The provider echoes the key back in its error body — it must not reach logs.
	if got := err.Error(); !strings.Contains(got, "403") || strings.Contains(got, "secret") {
		t.Errorf("error %q must name the status and must not leak the key", got)
	}
}
