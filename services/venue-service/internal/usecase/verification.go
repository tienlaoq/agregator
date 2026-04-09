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

	if strings.TrimSpace(v.OGRN) != "" {
		ogrn := normalizeDigits(v.OGRN)
		if len(ogrn) != 13 && len(ogrn) != 15 {
			return pkgerr.InvalidArgument("ОГРН — 13 цифр, ОГРНИП — 15 цифр, либо оставьте поле пустым")
		}
		v.OGRN = ogrn
	} else {
		v.OGRN = ""
	}

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
