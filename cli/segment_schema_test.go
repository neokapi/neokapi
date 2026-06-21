package cli

import (
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/segment"
	tools "github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Link the LLM engine (core/ai/tools) so the composed schema reflects it
	// alongside the framework's srx/uax29. The SaT engine is now plugin-provided
	// (Mode-C daemon) and is wired at plugin-discovery time, not linked here.
	_ "github.com/neokapi/neokapi/core/ai/tools"
)

// TestSegmentationSchemaComposition asserts the segmentation tool's schema is
// composed from the registered engines: one labeled selector option per engine
// (srx default first), each engine's own parameters gated to appear only when
// that engine is selected.
func TestSegmentationSchemaComposition(t *testing.T) {
	s := tools.SegmentationSchema()
	require.NotNil(t, s)

	engine, ok := s.Properties["engine"]
	require.True(t, ok, "engine selector present")
	assert.Equal(t, "select", engine.Widget)
	assert.Equal(t, segment.DefaultEngine, engine.Default)

	// Every registered engine appears as a labeled option with a description.
	got := map[string]string{}
	for _, o := range engine.Options {
		got[o.Value.(string)] = o.Label
	}
	for _, d := range segment.Descriptors() {
		assert.Contains(t, got, d.Name, "engine %q is an option", d.Name)
		assert.Equal(t, d.Label, got[d.Name])
		assert.NotEmpty(t, engine.EnumDescriptions[d.Name], "engine %q has a description", d.Name)
	}
	// The built-ins linked into this binary.
	for _, name := range []string{"srx", "uax29", "llm"} {
		assert.Contains(t, got, name)
	}

	// Engine-specific fields are gated at the group level (master-detail).
	groupVisibleForField := func(field string) *schema.ConditionExpr {
		for i := range s.Groups {
			if slices.Contains(s.Groups[i].Fields, field) {
				return s.Groups[i].Visible
			}
		}
		return nil
	}
	gated := func(field, engineName string) {
		t.Helper()
		require.Contains(t, s.Properties, field)
		v := groupVisibleForField(field)
		require.NotNil(t, v, "field %q's group is conditionally visible", field)
		assert.Equal(t, "engine", v.Field)
		assert.Equal(t, engineName, v.Eq)
	}
	gated("rulesPath", "srx") // SRX engine
	gated("provider", "llm")  // LLM engine
	gated("model", "llm")     // LLM engine

	// uax29 has no parameters → no group.
	for _, g := range s.Groups {
		if g.ID == "uax29" {
			t.Fatalf("uax29 should contribute no parameter group")
		}
	}

	// Common boundary options are not gated (always visible).
	for _, field := range []string{"segmentSource", "trimLeadingWhitespace"} {
		assert.Nil(t, groupVisibleForField(field), "common field %q is always visible", field)
	}
}
