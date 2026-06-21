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

	// AI fields gated on mode==ai.
	for _, f := range []string{"provider", "model", "checks"} {
		p, ok := s.Properties[f]
		require.True(t, ok, "field %q present", f)
		require.NotNil(t, p.Visible, "field %q gated", f)
		assert.Equal(t, "mode", p.Visible.Field)
		assert.Equal(t, "ai", p.Visible.Eq)
	}
	// A representative rule toggle gated on mode==rules.
	chk := s.Properties["checkEmptyTarget"]
	require.NotNil(t, chk.Visible, "rule toggle is gated")
	assert.Equal(t, "rules", chk.Visible.Eq)

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

	// Per-provider credentials gated on the matching provider.
	for field, provider := range map[string]string{
		"subscriptionKey": "microsoft",
		"region":          "microsoft",
		"projectId":       "google",
		"email":           "mymemory",
	} {
		p, ok := s.Properties[field]
		require.True(t, ok, "field %q present", field)
		require.NotNil(t, p.Visible, "field %q gated", field)
		assert.Equal(t, "provider", p.Visible.Field)
		assert.Equal(t, provider, p.Visible.Eq)
	}

	// Shared credential stays common (always visible).
	assert.Nil(t, s.Properties["apiKey"].Visible, "apiKey is common")
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

	// LLM fields are visible for llm OR hybrid (shared config).
	for _, f := range []string{"provider", "model", "batchSize", "batchConcurrency"} {
		p, ok := s.Properties[f]
		require.True(t, ok, "field %q present", f)
		require.NotNil(t, p.Visible, "field %q gated", f)
		require.Len(t, p.Visible.Any, 2, "field %q gated on two engines", f)
		eqs := []string{p.Visible.Any[0].Eq.(string), p.Visible.Any[1].Eq.(string)}
		assert.ElementsMatch(t, []string{EngineLLM, EngineHybrid}, eqs)
	}

	// Common fields stay ungated.
	assert.Nil(t, s.Properties["locale"].Visible)
	assert.Nil(t, s.Properties["knownTerms"].Visible)

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
