package platformconfig

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := NewStore(db)
	require.NoError(t, err)
	return s
}

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestStoreRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Empty table.
	all, err := s.GetAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	// Set + read back.
	require.NoError(t, s.Set(ctx, KeyAIProvider, raw(t, "bedrock"), "admin@example.com"))
	all, err = s.GetAll(ctx)
	require.NoError(t, err)
	require.Contains(t, all, KeyAIProvider)
	assert.JSONEq(t, `"bedrock"`, string(all[KeyAIProvider]))

	// Upsert overwrites.
	require.NoError(t, s.Set(ctx, KeyAIProvider, raw(t, "openai"), "admin@example.com"))
	all, err = s.GetAll(ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `"openai"`, string(all[KeyAIProvider]))

	// Delete resets to default (absent).
	require.NoError(t, s.Delete(ctx, KeyAIProvider))
	all, err = s.GetAll(ctx)
	require.NoError(t, err)
	assert.NotContains(t, all, KeyAIProvider)
}

func TestStoreSetManyAtomic(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	require.NoError(t, s.SetMany(ctx, map[string]json.RawMessage{
		KeyAIDefaultModel:   raw(t, "eu.anthropic.claude-sonnet-4-6"),
		KeyAIEnabledModels:  raw(t, []string{"eu.anthropic.claude-sonnet-4-6", "eu.anthropic.claude-opus-4-6-v1"}),
		KeyAICustomerChoice: raw(t, true),
		KeyTrialDays:        raw(t, 30),
	}, "admin@example.com"))

	all, err := s.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 4)
	assert.JSONEq(t, `"eu.anthropic.claude-sonnet-4-6"`, string(all[KeyAIDefaultModel]))
	assert.JSONEq(t, `30`, string(all[KeyTrialDays]))
}

func TestStoreRejectsInvalidJSON(t *testing.T) {
	s := newTestStore(t)
	err := s.Set(t.Context(), KeyAIProvider, json.RawMessage("not json"), "admin")
	require.Error(t, err)
}

// TestServiceWithStore exercises the Service against a real store: a write is
// visible after Refresh, and a Delete falls back to the bootstrap default.
func TestServiceWithStore(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	svc := NewService(store, testDefaults())

	// Before any write, accessors serve defaults.
	require.NoError(t, svc.Refresh(ctx))
	assert.Equal(t, "eu.anthropic.claude-sonnet-4-6", svc.AIDefaultModel())

	// Set + refresh via the Service.
	require.NoError(t, svc.Set(ctx, map[string]json.RawMessage{
		KeyAIDefaultModel:   raw(t, "eu.anthropic.claude-opus-4-6-v1"),
		KeyAICustomerChoice: raw(t, true),
		KeyAIEnabledModels:  raw(t, []string{"eu.anthropic.claude-sonnet-4-6", "eu.anthropic.claude-opus-4-6-v1"}),
	}, "admin@example.com"))
	assert.Equal(t, "eu.anthropic.claude-opus-4-6-v1", svc.AIDefaultModel())
	assert.True(t, svc.AICustomerChoice())
	assert.True(t, svc.IsModelEnabled("eu.anthropic.claude-sonnet-4-6"))

	// PublicSnapshot exposes the enabled models when customer choice is on.
	ps := svc.PublicSnapshot()
	assert.True(t, ps.CustomerChoice)
	assert.Len(t, ps.EnabledModels, 2)

	// Delete resets to the bootstrap default.
	require.NoError(t, svc.Delete(ctx, KeyAIDefaultModel))
	assert.Equal(t, "eu.anthropic.claude-sonnet-4-6", svc.AIDefaultModel())
}
