package mtprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const googleTestKey = "super-secret-google-key-abc123"

// failingRoundTripper mimics how net/http surfaces a transport-level failure:
// it returns a *url.Error wrapping the request URL, exactly as the real
// http.Client.Do would. This lets us assert that the API key is never embedded
// in the URL (and therefore never leaks into the wrapped error).
type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, &url.Error{
		Op:  "Post",
		URL: req.URL.String(),
		Err: errors.New("connection reset by peer"),
	}
}

// TestGoogleProviderUsesAPIKeyHeader verifies the key is sent via the
// X-Goog-Api-Key header and never appears in the request URL/query string.
func TestGoogleProviderUsesAPIKeyHeader(t *testing.T) {
	var sawKeyInQuery bool
	var sawKeyHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, googleTestKey) {
			sawKeyInQuery = true
		}
		sawKeyHeader = r.Header.Get("X-Goog-Api-Key")
		_, _ = w.Write([]byte(`{"data":{"translations":[{"translatedText":"Bonjour"}]}}`))
	}))
	defer server.Close()

	p := NewGoogleProvider(GoogleConfig{APIKey: googleTestKey, BaseURL: server.URL})
	_, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	require.NoError(t, err)
	assert.False(t, sawKeyInQuery, "API key must not appear in the URL query string")
	assert.Equal(t, googleTestKey, sawKeyHeader, "API key must be sent via the X-Goog-Api-Key header")
}

// TestGoogleProviderTransportErrorDoesNotLeakKey simulates a transport failure
// and asserts the API key never appears in the returned (wrapped) error.
func TestGoogleProviderTransportErrorDoesNotLeakKey(t *testing.T) {
	p := NewGoogleProvider(GoogleConfig{
		APIKey:  googleTestKey,
		BaseURL: "https://translation.googleapis.com",
	})
	// Inject a transport that always fails with a *url.Error wrapping the URL.
	p.client = &http.Client{Transport: failingRoundTripper{}}

	_, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), googleTestKey,
		"transport error must not leak the API key")
}
