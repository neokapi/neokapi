package tools_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const beanSentence = "Mr Bean is the new King of England"

func beanRules() []redaction.Rule {
	return []redaction.Rule{
		{Term: "Mr Bean", Category: "person"},
		{Term: "King of England", Category: "role"},
	}
}

func newBeanBlock() *model.Block {
	b := model.NewBlock("b1", beanSentence)
	b.SourceLocale = "en"
	return b
}

func TestRedactTool_InProcess(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     beanRules(),
	})
	require.NoError(t, err)
	assert.Equal(t, "redact", tl.Name())

	block := newBeanBlock()
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	// Leak invariant: no secret survives in the serialized source.
	srcText := rb.SourceText()
	assert.NotContains(t, srcText, "Mr Bean")
	assert.NotContains(t, srcText, "King of England")
	assert.Contains(t, model.FlattenRuns(rb.SourceRuns()), "[REDACTED:Person]")
	assert.Contains(t, model.FlattenRuns(rb.SourceRuns()), "[REDACTED:Role]")

	// The originals live only on the in-process secret annotation.
	ann, ok := model.AnnoAs[*redaction.SecretAnnotation](rb, redaction.SecretAnnotationKey)
	require.True(t, ok, "secret annotation must be attached in-process")
	assert.Len(t, ann.Values, 2)
}

func TestRedactUnredact_InProcessRoundtrip(t *testing.T) {
	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     beanRules(),
	})
	require.NoError(t, err)

	block := newBeanBlock()
	processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})

	// Simulate a translation step that preserves the placeholders: copy the
	// redacted source runs into a target locale.
	block.SetTargetRuns("fr", block.SourceRuns())

	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})

	// Both source and target are restored; the secret annotation is gone.
	assert.Equal(t, beanSentence, block.SourceText())
	assert.Equal(t, beanSentence, block.TargetText("fr"))
	_, ok := block.Anno(redaction.SecretAnnotationKey)
	assert.False(t, ok, "secret annotation must be removed after unredact")
}

func TestRedactUnredact_ExternalVault(t *testing.T) {
	vaultPath := filepath.Join(t.TempDir(), "redaction", "batch1.json")

	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     beanRules(),
		VaultPath: vaultPath,
	})
	require.NoError(t, err)

	block := newBeanBlock()
	result := processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	// External mode: NO secret annotation on the block (would leak into XLIFF).
	_, hasAnn := rb.Anno(redaction.SecretAnnotationKey)
	assert.False(t, hasAnn, "external mode must not attach the secret annotation")
	assert.NotContains(t, rb.SourceText(), "Mr Bean")

	// The sidecar holds the originals.
	vault, err := redaction.OpenFileVault(vaultPath)
	require.NoError(t, err)
	assert.Len(t, vault.All(), 2)

	// Restore from the sidecar.
	rb.SetTargetRuns("fr", rb.SourceRuns())
	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{VaultPath: vaultPath})
	require.NoError(t, err)
	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: rb})
	assert.Equal(t, beanSentence, rb.TargetText("fr"))
}

func TestRedactTool_EntityDetection(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectEntities},
	})
	require.NoError(t, err)

	block := model.NewBlock("b1", "Alice met Bob")
	block.SourceLocale = "en"
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    "entity:0",
		Range: model.RunRangeForBytes(block.Source, 0, 5),
		Value: &model.EntityAnnotation{
			Text: "Alice",
			Type: model.EntityPerson,
		},
	})
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    "entity:1",
		Range: model.RunRangeForBytes(block.Source, 10, 13),
		Value: &model.EntityAnnotation{
			Text: "Bob",
			Type: model.EntityPerson,
		},
	})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)
	src := rb.SourceText()
	assert.NotContains(t, src, "Alice")
	assert.NotContains(t, src, "Bob")
	annv, _ := rb.Anno(redaction.SecretAnnotationKey)
	ann := annv.(*redaction.SecretAnnotation)
	assert.Len(t, ann.Values, 2)
}

// TestRedactTool_PreservesUpstreamTermOverlay proves a source-transform that
// rewrites the source now rebases the surviving source overlays (a term tag from
// an upstream annotator) rather than dropping them: the unrelated term span
// follows the redaction onto the new runs (AD-006 / model.RemapOverlays).
func TestRedactTool_PreservesUpstreamTermOverlay(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
	})
	require.NoError(t, err)

	block := newBeanBlock()
	start := strings.Index(beanSentence, "England")
	block.AddOverlaySpan(model.OverlayTerm, model.Span{
		ID:    "term:england",
		Range: model.RunRangeForBytes(block.Source, start, start+len("England")),
	})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.NotContains(t, rb.SourceText(), "Mr Bean")
	sp := rb.OverlaySpan(model.OverlayTerm, "term:england")
	require.NotNil(t, sp, "the unrelated term overlay must survive the redaction")
	assert.Equal(t, "England", model.RunsText(sp.Range.ExtractRuns(rb.Source)))
}

func TestRedactConfig_EntityCategoryValidation(t *testing.T) {
	// Friendly names and aliases (plurals, "organization") are accepted.
	require.NoError(t, (&tools.RedactConfig{EntityTypes: []string{"person", "dates", "organization"}}).Validate())
	// An unknown category is rejected with a helpful message.
	err := (&tools.RedactConfig{EntityTypes: []string{"bogus"}}).Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entity category")
}

// TestRedactTool_EntityTypesEnablesEntities shows the ergonomic options: naming
// categories to redact ("redact dates", "redact person names") enables entity
// detection without also listing the "entities" detector, and aliases/case
// normalize to canonical categories.
func TestRedactTool_EntityTypesEnablesEntities(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{EntityTypes: []string{"Dates", "people"}})
	require.NoError(t, err)

	block := model.NewBlock("b1", "Bob shipped on 2024-01-02")
	block.SourceLocale = "en"
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID: "e0", Range: model.RunRangeForBytes(block.Source, 0, 3),
		Value: &model.EntityAnnotation{Text: "Bob", Type: model.EntityPerson},
	})
	dateStart := strings.Index(block.SourceText(), "2024-01-02")
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID: "e1", Range: model.RunRangeForBytes(block.Source, dateStart, dateStart+len("2024-01-02")),
		Value: &model.EntityAnnotation{Text: "2024-01-02", Type: model.EntityDate},
	})

	rb := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block}).Resource.(*model.Block)
	assert.NotContains(t, rb.SourceText(), "Bob", "person redacted")
	assert.NotContains(t, rb.SourceText(), "2024-01-02", "date redacted")
}

// TestResolveRedactContract is the config-derived contract: entity detection
// makes the upstream entity overlay a required input, so a flow that redacts
// entities without an NER step fails data-flow validation instead of silently
// leaving PII unredacted. Rule-based detection requires no upstream port.
func TestResolveRedactContract(t *testing.T) {
	base := registry.ToolInfo{
		Consumes: []schema.IOPort{{Type: string(model.OverlayEntity), Side: model.SideSource, Optional: true}},
	}
	rulesOnly := tools.ResolveRedactContract(map[string]any{"detectors": []string{"rules"}}, base)
	assert.True(t, rulesOnly.Consumes[0].Optional, "rules-only: entity input stays optional")

	withEntities := tools.ResolveRedactContract(map[string]any{"detectors": []string{"entities"}}, base)
	assert.False(t, withEntities.Consumes[0].Optional, "entities detector: entity input is required")

	byCategories := tools.ResolveRedactContract(map[string]any{"entityTypes": []string{"person"}}, base)
	assert.False(t, byCategories.Consumes[0].Optional, "naming categories also requires the entity input")

	assert.True(t, base.Consumes[0].Optional, "resolver must not mutate the base contract")
}

func TestRedactTool_NoMatchesPassThrough(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Nonexistent", Category: "person"}},
	})
	require.NoError(t, err)

	block := newBeanBlock()
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)
	assert.Equal(t, beanSentence, rb.SourceText())
	_, ok := rb.Anno(redaction.SecretAnnotationKey)
	assert.False(t, ok)
}

func TestRedactConfig_Validate(t *testing.T) {
	cfg := &tools.RedactConfig{Detectors: []string{"bogus"}}
	require.Error(t, cfg.Validate())

	ok := &tools.RedactConfig{Detectors: []string{tools.DetectRules, tools.DetectEntities}}
	assert.NoError(t, ok.Validate())
}
