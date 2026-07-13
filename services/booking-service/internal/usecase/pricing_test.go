package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
)

func TestHourlyTotal(t *testing.T) {
	tests := []struct {
		price   int64
		minutes int
		want    int64
	}{
		{3000, 60, 3000},  // exactly 1 hour
		{3000, 61, 6000},  // rounds up to 2 hours
		{3000, 120, 6000}, // exactly 2 hours
		{3000, 1, 3000},   // <1h still charges 1 hour
		{0, 120, 0},       // no price → 0
		{3000, 0, 0},      // no minutes → 0
		{3000, -10, 0},    // negative minutes → 0
	}
	for _, tt := range tests {
		if got := hourlyTotal(tt.price, tt.minutes); got != tt.want {
			t.Errorf("hourlyTotal(%d,%d) = %d, want %d", tt.price, tt.minutes, got, tt.want)
		}
	}
}

func TestNormalizeBookingHallIDs(t *testing.T) {
	got := normalizeBookingHallIDs([]string{" h1 ", "h2", "h1", "", "  ", "h2"})
	require.Equal(t, []string{"h1", "h2"}, got, "trims, dedups, drops blanks, preserves order")
	require.Nil(t, normalizeBookingHallIDs([]string{"", "  "}))
}

func TestNormalizeBookingServiceIDs(t *testing.T) {
	require.Equal(t, []string{"s1", "s2"}, normalizeBookingServiceIDs([]string{"s1", "s1", "s2"}, "legacy"))
	// Falls back to the legacy single id when the list is empty.
	require.Equal(t, []string{"legacy"}, normalizeBookingServiceIDs(nil, " legacy "))
	require.Nil(t, normalizeBookingServiceIDs(nil, "  "))
}

func TestPersistServiceFields(t *testing.T) {
	s, pkg := persistServiceFields(nil)
	require.Equal(t, "", s)
	require.Nil(t, pkg)

	s, pkg = persistServiceFields([]string{"only"})
	require.Equal(t, "only", s)
	require.Nil(t, pkg)

	s, pkg = persistServiceFields([]string{"a", "b"})
	require.Equal(t, "", s)
	require.Equal(t, []string{"a", "b"}, pkg)
}

func venueWithHalls() *venuev1.VenueResponse {
	return &venuev1.VenueResponse{
		Id: "v1", PriceFrom: 3000,
		Halls: []*venuev1.VenueHall{
			{Id: "h1", PriceFrom: 5000},
			{Id: "h2", PriceFrom: 2000},
		},
		Services: []*venuev1.VenueServiceItem{
			{Id: "svc-fixed", Price: 10000},
			{Id: "svc-hourly", Price: 0},
		},
	}
}

func TestValidateHallIDs(t *testing.T) {
	v := venueWithHalls()
	require.NoError(t, validateHallIDs(v, nil))
	require.NoError(t, validateHallIDs(v, []string{"h1", "h2"}))
	require.Error(t, validateHallIDs(v, []string{"h1", "unknown"}))
}

func TestEffectiveHourlyPriceFromVenueAndHalls(t *testing.T) {
	v := venueWithHalls()
	// No halls → venue base.
	require.Equal(t, int64(3000), effectiveHourlyPriceFromVenueAndHalls(v, nil))
	// Hall with higher price raises the base.
	require.Equal(t, int64(5000), effectiveHourlyPriceFromVenueAndHalls(v, []string{"h1"}))
	// Hall cheaper than base does not lower it.
	require.Equal(t, int64(3000), effectiveHourlyPriceFromVenueAndHalls(v, []string{"h2"}))
	// Max across selected halls.
	require.Equal(t, int64(5000), effectiveHourlyPriceFromVenueAndHalls(v, []string{"h1", "h2"}))
}

func TestComputeBookingTotalPriceMulti(t *testing.T) {
	v := venueWithHalls()

	t.Run("no services → hourly on effective base", func(t *testing.T) {
		// h1 base 5000 × 2h (90min → 2h) = 10000.
		total, err := computeBookingTotalPriceMulti(v, nil, []string{"h1"}, 90)
		require.NoError(t, err)
		require.Equal(t, int64(10000), total)
	})

	t.Run("no halls venue → venue base hourly", func(t *testing.T) {
		// Заведение без залов вовсе: цена берётся с venue.price_from.
		// 2000 × 2h (120min) = 4000.
		hallless := &venuev1.VenueResponse{Id: "v2", PriceFrom: 2000}
		total, err := computeBookingTotalPriceMulti(hallless, nil, nil, 120)
		require.NoError(t, err)
		require.Equal(t, int64(4000), total)
	})

	t.Run("fixed-price service", func(t *testing.T) {
		total, err := computeBookingTotalPriceMulti(v, []string{"svc-fixed"}, nil, 120)
		require.NoError(t, err)
		require.Equal(t, int64(10000), total)
	})

	t.Run("one hourly service uses hourly base", func(t *testing.T) {
		// svc-hourly has price 0 → charged hourly: base 3000 × 1h = 3000.
		total, err := computeBookingTotalPriceMulti(v, []string{"svc-hourly"}, nil, 60)
		require.NoError(t, err)
		require.Equal(t, int64(3000), total)
	})

	t.Run("fixed + hourly combine", func(t *testing.T) {
		// 10000 (fixed) + 3000 (hourly 1h) = 13000.
		total, err := computeBookingTotalPriceMulti(v, []string{"svc-fixed", "svc-hourly"}, nil, 60)
		require.NoError(t, err)
		require.Equal(t, int64(13000), total)
	})

	t.Run("unknown service rejected", func(t *testing.T) {
		_, err := computeBookingTotalPriceMulti(v, []string{"nope"}, nil, 60)
		require.Error(t, err)
	})

	t.Run("more than one hourly service rejected", func(t *testing.T) {
		v2 := venueWithHalls()
		v2.Services = append(v2.Services, &venuev1.VenueServiceItem{Id: "svc-hourly-2", Price: 0})
		_, err := computeBookingTotalPriceMulti(v2, []string{"svc-hourly", "svc-hourly-2"}, nil, 60)
		require.Error(t, err)
	})
}
