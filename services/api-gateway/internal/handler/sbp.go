package handler

import (
	"net/http"

	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
)

// sbpBank is one selectable СБП member bank: the id used for payout routing plus
// a human-readable label for the bank picker.
type sbpBank struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stubSBPBanks is a PLACEHOLDER directory for the payout bank picker.
//
// ⚠️ The ids below are NOT verified payout member ids. They must be replaced with
// the chosen payout provider's SBP bank directory before this goes live — the
// C2B/НСПК consumer registry is not a safe source (its `schema` differs from the
// payout member id for some banks, e.g. ВТБ). Wired now only so the frontend bank
// picker can be built ahead of the payout-provider decision. The response carries
// "stub": true so callers can surface that the list is provisional.
var stubSBPBanks = []sbpBank{
	{ID: "100000000111", Name: "Сбербанк"},
	{ID: "100000000004", Name: "Т-Банк"},
	{ID: "100000000005", Name: "ВТБ"},
	{ID: "100000000008", Name: "Альфа-Банк"},
	{ID: "100000000007", Name: "Райффайзенбанк"},
	{ID: "100000000001", Name: "Газпромбанк"},
	{ID: "100000000015", Name: "Банк Открытие"},
	{ID: "100000000013", Name: "Совкомбанк"},
}

// ListSBPBanks GET /api/v1/sbp/banks — directory consumed by the payout bank
// picker on the owner/master finance pages. STUB: see stubSBPBanks.
func ListSBPBanks(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"banks": stubSBPBanks,
		"stub":  true,
	})
}
