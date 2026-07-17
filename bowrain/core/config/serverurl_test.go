package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"landing host rewrites to app host", "https://bowrain.cloud", "https://app.bowrain.cloud"},
		{"landing host with slash", "https://bowrain.cloud/", "https://app.bowrain.cloud"},
		{"www landing host", "https://www.bowrain.cloud", "https://app.bowrain.cloud"},
		{"http landing host upgrades to https", "http://bowrain.cloud", "https://app.bowrain.cloud"},
		{"app host untouched", "https://app.bowrain.cloud", "https://app.bowrain.cloud"},
		{"self-hosted untouched", "https://bowrain.example.com", "https://bowrain.example.com"},
		{"localhost untouched", "http://localhost:8080", "http://localhost:8080"},
		{"trailing slash trimmed", "https://bowrain.example.com/", "https://bowrain.example.com"},
		{"subdomain of landing not rewritten", "https://ctrl.bowrain.cloud", "https://ctrl.bowrain.cloud"},
		{"case-insensitive host match", "https://BOWRAIN.CLOUD", "https://app.bowrain.cloud"},
		{"not a URL returned as-is", "not a url", "not a url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeServerURL(tt.in))
		})
	}
}
