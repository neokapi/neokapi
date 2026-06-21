package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func optionValues(p schema.PropertySchema) []string {
	out := make([]string, 0, len(p.Options))
	for _, o := range p.Options {
		s, _ := o.Value.(string)
		out = append(out, s)
	}
	return out
}

// groupVisibleForField returns the group-level ui:visible condition of the group
// that contains field (master-detail gating is per group, not per field), or nil.
func groupVisibleForField(s *schema.ComponentSchema, field string) *schema.ConditionExpr {
	for i := range s.Groups {
		for _, f := range s.Groups[i].Fields {
			if f == field {
				return s.Groups[i].Visible
			}
		}
	}
	return nil
}

// TestQASchemaComposition asserts the unified `qa` tool composes a `mode`
// selector (Deterministic rules / AI review) where each mode reveals only its
// own config: rule toggles for rules, provider/model for AI.
func TestQASchemaComposition(t *testing.T) {
	s := QASchema()
	require.NotNil(t, s)

	mode, ok := s.Properties["mode"]
	require.True(t, ok, "mode selector present")
	assert.Equal(t, "select", mode.Widget)
	assert.Equal(t, "rules", mode.Default)
	assert.Equal(t, []string{"rules", "ai"}, optionValues(mode))

	// AI fields gated on mode==ai (at the group level).
	for _, f := range []string{"provider", "model", "checks"} {
		require.Contains(t, s.Properties, f)
		v := groupVisibleForField(s, f)
		require.NotNil(t, v, "field %q's group gated", f)
		assert.Equal(t, "mode", v.Field)
		assert.Equal(t, "ai", v.Eq)
	}
	// A representative rule toggle's group gated on mode==rules.
	rv := groupVisibleForField(s, "checkEmptyTarget")
	require.NotNil(t, rv, "rule group is gated")
	assert.Equal(t, "rules", rv.Eq)

	// Provider options come from the AI provider registry.
	assert.Contains(t, optionValues(s.Properties["provider"]), "anthropic")
}

// TestTranslateSchemaComposition asserts the unified `translate` tool gates each
// provider's extra credentials behind the provider selector, instead of showing
// every MT credential at once.
func TestTranslateSchemaComposition(t *testing.T) {
	s := TranslateSchema()
	require.NotNil(t, s)

	prov := s.Properties["provider"]
	assert.Equal(t, "select", prov.Widget)
	assert.Equal(t, "anthropic", prov.Default)
	vals := optionValues(prov)
	for _, want := range []string{"anthropic", "openai", "deepl", "google", "microsoft", "mymemory"} {
		assert.Contains(t, vals, want)
	}
	assert.NotContains(t, vals, "", "no empty provider option values")

	// Per-provider credentials gated on the matching provider (group level).
	for field, provider := range map[string]string{
		"subscriptionKey": "microsoft",
		"region":          "microsoft",
		"projectId":       "google",
		"email":           "mymemory",
	} {
		require.Contains(t, s.Properties, field)
		v := groupVisibleForField(s, field)
		require.NotNil(t, v, "field %q's group gated", field)
		assert.Equal(t, "provider", v.Field)
		assert.Equal(t, provider, v.Eq)
	}

	// Shared credential stays common (its group is ungated).
	assert.Nil(t, groupVisibleForField(s, "apiKey"), "apiKey is common")
}

// TestEntityExtractSchemaComposition asserts the ai-entity-extract tool gates
// the LLM provider/batch fields to the engines that use an LLM (llm + hybrid),
// while locale/known-terms stay common to every engine.
func TestEntityExtractSchemaComposition(t *testing.T) {
	s := AIEntityExtractSchema()
	require.NotNil(t, s)

	eng := s.Properties["engine"]
	assert.Equal(t, "select", eng.Widget)
	assert.Equal(t, EngineLLM, eng.Default)
	assert.ElementsMatch(t, []string{"llm", "ner", "hybrid"}, optionValues(eng))

	// LLM fields' group is visible for llm OR hybrid (shared config).
	for _, f := range []string{"provider", "model", "batchSize", "batchConcurrency"} {
		require.Contains(t, s.Properties, f)
		v := groupVisibleForField(s, f)
		require.NotNil(t, v, "field %q's group gated", f)
		require.Len(t, v.Any, 2, "field %q gated on two engines", f)
		eqs := []string{v.Any[0].Eq.(string), v.Any[1].Eq.(string)}
		assert.ElementsMatch(t, []string{EngineLLM, EngineHybrid}, eqs)
	}

	// Common fields' groups stay ungated.
	assert.Nil(t, groupVisibleForField(s, "locale"))
	assert.Nil(t, groupVisibleForField(s, "knownTerms"))

	// Produces contract survives composition (redact depends on it).
	require.NotNil(t, s.ToolMeta)
	assert.NotEmpty(t, s.ToolMeta.Produces)
}

// TestQAUsesAI covers explicit-mode dispatch and the back-compat fallback to
// provider presence.
func TestQAUsesAI(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
		ai     bool
	}{
		{"explicit rules wins over provider", map[string]any{"mode": "rules", "provider": "anthropic"}, false},
		{"explicit ai", map[string]any{"mode": "ai"}, true},
		{"unset + provider (back-compat)", map[string]any{"provider": "anthropic"}, true},
		{"unset + no provider", map[string]any{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.ai, qaUsesAI(c.config))
		})
	}
}
