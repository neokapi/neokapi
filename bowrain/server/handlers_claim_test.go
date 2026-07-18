package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestClaimURL asserts the emailed claim link uses the path form
// /claim/<token> — the only form the web router matches (route
// "claim/$token") and the form the CLI prints on `kapi init`. The
// query form /claim?token=... lands on an unmatched route.
func TestClaimURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		token   string
		want    string
	}{
		{
			name:    "path form",
			baseURL: "https://bowrain.cloud",
			token:   "clm_0123456789abcdef",
			want:    "https://bowrain.cloud/claim/clm_0123456789abcdef",
		},
		{
			name:    "trailing slash on base URL",
			baseURL: "https://bowrain.cloud/",
			token:   "clm_0123456789abcdef",
			want:    "https://bowrain.cloud/claim/clm_0123456789abcdef",
		},
		{
			name:    "local dev base URL with port",
			baseURL: "http://localhost:8080",
			token:   "clm_deadbeef",
			want:    "http://localhost:8080/claim/clm_deadbeef",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, claimURL(tc.baseURL, tc.token))
		})
	}
}
