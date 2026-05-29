package domain

import "strings"

// EffectiveCities — уникальные непустые города для фильтра (ИЛИ).
func (p SearchParams) EffectiveCities() []string {
	return normalizeCityList(p.Cities)
}

func normalizeCityList(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		k := strings.ToLower(t)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, t)
	}
	return out
}
