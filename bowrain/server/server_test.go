package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoint(t *testing.T) {
	srv := NewServer(DefaultConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.NotEmpty(t, resp.Version)
}

func TestInfoEndpoint(t *testing.T) {
	srv := NewServer(DefaultConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp InfoResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Version)
	assert.NotEmpty(t, resp.Commit)
	assert.NotEmpty(t, resp.BuildDate)
	assert.Equal(t, "standalone", resp.Mode)
	assert.NotEmpty(t, resp.Formats)
	assert.NotNil(t, resp.Tools)
	assert.NotEmpty(t, resp.Locales)
}

func TestInfoEndpointServerMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp InfoResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "server", resp.Mode)
}

func TestInfoEndpointContainsFormatsAndLocales(t *testing.T) {
	srv := NewServer(DefaultConfig())
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp InfoResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Verify formats are populated and sorted.
	formatNames := make(map[string]FormatInfo)
	for _, f := range resp.Formats {
		formatNames[f.Name] = f
	}
	for _, name := range []string{"plaintext", "html", "json", "yaml"} {
		_, ok := formatNames[name]
		assert.True(t, ok, "expected format %q in /info response", name)
	}
	for i := 1; i < len(resp.Formats); i++ {
		assert.LessOrEqual(t, resp.Formats[i-1].Name, resp.Formats[i].Name, "formats should be sorted")
	}

	// Verify locales are populated.
	assert.Greater(t, len(resp.Locales), 10, "should have many locales")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "0.0.0.0", cfg.Host)
	assert.Empty(t, cfg.DataDir)
}

func TestRequestBaseURL(t *testing.T) {
	e := echo.New()

	tests := []struct {
		name           string
		host           string
		forwardedHost  string
		forwardedProto string
		want           string
	}{
		{
			name: "no forwarded headers",
			host: "localhost:8080",
			want: "http://localhost:8080",
		},
		{
			name:          "X-Forwarded-Host only",
			host:          "localhost:8080",
			forwardedHost: "bowrain.mymac",
			want:          "http://bowrain.mymac",
		},
		{
			name:           "X-Forwarded-Host and X-Forwarded-Proto",
			host:           "localhost:8080",
			forwardedHost:  "bowrain.mymac",
			forwardedProto: "https",
			want:           "https://bowrain.mymac",
		},
		{
			name:           "X-Forwarded-Proto only",
			host:           "localhost:8080",
			forwardedProto: "https",
			want:           "https://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, requestBaseURL(c))
		})
	}
}

func TestNewServerCreatesRegistries(t *testing.T) {
	srv := NewServer(DefaultConfig())
	assert.NotNil(t, srv.FormatRegistry)
	assert.NotNil(t, srv.ToolRegistry)
	// Verify formats were registered.
	assert.True(t, srv.FormatRegistry.HasReader("plaintext"))
	assert.True(t, srv.FormatRegistry.HasWriter("plaintext"))
}
