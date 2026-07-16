package forge

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAppKeyPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, string(pemBytes)
}

func TestNewGitHubAppValidation(t *testing.T) {
	_, pemStr := testAppKeyPEM(t)

	_, err := NewGitHubApp("", pemStr, "sec")
	require.ErrorContains(t, err, "app id")
	_, err = NewGitHubApp("123", "not pem", "sec")
	require.ErrorContains(t, err, "PEM")
	_, err = NewGitHubApp("123", pemStr, "")
	require.ErrorContains(t, err, "webhook secret")

	app, err := NewGitHubApp("123", pemStr, "sec")
	require.NoError(t, err)
	assert.Equal(t, "sec", app.WebhookSecret())
}

func TestGitHubAppJWTAndInstallationToken(t *testing.T) {
	key, pemStr := testAppKeyPEM(t)
	app, err := NewGitHubApp("31337", pemStr, "sec")
	require.NoError(t, err)

	var mints atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every app-API call must carry a JWT that verifies against the app
		// key and names the app as issuer.
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tok, err := jwt.ParseWithClaims(auth, &jwt.RegisteredClaims{}, func(*jwt.Token) (any, error) {
			return &key.PublicKey, nil
		}, jwt.WithValidMethods([]string{"RS256"}))
		assert.NoError(t, err)
		if err == nil {
			claims := tok.Claims.(*jwt.RegisteredClaims)
			assert.Equal(t, "31337", claims.Issuer)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/site/installation":
			_, _ = w.Write([]byte(`{"id": 42}`))
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens":
			mints.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"token": "ghs_test%d", "expires_at": %q}`,
				mints.Load(), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	app.baseURL = srv.URL
	app.http = srv.Client()

	// TokenForRepo resolves the installation and mints.
	tok, err := app.TokenForRepo(context.Background(), "acme/site")
	require.NoError(t, err)
	assert.Equal(t, "ghs_test1", tok)

	// A second call within the expiry window reuses the cached token.
	tok2, err := app.InstallationToken(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "ghs_test1", tok2)
	assert.Equal(t, int32(1), mints.Load(), "token minted once, then cached")

	// An expiring token re-mints.
	app.mu.Lock()
	app.tokens[42] = installationToken{token: "ghs_test1", expiresAt: time.Now().Add(time.Minute)}
	app.mu.Unlock()
	tok3, err := app.InstallationToken(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, "ghs_test2", tok3)
}
