package repository

import (
	"testing"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

func TestDerivedVenueFieldsFromHalls(t *testing.T) {
	tests := []struct {
		name         string
		halls        []domain.VenueHall
		wantFrom     int64
		wantWeekend  int64
		wantCapacity int32
	}{
		{
			name:  "no halls resets to zero",
			halls: nil,
		},
		{
			name: "weekday = min price_from, weekend = min explicit weekend",
			halls: []domain.VenueHall{
				{PriceFrom: 5000, PriceWeekend: 6000, Capacity: 10},
				{PriceFrom: 3000, PriceWeekend: 4000, Capacity: 20},
			},
			wantFrom:     3000,
			wantWeekend:  4000,
			wantCapacity: 20,
		},
		{
			name: "hall without weekend falls back to its weekday for the weekend min",
			halls: []domain.VenueHall{
				{PriceFrom: 5000, PriceWeekend: 6000, Capacity: 8},
				{PriceFrom: 2000, PriceWeekend: 0, Capacity: 8}, // weekend unset → 2000
			},
			wantFrom:     2000,
			wantWeekend:  2000, // min(6000, 2000)
			wantCapacity: 8,
		},
		{
			name: "weekend cheaper than any weekday",
			halls: []domain.VenueHall{
				{PriceFrom: 5000, PriceWeekend: 1000, Capacity: 8},
			},
			wantFrom:     5000,
			wantWeekend:  1000,
			wantCapacity: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, weekend, capacity, _ := derivedVenueFieldsFromHalls(tt.halls)
			if from != tt.wantFrom {
				t.Errorf("priceFrom = %d, want %d", from, tt.wantFrom)
			}
			if weekend != tt.wantWeekend {
				t.Errorf("priceWeekend = %d, want %d", weekend, tt.wantWeekend)
			}
			if capacity != tt.wantCapacity {
				t.Errorf("capacity = %d, want %d", capacity, tt.wantCapacity)
			}
		})
	}
}
