package grpc

import (
	"testing"
	"time"

	"github.com/google/uuid"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

func TestVenueToProto_FullMapping(t *testing.T) {
	id := uuid.New()
	ownerID := uuid.New()
	moderatedBy := uuid.New()
	moderatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	createdAt := time.Date(2025, 12, 31, 23, 0, 0, 0, time.UTC)

	svcID := uuid.New()
	photoID := uuid.New()
	hallID := uuid.New()
	hallPhotoID := uuid.New()

	v := &domain.Venue{
		ID:                id,
		OwnerID:           ownerID,
		Slug:              "banya-1",
		Name:              "Тестовая баня",
		Type:              domain.VenueTypeBanya,
		Description:       "описание",
		Address:           "ул. Пушкина, 1",
		City:              "Москва",
		Latitude:          55.75,
		Longitude:         37.62,
		PriceFrom:         1500,
		Capacity:          10,
		Amenities:         []string{"wifi", "parking"},
		WorkingHours:      "10-22",
		Phone:             "+70000000000",
		AvgRating:         4.5,
		ReviewCount:       12,
		IsActive:          true,
		Status:            domain.StatusActive,
		ModerationComment: "ok",
		ModeratedAt:       &moderatedAt,
		ModeratedBy:       &moderatedBy,
		LegalEntityName:   "ИП Тестов",
		INN:               "7707083893",
		OGRN:              "1027700132195",
		PublicListingURL:  "https://yandex.ru/maps/org/test/123",
		VerificationNote:  "проверено",
		SocialLinks:       `{"vk":"https://vk.com/x"}`,
		PayoutLegalForm:   domain.PayoutLegalFormIP,
		CreatedAt:         createdAt,
		Services: []domain.VenueService{
			{ID: svcID, Name: "Парение", DurationMin: 60, Price: 2000, Description: "веник"},
		},
		Photos: []domain.VenuePhoto{
			{ID: photoID, URL: "https://cdn/x.jpg", SortOrder: 1, IsCover: true},
		},
		Halls: []domain.VenueHall{
			{
				ID:        hallID,
				Name:      "Большой зал",
				PriceFrom: 3000,
				Capacity:  20,
				Amenities: []string{"бассейн"},
				SortOrder: 2,
				Photos: []domain.VenueHallPhoto{
					{ID: hallPhotoID, URL: "https://cdn/hall.jpg", SortOrder: 1, IsCover: true},
				},
			},
		},
	}

	got := venueToProto(v)

	if got.GetId() != id.String() {
		t.Errorf("Id = %q, want %q", got.GetId(), id.String())
	}
	if got.GetOwnerId() != ownerID.String() {
		t.Errorf("OwnerId = %q, want %q", got.GetOwnerId(), ownerID.String())
	}
	if got.GetName() != "Тестовая баня" || got.GetSlug() != "banya-1" {
		t.Errorf("Name/Slug mismatch: %q/%q", got.GetName(), got.GetSlug())
	}
	if got.GetPriceFrom() != 1500 || got.GetCapacity() != 10 {
		t.Errorf("PriceFrom/Capacity mismatch: %d/%d", got.GetPriceFrom(), got.GetCapacity())
	}
	if got.GetInn() != "7707083893" || got.GetOgrn() != "1027700132195" {
		t.Errorf("INN/OGRN mismatch: %q/%q", got.GetInn(), got.GetOgrn())
	}
	if got.GetPayoutLegalForm() != domain.PayoutLegalFormIP {
		t.Errorf("PayoutLegalForm = %q, want ip", got.GetPayoutLegalForm())
	}
	if got.GetModeratedBy() != moderatedBy.String() {
		t.Errorf("ModeratedBy = %q, want %q", got.GetModeratedBy(), moderatedBy.String())
	}
	if !got.GetModeratedAt().AsTime().Equal(moderatedAt) {
		t.Errorf("ModeratedAt = %v, want %v", got.GetModeratedAt().AsTime(), moderatedAt)
	}
	if !got.GetCreatedAt().AsTime().Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", got.GetCreatedAt().AsTime(), createdAt)
	}

	if len(got.GetServices()) != 1 || got.GetServices()[0].GetName() != "Парение" {
		t.Fatalf("services not mapped: %+v", got.GetServices())
	}
	if len(got.GetPhotos()) != 1 || !got.GetPhotos()[0].GetIsCover() {
		t.Fatalf("photos not mapped: %+v", got.GetPhotos())
	}
	if len(got.GetHalls()) != 1 {
		t.Fatalf("halls not mapped: %+v", got.GetHalls())
	}
	hall := got.GetHalls()[0]
	if hall.GetName() != "Большой зал" || hall.GetCapacity() != 20 {
		t.Errorf("hall mapping mismatch: %+v", hall)
	}
	if len(hall.GetPhotos()) != 1 || !hall.GetPhotos()[0].GetIsCover() {
		t.Errorf("hall photo not mapped: %+v", hall.GetPhotos())
	}
}

func TestVenueToProto_NilModeration(t *testing.T) {
	v := &domain.Venue{ID: uuid.New(), OwnerID: uuid.New(), Status: domain.StatusDraft}
	got := venueToProto(v)
	if got.GetModeratedAt() != nil {
		t.Errorf("ModeratedAt = %v, want nil", got.GetModeratedAt())
	}
	if got.GetModeratedBy() != "" {
		t.Errorf("ModeratedBy = %q, want empty", got.GetModeratedBy())
	}
}

func TestManualBlockToProto(t *testing.T) {
	id := uuid.New()
	venueID := uuid.New()
	b := &domain.ManualSlotBlock{
		ID:       id,
		VenueID:  venueID,
		Date:     "2026-06-30",
		TimeFrom: "10:00",
		TimeTo:   "12:00",
		Note:     "телефонная бронь",
	}
	got := manualBlockToProto(b)
	if got.GetId() != id.String() || got.GetVenueId() != venueID.String() {
		t.Errorf("ids mismatch: %q/%q", got.GetId(), got.GetVenueId())
	}
	if got.GetDate() != "2026-06-30" || got.GetTimeFrom() != "10:00" || got.GetTimeTo() != "12:00" {
		t.Errorf("time fields mismatch: %+v", got)
	}
	if got.GetNote() != "телефонная бронь" {
		t.Errorf("Note = %q", got.GetNote())
	}
}

func TestListResultToProto(t *testing.T) {
	r := &domain.ListResult{
		Total:    42,
		Page:     2,
		PageSize: 20,
		Venues: []domain.Venue{
			{ID: uuid.New(), OwnerID: uuid.New(), Name: "A"},
			{ID: uuid.New(), OwnerID: uuid.New(), Name: "B"},
		},
	}
	got := listResultToProto(r)
	if got.GetTotal() != 42 || got.GetPage() != 2 || got.GetPageSize() != 20 {
		t.Errorf("pagination mismatch: %+v", got)
	}
	if len(got.GetVenues()) != 2 {
		t.Fatalf("venues len = %d, want 2", len(got.GetVenues()))
	}
	if got.GetVenues()[0].GetName() != "A" || got.GetVenues()[1].GetName() != "B" {
		t.Errorf("venue order/name mismatch")
	}
}

func TestHallProtoInputsToUpserts(t *testing.T) {
	existingID := uuid.New()
	existingIDStr := existingID.String()
	inputs := []*venuev1.VenueHallInput{
		{
			Id:        &existingIDStr,
			Name:      "Зал 1",
			PriceFrom: 1000,
			Capacity:  8,
			Amenities: []string{"душ"},
			SortOrder: 1,
		},
		{
			// no Id → insert; nil amenities normalized to empty slice
			Name:      "Зал 2",
			PriceFrom: 2000,
			Capacity:  4,
			SortOrder: 2,
		},
	}

	ups, err := hallProtoInputsToUpserts(inputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ups) != 2 {
		t.Fatalf("len = %d, want 2", len(ups))
	}
	if ups[0].ID == nil || *ups[0].ID != existingID {
		t.Errorf("first upsert ID = %v, want %v", ups[0].ID, existingID)
	}
	if ups[1].ID != nil {
		t.Errorf("second upsert ID = %v, want nil", ups[1].ID)
	}
	if len(ups[1].Amenities) != 0 {
		t.Errorf("nil amenities should yield empty list, got %#v", ups[1].Amenities)
	}
	if len(ups[0].Amenities) != 1 || ups[0].Amenities[0] != "душ" {
		t.Errorf("amenities not copied: %#v", ups[0].Amenities)
	}
}

func TestHallProtoInputsToUpserts_InvalidID(t *testing.T) {
	bad := "not-a-uuid"
	inputs := []*venuev1.VenueHallInput{
		{Id: &bad, Name: "Зал"},
	}
	if _, err := hallProtoInputsToUpserts(inputs); err == nil {
		t.Fatal("expected error for malformed hall id, got nil")
	}
}
