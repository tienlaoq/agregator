package usecase

import (
	"net/url"
	"strings"
	"unicode/utf8"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/venue-service/internal/domain"
)

func normalizeDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// validateVenueVerificationCore проверяет формат данных для модерации (ИНН, ссылка на карты и т.д.).
func validateVenueVerificationCore(v *domain.Venue) error {
	legal := strings.TrimSpace(v.LegalEntityName)
	if utf8.RuneCountInString(legal) < 3 {
		return pkgerr.InvalidArgument("укажите полное наименование ИП или юрлица как в ЕГРЮЛ/ЕГРИП (не менее 3 символов)")
	}
	if utf8.RuneCountInString(legal) > 500 {
		return pkgerr.InvalidArgument("наименование юрлица слишком длинное")
	}
	v.LegalEntityName = legal

	inn := normalizeDigits(v.INN)
	if len(inn) != 10 && len(inn) != 12 {
		return pkgerr.InvalidArgument("ИНН должен содержать 10 цифр (организация) или 12 цифр (ИП)")
	}
	v.INN = inn

	ogrn := normalizeDigits(v.OGRN)
	switch len(inn) {
	case 10:
		if len(ogrn) != 13 {
			return pkgerr.InvalidArgument("для юридического лица укажите ОГРН — ровно 13 цифр (проверка по ЕГРЮЛ)")
		}
	case 12:
		if len(ogrn) != 15 {
			return pkgerr.InvalidArgument("для ИП укажите ОГРНИП — ровно 15 цифр (проверка по ЕГРИП)")
		}
	}
	v.OGRN = ogrn

	rawURL := strings.TrimSpace(v.PublicListingURL)
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return pkgerr.InvalidArgument("укажите ссылку на карточку заведения в Яндекс.Картах, 2ГИС или аналоге (https://...)")
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, "localhost") || strings.HasPrefix(host, "127.") {
		return pkgerr.InvalidArgument("недопустимый адрес ссылки")
	}
	v.PublicListingURL = rawURL

	note := strings.TrimSpace(v.VerificationNote)
	if utf8.RuneCountInString(note) > 2000 {
		return pkgerr.InvalidArgument("комментарий для модерации слишком длинный")
	}
	v.VerificationNote = note

	return nil
}

// ValidateVenueDraftReadyForReview проверяет карточку и блок проверки перед переводом черновика в очередь модерации.
func ValidateVenueDraftReadyForReview(v *domain.Venue) error {
	if err := validateVenueVerificationCore(v); err != nil {
		return err
	}
	if len(v.Photos) == 0 {
		return pkgerr.InvalidArgument("добавьте хотя бы одно фото карточки заведения")
	}
	var hallOk bool
	for _, h := range v.Halls {
		if strings.TrimSpace(h.Name) != "" && h.PriceFrom > 0 {
			hallOk = true
			break
		}
	}
	if !hallOk {
		return pkgerr.InvalidArgument("укажите название и цену за час хотя бы для одного зала")
	}
	if utf8.RuneCountInString(strings.TrimSpace(v.Description)) < 40 {
		return pkgerr.InvalidArgument("опишите заведение: не менее 40 символов (примерно 2–3 предложения)")
	}
	return nil
}

// ValidateVenueVerificationForCreate — обязательные поля при создании заведения.
func ValidateVenueVerificationForCreate(v *domain.Venue) error {
	return validateVenueVerificationCore(v)
}

// ValidateVenueVerificationForUpdate — если партнёр меняет блок проверки, требуем полный корректный набор;
// пустой блок оставляем для старых записей до первого заполнения.
func ValidateVenueVerificationForUpdate(v *domain.Venue) error {
	inn := normalizeDigits(v.INN)
	legal := strings.TrimSpace(v.LegalEntityName)
	urlStr := strings.TrimSpace(v.PublicListingURL)
	ogrn := normalizeDigits(v.OGRN)
	note := strings.TrimSpace(v.VerificationNote)

	if inn == "" && legal == "" && urlStr == "" && ogrn == "" && note == "" {
		return nil
	}
	return validateVenueVerificationCore(v)
}

// validateVenueCoordinates returns an error if lat/lng are outside valid WGS-84 ranges.
func validateVenueCoordinates(lat, lng float64) error {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return pkgerr.InvalidArgument("некорректные координаты: широта -90..90, долгота -180..180")
	}
	return nil
}

// validateVenueType returns an error if t is not one of the known venue types.
func validateVenueType(t string) error {
	switch t {
	case domain.VenueTypeBanya, domain.VenueTypeSauna, domain.VenueTypeHammam:
		return nil
	default:
		return pkgerr.InvalidArgument("тип заведения: banya, sauna или hammam")
	}
}

// NormalizeVenuePayoutProfile validates payout_legal_form (empty allowed).
//
// Prior versions also trimmed yookassa_seller_account_id; that field is gone
// since the escrow refactor — partner payout rails live in payment-service.
func NormalizeVenuePayoutProfile(v *domain.Venue) error {
	plf := strings.TrimSpace(strings.ToLower(v.PayoutLegalForm))
	if plf == "" {
		v.PayoutLegalForm = domain.PayoutLegalFormEmpty
		return nil
	}
	switch plf {
	case domain.PayoutLegalFormIP, domain.PayoutLegalFormOOO, domain.PayoutLegalFormSelfEmployed, domain.PayoutLegalFormGPH:
		v.PayoutLegalForm = plf
		return nil
	default:
		return pkgerr.InvalidArgument("payout_legal_form: ip, ooo, self_employed или gph")
	}
}
