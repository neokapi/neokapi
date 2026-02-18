package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTargetLocales(t *testing.T) {
	tests := []struct {
		input string
		want  []model.LocaleID
	}{
		{"", nil},
		{"nb", []model.LocaleID{"nb"}},
		{"nb,fr", []model.LocaleID{"nb", "fr"}},
		{"nb, fr, de", []model.LocaleID{"nb", "fr", "de"}},
		{" nb , fr , ", []model.LocaleID{"nb", "fr"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTargetLocales(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunInitQuickStart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/projects/anonymous", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-project", req["name"])
		assert.Equal(t, "en", req["source_locale"])

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"project_id":  "proj_qs_123",
			"claim_token": "clm_qs_abc",
		})
	}))
	defer srv.Close()

	dir := t.TempDir()

	cfg := project.DefaultConfig()
	cfg.Project.Name = "test-project"
	cfg.Project.SourceLocale = "en"
	cfg.Project.TargetLocales = []model.LocaleID{"nb", "fr"}

	err := runInitQuickStart(dir, cfg, withServerURL(srv.URL))
	require.NoError(t, err)

	// Verify .kapi/ directory was created.
	kapiDir := filepath.Join(dir, ".kapi")
	_, err = os.Stat(kapiDir)
	require.NoError(t, err)

	// Load and verify config.
	loadedCfg, err := project.LoadConfig(kapiDir)
	require.NoError(t, err)
	assert.Equal(t, "test-project", loadedCfg.Project.Name)
	assert.Equal(t, model.LocaleID("en"), loadedCfg.Project.SourceLocale)
	assert.Equal(t, []model.LocaleID{"nb", "fr"}, loadedCfg.Project.TargetLocales)
	require.NotNil(t, loadedCfg.Server)
	assert.Equal(t, srv.URL, loadedCfg.Server.URL)
	assert.Equal(t, "proj_qs_123", loadedCfg.Server.ProjectID)
	assert.Equal(t, "clm_qs_abc", loadedCfg.Server.ClaimToken)
}
