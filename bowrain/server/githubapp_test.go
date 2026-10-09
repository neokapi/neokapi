package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GITHUB_API_URL points the app at the API it should call: api.github.com
// when unset, a GitHub Enterprise Server's /api/v3 when set, and no app at
// all when the value is not a URL the app could call.
func TestGitHubAPIURLConfiguresTheApp(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	configured := Config{GitHubAppID: "1", GitHubAppPrivateKey: pemText, GitHubAppWebhookSecret: "s3cret"}

	tests := []struct {
		name    string
		cfg     Config
		apiURL  string
		wantAPI string // "" = no app
	}{
		{name: "no app configured", cfg: Config{}, apiURL: "https://ghe.example.com/api/v3"},
		{name: "unset is github.com", cfg: configured, wantAPI: "https://api.github.com"},
		{name: "enterprise server", cfg: configured, apiURL: "https://ghe.example.com/api/v3", wantAPI: "https://ghe.example.com/api/v3"},
		{name: "trailing slash is dropped", cfg: configured, apiURL: "https://ghe.example.com/api/v3/", wantAPI: "https://ghe.example.com/api/v3"},
		{name: "host without scheme disables the app", cfg: configured, apiURL: "ghe.example.com/api/v3"},
		{name: "other scheme disables the app", cfg: configured, apiURL: "ftp://ghe.example.com/api/v3"},
		{name: "half-configured app is disabled", cfg: Config{GitHubAppID: "1"}, wantAPI: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.GitHubAPIURL = tt.apiURL
			app := newGitHubApp(&cfg)
			if tt.wantAPI == "" {
				assert.Nil(t, app)
				return
			}
			require.NotNil(t, app)
			assert.Equal(t, tt.wantAPI, app.APIBase())
		})
	}
}
