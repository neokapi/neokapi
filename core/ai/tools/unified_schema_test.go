package tools

import (
	"slices"
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
		if slices.Contains(s.Groups[i].Fields, field) {
			return s.Groups[i].Visible
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

// optionSetValues returns the option values of the option-set whose When gates
// on field==value, for asserting a cascading select's per-branch options.
func optionSetValues(p schema.PropertySchema, field, value string) []string {
	for _, set := range p.OptionSets {
		if set.When != nil && set.When.Field == field && set.When.Eq == value {
			out := make([]string, 0, len(set.Options))
			for _, o := range set.Options {
				s, _ := o.Value.(string)
				out = append(out, s)
			}
			return out
		}
	}
	return nil
}

// TestTranslateSchemaComposition asserts the unified `translate` tool is a
// two-level engine→provider group: an engine selector (LLM / MT) whose provider
// options cascade off the engine, and each MT provider's extra credentials shown
// only when both the MT engine and that provider are selected.
func TestTranslateSchemaComposition(t *testing.T) {
	s := TranslateSchema()
	require.NotNil(t, s)

	// Engine discriminator.
	eng := s.Properties["engine"]
	assert.Equal(t, "select", eng.Widget)
	assert.Equal(t, "llm", eng.Default)
	assert.Equal(t, []string{"llm", "mt"}, optionValues(eng))

	// Provider selector keeps the flat union (CLI/docs) and cascading option-sets.
	prov := s.Properties["provider"]
	assert.Equal(t, "select", prov.Widget)
	assert.Equal(t, "anthropic", prov.Default)
	vals := optionValues(prov)
	for _, want := range []string{"anthropic", "openai", "deepl", "google", "microsoft", "mymemory"} {
		assert.Contains(t, vals, want)
	}
	assert.NotContains(t, vals, "", "no empty provider option values")

	// Cascading option-sets: LLM engine offers LLM providers, MT engine offers MT.
	assert.Contains(t, optionSetValues(prov, "engine", "llm"), "anthropic")
	assert.NotContains(t, optionSetValues(prov, "engine", "llm"), "deepl")
	assert.Contains(t, optionSetValues(prov, "engine", "mt"), "deepl")
	assert.NotContains(t, optionSetValues(prov, "engine", "mt"), "anthropic")

	// The MT credentials live in one section gated on the MT engine...
	for _, field := range []string{"subscriptionKey", "region", "projectId", "email"} {
		require.Contains(t, s.Properties, field)
		g := groupVisibleForField(s, field)
		require.NotNil(t, g, "field %q's group gated", field)
		assert.Equal(t, "engine", g.Field)
		assert.Equal(t, "mt", g.Eq)
	}

	// ...and each credential field is further gated on its own provider.
	for field, provider := range map[string]string{
		"subscriptionKey": "microsoft",
		"region":          "microsoft",
		"projectId":       "google",
		"email":           "mymemory",
	} {
		v := s.Properties[field].Visible
		require.NotNil(t, v, "field %q gated on its provider", field)
		assert.Equal(t, "provider", v.Field)
		assert.Equal(t, provider, v.Eq)
	}

	// Shared credential stays common (its group is ungated).
	assert.Nil(t, groupVisibleForField(s, "apiKey"), "apiKey is common")
}

// TestEntityExtractSchemaComposition asserts the entity-extract tool gates
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
