// Package geocode resolves a postal address to coordinates through the Yandex
// HTTP Geocoder — the same provider (and the same API key) the frontend map
// already uses, so there is no second vendor to license or key-manage.
package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when the geocoder has no usable match for an address.
// Callers treat it as "leave the venue without coordinates", not as a failure.
var ErrNotFound = errors.New("geocode: address not found")

// Point is a resolved coordinate pair. Named fields rather than a positional
// (lat, lng) pair: both are float64 and Yandex itself returns them in the
// opposite order, which is exactly how a silent swap gets in.
type Point struct {
	Lat float64
	Lng float64
}

// lowPrecision are Yandex precision values that did not resolve to a building or
// a street. They usually collapse to the centre of a city, which would put every
// venue of that city on one spot and make "бани рядом" nonsense — worse than
// having no coordinates at all, because nothing looks broken.
var lowPrecision = map[string]bool{"other": true}

// Client is a Yandex geocoder client. Safe for concurrent use; it holds only
// configuration, never per-request state.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewClient builds a geocoder client. An empty apiKey yields a disabled client
// whose Geocode always reports ErrNotFound, so venue creation keeps working in
// environments where no key is provisioned (local dev, CI).
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: "https://geocode-maps.yandex.ru/v1/",
		// Geocoding runs inside the venue-creation request; a slow provider must
		// not hold a partner's form open. 5s is well past Yandex's typical p99.
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Enabled reports whether an API key is configured.
func (c *Client) Enabled() bool { return c.apiKey != "" }

type geocoderResponse struct {
	Response struct {
		GeoObjectCollection struct {
			FeatureMember []struct {
				GeoObject struct {
					Point struct {
						Pos string `json:"pos"`
					} `json:"Point"`
					MetaDataProperty struct {
						GeocoderMetaData struct {
							Precision string `json:"precision"`
						} `json:"GeocoderMetaData"`
					} `json:"metaDataProperty"`
				} `json:"GeoObject"`
			} `json:"featureMember"`
		} `json:"GeoObjectCollection"`
	} `json:"response"`
}

// Geocode resolves address to a Point. The address should already include the
// city: a bare street name matches in dozens of Russian cities and Yandex will
// happily return the wrong one.
func (c *Client) Geocode(ctx context.Context, address string) (Point, error) {
	if !c.Enabled() {
		return Point{}, ErrNotFound
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return Point{}, ErrNotFound
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return Point{}, fmt.Errorf("geocode: parse base url: %w", err)
	}
	q := u.Query()
	q.Set("apikey", c.apiKey)
	q.Set("geocode", address)
	q.Set("lang", "ru_RU")
	q.Set("format", "json")
	q.Set("results", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Point{}, fmt.Errorf("geocode: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Point{}, fmt.Errorf("geocode: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The body may carry the API key back in an error echo — report the code only.
		return Point{}, fmt.Errorf("geocode: provider returned %d", resp.StatusCode)
	}

	var body geocoderResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Point{}, fmt.Errorf("geocode: decode response: %w", err)
	}

	members := body.Response.GeoObjectCollection.FeatureMember
	if len(members) == 0 {
		return Point{}, ErrNotFound
	}
	obj := members[0].GeoObject
	if lowPrecision[obj.MetaDataProperty.GeocoderMetaData.Precision] {
		return Point{}, ErrNotFound
	}
	return parsePos(obj.Point.Pos)
}

// parsePos reads Yandex's "longitude latitude" pair — note the order, which is
// the reverse of how coordinates are written everywhere else in this codebase.
func parsePos(pos string) (Point, error) {
	fields := strings.Fields(pos)
	if len(fields) < 2 {
		return Point{}, ErrNotFound
	}
	lng, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return Point{}, ErrNotFound
	}
	lat, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return Point{}, ErrNotFound
	}
	if math.Abs(lat) > 90 || math.Abs(lng) > 180 {
		return Point{}, ErrNotFound
	}
	// (0,0) is a valid coordinate in the Gulf of Guinea but never a Russian
	// address; treating it as "no match" keeps the zero value meaningful upstream.
	if lat == 0 && lng == 0 {
		return Point{}, ErrNotFound
	}
	return Point{Lat: lat, Lng: lng}, nil
}
