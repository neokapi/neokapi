package jobs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corestore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// fakeWorkspaceProviders serves one saved config per workspace.
type fakeWorkspaceProviders struct {
	saved      map[string]bstore.ProviderConfig // workspace id -> config, key included
	latestErr  error
	resolveErr error
}

func (f fakeWorkspaceProviders) Latest(_ context.Context, ws string) (bstore.ProviderConfig, error) {
	if f.latestErr != nil {
		return bstore.ProviderConfig{}, f.latestErr
	}
	cfg, ok := f.saved[ws]
	if !ok {
		return bstore.ProviderConfig{}, bstore.ErrProviderConfigNotFound
	}
	cfg.APIKey = ""
	return cfg, nil
}

func (f fakeWorkspaceProviders) Resolve(_ context.Context, ws, id string) (bstore.ProviderConfig, error) {
	if f.resolveErr != nil {
		return bstore.ProviderConfig{}, f.resolveErr
	}
	cfg, ok := f.saved[ws]
	if !ok || cfg.ID != id {
		return bstore.ProviderConfig{}, bstore.ErrProviderConfigNotFound
	}
	return cfg, nil
}

func TestResolveExtractionProviderPrefersTheWorkspaceKey(t *testing.T) {
	saved := map[string]bstore.ProviderConfig{
		"ws-byo": {ID: "cfg-1", WorkspaceID: "ws-byo", Type: "openai", APIKey: "sk-own"},
	}
	tests := []struct {
		name       string
		store      WorkspaceProviderStore
		workspace  string
		props      map[string]string
		wantSource ProviderSource
		wantName   aiprovider.ProviderID
		wantErr    bool
	}{
		{
			name:       "workspace with a saved config extracts on it",
			store:      fakeWorkspaceProviders{saved: saved},
			workspace:  "ws-byo",
			wantSource: ProviderSourceBYO,
			wantName:   aiprovider.OpenAI,
		},
		{
			name:       "workspace with none extracts on the platform",
			store:      fakeWorkspaceProviders{saved: saved},
			workspace:  "ws-other",
			wantSource: ProviderSourcePlatform,
			wantName:   aiprovider.Demo,
		},
		{
			name:       "no store wired extracts on the platform",
			workspace:  "ws-byo",
			wantSource: ProviderSourcePlatform,
			wantName:   aiprovider.Demo,
		},
		{
			name:       "job without a workspace id extracts on the platform",
			store:      fakeWorkspaceProviders{saved: saved},
			workspace:  "",
			wantSource: ProviderSourcePlatform,
			wantName:   aiprovider.Demo,
		},
		{
			name:       "a failed lookup falls back to the platform",
			store:      fakeWorkspaceProviders{saved: saved, latestErr: assert.AnError},
			workspace:  "ws-byo",
			wantSource: ProviderSourcePlatform,
			wantName:   aiprovider.Demo,
		},
		{
			name:      "a found config that cannot be resolved is an error",
			store:     fakeWorkspaceProviders{saved: saved, resolveErr: assert.AnError},
			workspace: "ws-byo",
			wantErr:   true,
		},
		{
			name:       "an explicit platform extraction_provider still prefers the workspace key",
			store:      fakeWorkspaceProviders{saved: saved},
			workspace:  "ws-byo",
			props:      map[string]string{"extraction_provider": "platform"},
			wantSource: ProviderSourceBYO,
			wantName:   aiprovider.OpenAI,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := &ExtractionWorkerDeps{
				Platform:      &PlatformProviderConfig{Provider: "demo"},
				ProviderStore: tc.store,
			}
			job := &ExtractionJob{ID: "job-1", WorkspaceID: tc.workspace}
			proj := &corestore.Project{ID: "proj-1", Properties: tc.props}

			got, err := resolveExtractionProvider(context.Background(), deps, job, proj)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantSource, got.Source)
			require.NotNil(t, got.LLM)
			assert.Equal(t, tc.wantName, got.LLM.Name())
		})
	}
}

func TestBYOModel(t *testing.T) {
	anthropic, ok := aiprovider.ProviderInfoFor(aiprovider.Anthropic)
	require.True(t, ok)
	tests := []struct {
		name string
		cfg  bstore.ProviderConfig
		want string
	}{
		{"config model wins", bstore.ProviderConfig{Type: "anthropic", Model: "claude-haiku-4-5"}, "claude-haiku-4-5"},
		{"provider default when unset", bstore.ProviderConfig{Type: "anthropic"}, anthropic.DefaultModel},
		{"unknown provider leaves it to the provider", bstore.ProviderConfig{Type: "nope"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, BYOModel(tc.cfg))
		})
	}
}
