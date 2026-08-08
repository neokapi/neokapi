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
			assert.Contains(t, info.Produces, schema.IOPort{Type: schema.PortTarget, Side: model.SideTarget})
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

func TestMemoryLeverageHasSideEffects(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	for _, info := range reg.ListWithSchemas() {
		if info.Name == "recycle" {
			assert.Contains(t, info.SideEffects, schema.SideEffectMemoryRead)
			assert.Contains(t, info.Produces, schema.IOPort{Type: model.AnnoMemoryMatch, Side: model.SideSource})
			return
		}
	}
	t.Fatal("recycle not found")
}

// TestIsSourceTransformProbe verifies the registry's capability probe: tools
// that rewrite source (Transform handler) report IsSourceTransform=true, while
// annotators and target-writers report false. This is the DRY source of truth
// the flow editors use to gate the source-transform stage.
func TestIsSourceTransformProbe(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	byName := map[string]registry.ToolInfo{}
	for _, info := range reg.ListWithSchemas() {
		byName[string(info.Name)] = info
	}

	// Source-transform-capable (Transform handler).
	for _, name := range []string{"redact", "case-transform", "search-replace"} {
		info, ok := byName[name]
		if !ok {
			continue // tool not registered in this build; skip
		}
		assert.True(t, info.IsSourceTransform, "tool %q should be source-transform-capable", name)
	}

	// Not source transforms: annotators and target-writers.
	for _, name := range []string{"word-count", "qa", "create-target", "pseudo-translate"} {
		info, ok := byName[name]
		if !ok {
			continue
		}
		assert.False(t, info.IsSourceTransform, "tool %q should NOT be source-transform-capable", name)
	}
}

// TestToolRequiresUseCanonicalTokens guards against a requires-token
// split-brain: the flow-path resource guards (host/flow.go, host/flowrun.go)
// switch on the schema.Requires* constants to inject the content memory and the
// terms store, so a registration that declares any other spelling is invisible
// to them — the requirement gate silently does nothing.
func TestToolRequiresUseCanonicalTokens(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	canonical := map[string]bool{
		schema.RequiresTargetLanguage: true,
		schema.RequiresSourceLanguage: true,
		schema.RequiresMemory:         true,
		schema.RequiresTerms:          true,
		schema.RequiresCredentials:    true,
		schema.RequiresRetryable:      true,
	}
	for _, info := range reg.ListWithSchemas() {
		for _, req := range info.Requires {
			assert.Truef(t, canonical[req],
				"tool %q declares requires token %q, which no flow guard recognises; declare a schema.Requires* constant instead",
				info.Name, req)
		}
	}
}

// TestMemoryAndTermsToolsMatchFlowGuards pins the two built-ins whose flow-path
// gating the split-brain had killed: recycle must satisfy the RequiresMemory
// guard (host/flowrun.go injectMemory) and term-check the RequiresTerms guard
// (host/flow.go), or a flow run leverages neither store while reporting success.
func TestMemoryAndTermsToolsMatchFlowGuards(t *testing.T) {
	reg := registry.NewToolRegistry()
	RegisterAll(reg)

	requires := func(name string) []string {
		s := reg.Schema(registry.ToolID(name))
		if s == nil || s.ToolMeta == nil {
			t.Fatalf("no schema for tool %q", name)
		}
		return s.ToolMeta.Requires
	}
	assert.Contains(t, requires("recycle"), schema.RequiresMemory,
		"recycle must declare RequiresMemory so the flow content-memory guard fires")
	assert.Contains(t, requires("term-check"), schema.RequiresTerms,
		"term-check must declare RequiresTerms so the flow terms guard fires")
}
