package usecase

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

func validDraftVenue() *domain.Venue {
	return &domain.Venue{
		LegalEntityName:  "ООО Тест",
		INN:              "7707083893",
		OGRN:             "1027700132195",
		PublicListingURL: "https://yandex.ru/maps/org/foo/123",
		Description:      "Это описание заведения достаточной длины для отправки на модерацию.",
		Photos:           []domain.VenuePhoto{{ID: uuid.New(), URL: "https://example.com/p.jpg"}},
		Halls: []domain.VenueHall{
			{Name: "Зал 1", PriceFrom: 1000},
		},
	}
}

func TestValidateVenueType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errSub  string
	}{
		{name: "ok_banya", input: "banya"},
		{name: "ok_sauna", input: "sauna"},
		{name: "ok_hammam", input: "hammam"},
		{name: "err_empty", input: "", wantErr: true, errSub: "banya"},
		{name: "err_uppercase", input: "BANYA", wantErr: true, errSub: "banya"},
		{name: "err_unknown_jacuzzi", input: "jacuzzi", wantErr: true, errSub: "banya"},
		{name: "err_unknown_pool", input: "pool", wantErr: true, errSub: "banya"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVenueType(tt.input)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status, got %T: %v", err, err)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			if tt.errSub != "" {
				assert.Contains(t, strings.ToLower(st.Message()), strings.ToLower(tt.errSub))
			}
		})
	}
}

func TestValidateVenueCoordinates(t *testing.T) {
	tests := []struct {
		name    string
		lat     float64
		lng     float64
		wantErr bool
	}{
		{name: "ok_moscow", lat: 55.7558, lng: 37.6176, wantErr: false},
		{name: "ok_zero_zero", lat: 0, lng: 0, wantErr: false},
		{name: "ok_max_bounds", lat: 90, lng: 180, wantErr: false},
		{name: "ok_min_bounds", lat: -90, lng: -180, wantErr: false},
		{name: "err_lat_too_high", lat: 90.0001, lng: 0, wantErr: true},
		{name: "err_lat_too_low", lat: -90.0001, lng: 0, wantErr: true},
		{name: "err_lng_too_high", lat: 0, lng: 180.0001, wantErr: true},
		{name: "err_lng_too_low", lat: 0, lng: -180.0001, wantErr: true},
		{name: "err_both_out_of_range", lat: 91, lng: 200, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVenueCoordinates(tt.lat, tt.lng)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status, got %T: %v", err, err)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			assert.Contains(t, st.Message(), "координаты")
		})
	}
}

func TestValidateVenueDraftReadyForReview(t *testing.T) {
	desc40 := strings.Repeat("б", 40)
	desc39 := strings.Repeat("в", 39)

	tests := []struct {
		name     string
		mutate   func(*domain.Venue)
		wantErr  bool
		wantCode codes.Code
		errSub   string
	}{
		{
			name:    "ok_default",
			wantErr: false,
		},
		{
			name: "ok_ip_inn_12_ogrnip_15",
			mutate: func(v *domain.Venue) {
				v.INN = "500100732259"
				v.OGRN = "315502400000058"
			},
			wantErr: false,
		},
		{
			name: "ok_description_exactly_40_runes",
			mutate: func(v *domain.Venue) {
				v.Description = desc40
			},
			wantErr: false,
		},
		{
			name: "ok_second_hall_salvages_first_empty_name",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{
					{Name: "", PriceFrom: 5000},
					{Name: "Основной", PriceFrom: 800},
				}
			},
			wantErr: false,
		},
		{
			name: "ok_second_hall_salvages_first_zero_price",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{
					{Name: "Без цены", PriceFrom: 0},
					{Name: "С ценой", PriceFrom: 1},
				}
			},
			wantErr: false,
		},
		{
			name: "err_photos_nil",
			mutate: func(v *domain.Venue) {
				v.Photos = nil
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "фото",
		},
		{
			name: "err_photos_empty_slice",
			mutate: func(v *domain.Venue) {
				v.Photos = []domain.VenuePhoto{}
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "фото",
		},
		{
			name: "err_halls_nil",
			mutate: func(v *domain.Venue) {
				v.Halls = nil
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "зал",
		},
		{
			name: "err_halls_empty_slice",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{}
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "зал",
		},
		{
			name: "err_hall_name_only_whitespace",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{{Name: "  \t  ", PriceFrom: 1000}}
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "зал",
		},
		{
			name: "err_hall_empty_name_positive_price",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{{Name: "", PriceFrom: 500}}
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "зал",
		},
		{
			name: "err_hall_name_ok_zero_price",
			mutate: func(v *domain.Venue) {
				v.Halls = []domain.VenueHall{{Name: "Зал", PriceFrom: 0}}
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "зал",
		},
		{
			name: "err_description_39_runes",
			mutate: func(v *domain.Venue) {
				v.Description = desc39
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "40",
		},
		{
			name: "err_description_only_whitespace",
			mutate: func(v *domain.Venue) {
				v.Description = "  \n\t  "
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "40",
		},
		{
			name: "err_inn_invalid_length",
			mutate: func(v *domain.Venue) {
				v.INN = "123"
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "ИНН",
		},
		{
			name: "err_ogrn_wrong_length_for_org",
			mutate: func(v *domain.Venue) {
				// 10-значный ИНН (юрлицо) → нужен ОГРН 13 цифр, не 15
				v.OGRN = "123456789012345"
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "ОГРН",
		},
		{
			name: "err_ogrn_wrong_length_for_ip",
			mutate: func(v *domain.Venue) {
				v.INN = "500100732259"
				v.OGRN = "1027700132195"
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "ОГРНИП",
		},
		{
			name: "err_public_listing_localhost",
			mutate: func(v *domain.Venue) {
				v.PublicListingURL = "http://localhost:3000/venue"
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "недопустимый",
		},
		{
			name: "err_legal_name_too_short",
			mutate: func(v *domain.Venue) {
				v.LegalEntityName = "АБ"
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "наименован",
		},
		{
			name: "err_verification_note_too_long",
			mutate: func(v *domain.Venue) {
				v.VerificationNote = strings.Repeat("x", 2001)
			},
			wantErr:  true,
			wantCode: codes.InvalidArgument,
			errSub:   "комментарий",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validDraftVenue()
			if tt.mutate != nil {
				tt.mutate(v)
			}
			err := ValidateVenueDraftReadyForReview(v)
			if !tt.wantErr {
				require.NoError(t, err, "expected success")
				return
			}
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, st.Code(), "message: %s", st.Message())
			if tt.errSub != "" {
				assert.Contains(t, strings.ToLower(st.Message()), strings.ToLower(tt.errSub),
					"full message: %q", st.Message())
			}
		})
	}
}
