package platformconfig

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDefaults() Defaults {
	return Defaults{
		AIProvider:     "bedrock",
		AIDefaultModel: "eu.anthropic.claude-sonnet-4-6",
		SignupsOpen:    true,
		DefaultPlan:    "pro",
		TrialDays:      14,
	}
}

// withCache builds a Service with a pre-populated cache (bypassing the DB) for
// white-box testing of the accessor + fallback logic.
func withCache(t *testing.T, kv map[string]any) *Service {
	t.Helper()
	svc := NewService(nil, testDefaults())
	cache := map[string]json.RawMessage{}
	for k, v := range kv {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		cache[k] = b
	}
	svc.cache = cache
	return svc
}

func TestAccessorsFallBackToDefaults(t *testing.T) {
	svc := withCache(t, nil) // empty cache → everything is default
	assert.Equal(t, "bedrock", svc.AIProvider())
	assert.Equal(t, "eu.anthropic.claude-sonnet-4-6", svc.AIDefaultModel())
	assert.True(t, svc.SignupsOpen())
	assert.Equal(t, "pro", svc.DefaultPlan())
	assert.Equal(t, 14, svc.TrialDays())
	assert.False(t, svc.AICustomerChoice())
	assert.False(t, svc.Maintenance().Enabled)
	// Enabled models fall back to the single default model.
	assert.Equal(t, []string{"eu.anthropic.claude-sonnet-4-6"}, svc.AIEnabledModels())
}

func TestDBValuesOverrideDefaults(t *testing.T) {
	svc := withCache(t, map[string]any{
		KeyAIDefaultModel:   "eu.anthropic.claude-opus-4-6-v1",
		KeyAIEnabledModels:  []string{"eu.anthropic.claude-sonnet-4-6", "eu.anthropic.claude-opus-4-6-v1"},
		KeyAICustomerChoice: true,
		KeySignupsOpen:      false,
		KeyMaintenanceOn:    true,
		KeyMaintenanceMsg:   "Back at 15:00 UTC",
		KeyTrialDays:        30,
		KeyDefaultPlan:      "team",
		KeyFeatures:         map[string]bool{"bravo": true},
	})
	assert.Equal(t, "eu.anthropic.claude-opus-4-6-v1", svc.AIDefaultModel())
	assert.Len(t, svc.AIEnabledModels(), 2)
	assert.True(t, svc.IsModelEnabled("eu.anthropic.claude-opus-4-6-v1"))
	assert.False(t, svc.IsModelEnabled("eu.anthropic.claude-sonnet-5"))
	assert.True(t, svc.AICustomerChoice())
	assert.False(t, svc.SignupsOpen())
	assert.Equal(t, Maintenance{Enabled: true, Message: "Back at 15:00 UTC"}, svc.Maintenance())
	assert.Equal(t, 30, svc.TrialDays())
	assert.Equal(t, "team", svc.DefaultPlan())
	assert.Equal(t, map[string]bool{"bravo": true}, svc.GlobalFeatures())
}

func TestInvalidOrZeroValuesFallBack(t *testing.T) {
	svc := withCache(t, map[string]any{
		KeyAIProvider: "", // empty string → default
		KeyTrialDays:  0,  // non-positive → default
	})
	assert.Equal(t, "bedrock", svc.AIProvider())
	assert.Equal(t, 14, svc.TrialDays())
}

func TestPublicSnapshotHidesModelsUnlessCustomerChoice(t *testing.T) {
	// customer choice off → no model list exposed publicly
	off := withCache(t, map[string]any{KeyAICustomerChoice: false, KeyMaintenanceOn: true, KeyMaintenanceMsg: "hi"})
	ps := off.PublicSnapshot()
	assert.False(t, ps.CustomerChoice)
	assert.Empty(t, ps.EnabledModels)
	assert.True(t, ps.Maintenance.Enabled)
	assert.Equal(t, "hi", ps.Maintenance.Message)

	// customer choice on → enabled models exposed, resolved against the catalog
	on := withCache(t, map[string]any{
		KeyAICustomerChoice: true,
		KeyAIEnabledModels:  []string{"eu.anthropic.claude-sonnet-4-6", "custom.model.id"},
	})
	ps = on.PublicSnapshot()
	assert.True(t, ps.CustomerChoice)
	require.Len(t, ps.EnabledModels, 2)
	assert.Equal(t, "Claude Sonnet 4.6", ps.EnabledModels[0].Name) // catalog-resolved
	assert.Equal(t, "custom.model.id", ps.EnabledModels[1].Name)   // unknown → bare id
}

func TestSnapshotIncludesCatalog(t *testing.T) {
	svc := withCache(t, nil)
	snap := svc.Snapshot()
	assert.Equal(t, "bedrock", snap.AI.Provider)
	assert.NotEmpty(t, snap.ModelCatalog)
	assert.Equal(t, BedrockAnthropicCatalog, snap.ModelCatalog)
}

func TestCatalogHelpers(t *testing.T) {
	assert.True(t, InCatalog("bedrock", "eu.anthropic.claude-sonnet-4-6"))
	assert.False(t, InCatalog("bedrock", "eu.anthropic.claude-nonexistent"))
	assert.Nil(t, Catalog("openai"))
	// Marketplace-gated models are flagged.
	for _, m := range BedrockAnthropicCatalog {
		if m.ID == "eu.anthropic.claude-sonnet-5" {
			assert.True(t, m.RequiresMarketplace)
		}
		if m.ID == "eu.anthropic.claude-sonnet-4-6" {
			assert.False(t, m.RequiresMarketplace)
		}
	}
}
