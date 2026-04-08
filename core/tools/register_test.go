package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
)

func TestAllToolsHaveCardinality(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	for _, info := range reg.ListWithSchemas() {
		assert.NotEmpty(t, info.Cardinality,
			"tool %q has no Cardinality set", info.Name)
		switch info.Cardinality {
		case schema.Monolingual, schema.Bilingual, schema.Multilingual:
			// valid
		default:
			t.Errorf("tool %q has invalid Cardinality %q", info.Name, info.Cardinality)
		}
	}
}

func TestPseudoTranslateHasDefaultLocale(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	for _, info := range reg.ListWithSchemas() {
		if info.Name == "pseudo-translate" {
			assert.Equal(t, schema.Bilingual, info.Cardinality)
			assert.Equal(t, model.LocaleID("qps"), info.DefaultLocale)
			assert.Contains(t, info.Produces, schema.AnnotationTranslation)
			return
		}
	}
	t.Fatal("pseudo-translate not found in registry")
}

func TestBilingualToolCount(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	var bilingual []string
	for _, info := range reg.ListWithSchemas() {
		if info.Cardinality == schema.Bilingual {
			bilingual = append(bilingual, string(info.Name))
		}
	}
	// Sanity check: we have a reasonable number of bilingual tools.
	assert.GreaterOrEqual(t, len(bilingual), 10,
		"expected at least 10 bilingual tools, got: %v", bilingual)
}

func TestTMLeverageHasSideEffects(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	for _, info := range reg.ListWithSchemas() {
		if info.Name == "tm-leverage" {
			assert.Contains(t, info.SideEffects, schema.SideEffectTMRead)
			assert.Contains(t, info.Produces, schema.AnnotationTMMatch)
			return
		}
	}
	t.Fatal("tm-leverage not found")
}
