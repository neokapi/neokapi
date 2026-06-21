package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeVariants(t *testing.T) {
	type base struct {
		Engine string `json:"engine" schema:"title=Engine,group=main,order=0"`
		Scope  bool   `json:"scope,omitempty" schema:"title=Scope,group=main,order=10"`
	}
	type srxParams struct {
		RulesPath string `json:"rulesPath,omitempty" schema:"title=Rules File,order=10"`
	}
	type satParams struct {
		Model     string  `json:"model,omitempty" schema:"title=Model,order=10"`
		Threshold float64 `json:"threshold,omitempty" schema:"title=Threshold,order=20"`
	}

	b := FromStruct(&base{}, ToolMeta{ID: "seg"})
	variants := []Variant{
		{Name: "srx", Label: "Default", Description: "rule-based", Params: FromStruct(&srxParams{}, ToolMeta{ID: "v-srx"})},
		{Name: "uax29", Label: "Unicode"}, // no params
		{Name: "sat", Label: "ML", Description: "model", Params: FromStruct(&satParams{}, ToolMeta{ID: "v-sat"})},
	}

	s := ComposeVariants(b, "engine", "srx", variants)

	// Discriminator became a labeled select with all options + default.
	eng := s.Properties["engine"]
	assert.Equal(t, "select", eng.Widget)
	assert.Equal(t, "srx", eng.Default)
	require.Len(t, eng.Options, 3)
	assert.Equal(t, "srx", eng.Options[0].Value)
	assert.Equal(t, "Default", eng.Options[0].Label)
	assert.Equal(t, "rule-based", eng.EnumDescriptions["srx"])

	// Each variant's fields are merged and gated on the discriminator.
	for field, engine := range map[string]string{"rulesPath": "srx", "model": "sat", "threshold": "sat"} {
		p, ok := s.Properties[field]
		require.True(t, ok, "field %q present", field)
		require.NotNil(t, p.Visible, "field %q gated", field)
		assert.Equal(t, "engine", p.Visible.Field)
		assert.Equal(t, engine, p.Visible.Eq)
	}

	// Common fields stay ungated.
	assert.Nil(t, s.Properties["scope"].Visible)

	// Variant groups are inserted right after the discriminator's group, and the
	// parameterless variant (uax29) contributes no group.
	ids := make([]string, 0, len(s.Groups))
	for _, g := range s.Groups {
		ids = append(ids, g.ID)
	}
	assert.Equal(t, []string{"main", "srx", "sat"}, ids)

	// The base schema was not mutated.
	assert.Nil(t, b.Properties["rulesPath"].Visible)
	_, hadField := b.Properties["model"]
	assert.False(t, hadField, "base schema untouched")
}

func TestComposeVariantsTrailingInsertWhenNoDiscriminatorGroup(t *testing.T) {
	// When no group holds the discriminator, variant groups append at the end.
	b := &ComponentSchema{
		Type:       "object",
		Properties: map[string]PropertySchema{"engine": {Type: "string"}},
	}
	v := []Variant{{Name: "a", Label: "A", Params: &ComponentSchema{Properties: map[string]PropertySchema{"x": {Type: "string"}}}}}
	s := ComposeVariants(b, "engine", "a", v)
	require.Len(t, s.Groups, 1)
	assert.Equal(t, "a", s.Groups[0].ID)
}
