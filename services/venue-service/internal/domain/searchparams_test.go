package domain

import (
	"reflect"
	"testing"
)

func TestSearchParams_EffectiveCities(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty and whitespace dropped",
			in:   []string{"", "   ", "\t"},
			want: nil,
		},
		{
			name: "trims surrounding whitespace",
			in:   []string{"  Москва  "},
			want: []string{"Москва"},
		},
		{
			name: "dedupes case-insensitively keeping first form",
			in:   []string{"Москва", "москва", "МОСКВА"},
			want: []string{"Москва"},
		},
		{
			name: "preserves order of distinct cities",
			in:   []string{"Казань", "Москва", "Сочи"},
			want: []string{"Казань", "Москва", "Сочи"},
		},
		{
			name: "mixed: trim, dedupe, drop empty",
			in:   []string{" Москва", "москва ", "", "Сочи", "СОЧИ"},
			want: []string{"Москва", "Сочи"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SearchParams{Cities: tt.in}.EffectiveCities()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EffectiveCities() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
