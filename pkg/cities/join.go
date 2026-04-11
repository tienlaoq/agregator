package cities

import "strings"

// Sep разделяет несколько городов в одном строковом поле (gRPC city). Не встречается в обычных названиях.
const Sep = "\x1e"

// Join объединяет города для передачи в одном поле.
func Join(parts []string) string {
	var b []string
	seen := map[string]struct{}{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		k := strings.ToLower(t)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		b = append(b, t)
	}
	if len(b) == 0 {
		return ""
	}
	return strings.Join(b, Sep)
}

// Split разбирает поле обратно в список уникальных городов (без учёта регистра при дедупе).
func Split(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if !strings.Contains(s, Sep) {
		return []string{s}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, p := range strings.Split(s, Sep) {
		t := strings.TrimSpace(p)
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
