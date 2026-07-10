package jobs

import (
	"context"
	"errors"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProviderResolver is an in-memory ProviderConfigResolver for exercising the
// worker's BYO resolve path without a database.
type fakeProviderResolver struct {
	cfg bstore.ProviderConfig
	err error

	gotWorkspaceID string
	gotID          string
}

func (f *fakeProviderResolver) Resolve(_ context.Context, workspaceID, id string) (bstore.ProviderConfig, error) {
	f.gotWorkspaceID, f.gotID = workspaceID, id
	if f.err != nil {
		return bstore.ProviderConfig{}, f.err
	}
	return f.cfg, nil
}

func TestResolveProvider_Platform(t *testing.T) {
	deps := &WorkerDeps{
		Platform: &PlatformProviderConfig{Provider: "demo"}, // offline, keyless
	}
	job := &TranslationJob{ProviderConfigID: "platform", Model: "any"}

	resolved, err := resolveProvider(t.Context(), deps, job)
	require.NoError(t, err)
	require.NotNil(t, resolved.LLM)
	require.NotNil(t, resolved.Limiter)
	assert.Equal(t, ProviderSourcePlatform, resolved.Source,
		"platform-key jobs are metered; source must be platform")
	_ = resolved.LLM.Close()
}

func TestResolveProvider_EmptyProviderIsPlatform(t *testing.T) {
	deps := &WorkerDeps{Platform: &PlatformProviderConfig{Provider: "demo"}}
	job := &TranslationJob{ProviderConfigID: ""} // empty == platform

	resolved, err := resolveProvider(t.Context(), deps, job)
	require.NoError(t, err)
	assert.Equal(t, ProviderSourcePlatform, resolved.Source)
	_ = resolved.LLM.Close()
}

func TestResolveProvider_BYO(t *testing.T) {
	fake := &fakeProviderResolver{
		cfg: bstore.ProviderConfig{
			Type:   "demo", // offline, keyless — constructs without network
			Model:  "cfg-model",
			APIKey: "sk-byo",
		},
	}
	deps := &WorkerDeps{ProviderStore: fake}
	job := &TranslationJob{
		ProviderConfigID: "cfg-123",
		WorkspaceID:      "ws-id",
		WorkspaceSlug:    "ws-slug",
		Model:            "job-model",
	}

	resolved, err := resolveProvider(t.Context(), deps, job)
	require.NoError(t, err)
	require.NotNil(t, resolved.LLM)
	require.NotNil(t, resolved.Limiter)
	assert.Equal(t, ProviderSourceBYO, resolved.Source,
		"BYO-key jobs burn no credits; source must be byo")

	// The resolve path scopes strictly to the job's durable workspace id and
	// config id — the slug is never used to authorize secret retrieval.
	assert.Equal(t, "ws-id", fake.gotWorkspaceID)
	assert.Equal(t, "cfg-123", fake.gotID)
	_ = resolved.LLM.Close()
}

func TestResolveProvider_BYO_NoStore(t *testing.T) {
	// A saved-config job with no provider store configured must fail clearly,
	// not silently fall back to a machine-global key.
	deps := &WorkerDeps{}
	job := &TranslationJob{ProviderConfigID: "cfg-123", WorkspaceID: "ws-id"}

	_, err := resolveProvider(t.Context(), deps, job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider store not configured")
}

func TestResolveProvider_BYO_ResolveError(t *testing.T) {
	fake := &fakeProviderResolver{err: errors.New("not found")}
	deps := &WorkerDeps{ProviderStore: fake}
	job := &TranslationJob{ProviderConfigID: "missing", WorkspaceID: "ws-id"}

	_, err := resolveProvider(t.Context(), deps, job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve provider config")
}

func TestResolveProvider_PlatformNotConfigured(t *testing.T) {
	deps := &WorkerDeps{} // no Platform
	job := &TranslationJob{ProviderConfigID: "platform"}

	_, err := resolveProvider(t.Context(), deps, job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "platform provider not configured")
}
