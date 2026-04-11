package usecase

import (
	"encoding/json"
	"strings"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"
)

const maxSocialLinksJSONBytes = 4000

// NormalizeSocialLinksJSON validates and normalizes JSON object for venues.social_links.
func NormalizeSocialLinksJSON(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "{}", nil
	}
	if len(s) > maxSocialLinksJSONBytes {
		return "", pkgerrors.InvalidArgument("соцсети: слишком много данных")
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", pkgerrors.InvalidArgument("соцсети: неверный JSON")
	}
	if _, ok := v.(map[string]any); !ok {
		return "", pkgerrors.InvalidArgument("соцсети: ожидается JSON-объект")
	}
	return s, nil
}
