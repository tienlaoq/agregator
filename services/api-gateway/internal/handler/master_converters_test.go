package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
)

func TestMasterProtoToJSON_NilReturnsNil(t *testing.T) {
	if got := masterProtoToJSON(nil); got != nil {
		t.Fatalf("masterProtoToJSON(nil) = %v, want nil", got)
	}
}

func TestMasterProtoToJSON_RendersNestedCollections(t *testing.T) {
	m := &masterv1.Master{
		Id:          "m1",
		UserId:      "u1",
		Slug:        "ivan",
		DisplayName: "Иван",
		Services: []*masterv1.MasterServiceItem{
			{Id: "s1", Name: "Парение", DurationMin: 60, Price: 3000, SortOrder: 1},
		},
		Photos: []*masterv1.MasterPhoto{
			{Id: "p1", Url: "https://cdn/x.jpg", SortOrder: 0, IsCover: true},
		},
		Credentials: []*masterv1.MasterCredentialItem{
			{Id: "c1", Kind: "diploma", Title: "Сертификат", Issuer: "Школа", Year: 2020, SortOrder: 0},
		},
		TravelExcludeZones: []*masterv1.MasterTravelExcludeZone{
			{Id: "z1", Latitude: 55.7, Longitude: 37.6, RadiusKm: 5, Label: "Центр"},
		},
	}
	out := masterProtoToJSON(m)

	for _, key := range []string{"services", "photos", "credentials", "travel_exclude_zones"} {
		arr, ok := out[key].([]map[string]any)
		if !ok {
			t.Fatalf("%q should be []map[string]any, got %T", key, out[key])
		}
		if len(arr) != 1 {
			t.Fatalf("%q should have 1 element, got %d", key, len(arr))
		}
	}
	if out["photos"].([]map[string]any)[0]["is_cover"] != true {
		t.Fatalf("photo is_cover not propagated: %v", out["photos"])
	}
}

func TestMasterProtoToJSON_OptionalPointerFields(t *testing.T) {
	// Absent: optional fields must be omitted entirely.
	bare := masterProtoToJSON(&masterv1.Master{Id: "m1"})
	for _, k := range []string{"moderated_by", "moderated_at", "travel_base_latitude", "travel_base_longitude", "created_at", "updated_at"} {
		if _, ok := bare[k]; ok {
			t.Fatalf("optional %q should be omitted when unset: %v", k, bare)
		}
	}

	// Present: optional fields must be rendered.
	full := masterProtoToJSON(&masterv1.Master{
		Id:                  "m1",
		ModeratedBy:         proto.String("admin-1"),
		ModeratedAt:         timestamppb.Now(),
		TravelBaseLatitude:  proto.Float64(55.75),
		TravelBaseLongitude: proto.Float64(37.61),
		CreatedAt:           timestamppb.Now(),
		UpdatedAt:           timestamppb.Now(),
	})
	if full["moderated_by"] != "admin-1" {
		t.Fatalf("moderated_by = %v, want admin-1", full["moderated_by"])
	}
	if full["travel_base_latitude"] != 55.75 {
		t.Fatalf("travel_base_latitude = %v, want 55.75", full["travel_base_latitude"])
	}
	for _, k := range []string{"moderated_at", "created_at", "updated_at"} {
		if _, ok := full[k]; !ok {
			t.Fatalf("timestamp %q should be present when set", k)
		}
	}
}

// The public projection must strip every payout/financial field — this is a
// PII boundary, so it is asserted explicitly and exhaustively.
func TestMasterProtoToJSONPublic_StripsAllPayoutFields(t *testing.T) {
	m := &masterv1.Master{
		Id:                         "m1",
		DisplayName:                "Иван",
		PayoutLegalForm:            "ip",
		PayoutLegalName:            "ИП Иванов",
		PayoutInn:                  "771234567890",
		PayoutKpp:                  "770101001",
		PayoutOgrn:                 "1027700000000",
		PayoutOgrnip:               "304770000000000",
		PayoutBankName:             "Bank",
		PayoutBik:                  "044525225",
		PayoutSettlementAccount:    "40702810000000000001",
		PayoutCorrespondentAccount: "30101810400000000225",
		PayoutVerificationStatus:   "verified",
		PayoutReady:                true,
	}

	pub := masterProtoToJSONPublic(m)

	payoutKeys := []string{
		"payout_legal_form", "payout_legal_name", "payout_inn", "payout_kpp",
		"payout_ogrn", "payout_ogrnip", "payout_bank_name", "payout_bik",
		"payout_settlement_account", "payout_correspondent_account",
		"payout_verification_status", "payout_ready",
	}
	for _, k := range payoutKeys {
		if _, leaked := pub[k]; leaked {
			t.Fatalf("SECURITY: public projection leaked payout field %q", k)
		}
	}
	// Non-payout fields must survive.
	if pub["display_name"] != "Иван" {
		t.Fatalf("display_name should remain in public projection: %v", pub["display_name"])
	}

	// And the private projection must still contain them, proving the deletes
	// are the only difference.
	priv := masterProtoToJSON(m)
	for _, k := range payoutKeys {
		if _, ok := priv[k]; !ok {
			t.Fatalf("private projection should retain payout field %q", k)
		}
	}
}
