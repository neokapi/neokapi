package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalLanguages(t *testing.T) {
	tests := []struct {
		name  string
		langs []string
		want  string
	}{
		{"nil", nil, "[]"},
		{"empty", []string{}, "[]"},
		{"single", []string{"fr"}, `["fr"]`},
		{"multiple", []string{"fr", "de", "es"}, `["fr","de","es"]`},
		{"with-region", []string{"fr-FR", "pt-BR"}, `["fr-FR","pt-BR"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, marshalLanguages(tt.langs))
		})
	}
}

func TestUnmarshalLanguages(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty-string", "", nil},
		{"empty-array", "[]", nil},
		{"single", `["fr"]`, []string{"fr"}},
		{"multiple", `["fr","de"]`, []string{"fr", "de"}},
		{"invalid-json", "not json", nil},
		{"wrong-type", `{"a":1}`, nil},
		{"json-empty-after-parse", `[]`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, unmarshalLanguages(tt.in))
		})
	}
}

func TestLanguagesRoundtrip(t *testing.T) {
	// marshal then unmarshal should be identity for non-empty slices.
	cases := [][]string{
		{"fr"},
		{"fr", "de", "es"},
		{"en-US", "zh-Hant"},
	}
	for _, langs := range cases {
		got := unmarshalLanguages(marshalLanguages(langs))
		assert.Equal(t, langs, got)
	}
}
