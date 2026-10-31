package tools_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
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

// Two files whose blocks share an ID redact into one sidecar vault, each
// under its own document, and each restores its own original.
func TestRedactUnredact_ExternalVaultKeepsEachDocument(t *testing.T) {
	vaultPath := filepath.Join(t.TempDir(), "batch.json")
	rules := []redaction.Rule{{Term: "Alice Archer", Category: "person"}, {Term: "Bruno Black", Category: "person"}}
	names := map[string]string{"src/a.json": "Alice Archer", "src/b.json": "Bruno Black"}
	redacted := map[string]*model.Block{}
	for _, doc := range []string{"src/a.json", "src/b.json"} {
		rt, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules}, Rules: rules, VaultPath: vaultPath, Document: doc})
		require.NoError(t, err)
		b := model.NewBlock("greeting", "Hello "+names[doc])
		b.SourceLocale = "en"
		rb := processPart(t, rt, &model.Part{Type: model.PartBlock, Resource: b}).Resource.(*model.Block)
		assert.NotContains(t, rb.SourceText(), names[doc])
		redacted[doc] = rb
	}

	vault, err := redaction.OpenFileVault(vaultPath)
	require.NoError(t, err)
	require.Len(t, vault.All(), 2, "the second file's original must not overwrite the first's")

	for doc, rb := range redacted {
		rb.SetTargetRuns("fr", rb.SourceRuns())
		ut, err := tools.NewUnredactTool(&tools.UnredactConfig{VaultPath: vaultPath, Document: doc})
		require.NoError(t, err)
		processPart(t, ut, &model.Part{Type: model.PartBlock, Resource: rb})
		assert.Equal(t, "Hello "+names[doc], rb.SourceText(), doc)
		assert.Equal(t, "Hello "+names[doc], rb.TargetText("fr"), doc)
	}
}

// In process, the originals ride on each block, so two blocks with one ID
// that one redact tool rewrites each restore their own.
func TestRedactUnredact_InProcessBlocksWithOneID(t *testing.T) {
	rt, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules},
		Rules: []redaction.Rule{{Term: "Alice Archer", Category: "person"}, {Term: "Bruno Black", Category: "person"}}})
	require.NoError(t, err)
	ut, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	a := model.NewBlock("greeting", "Hello Alice Archer")
	b := model.NewBlock("greeting", "Hello Bruno Black")
	for _, blk := range []*model.Block{a, b} {
		blk.SourceLocale = "en"
		processPart(t, rt, &model.Part{Type: model.PartBlock, Resource: blk})
		blk.SetTargetRuns("fr", blk.SourceRuns())
	}
	for _, blk := range []*model.Block{a, b} {
		processPart(t, ut, &model.Part{Type: model.PartBlock, Resource: blk})
	}
	assert.Equal(t, "Hello Alice Archer", a.TargetText("fr"))
	assert.Equal(t, "Hello Bruno Black", b.TargetText("fr"))
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
		Range: model.RangeAnchorForBytes(block.SourceRuns(), 0, 5),
		Value: &model.EntityAnnotation{
			Text: "Alice",
			Type: model.EntityPerson,
		},
	})
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    "entity:1",
		Range: model.RangeAnchorForBytes(block.SourceRuns(), 10, 13),
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
		Range: model.RangeAnchorForBytes(block.SourceRuns(), start, start+len("England")),
	})

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)

	assert.NotContains(t, rb.SourceText(), "Mr Bean")
	sp := rb.OverlaySpan(model.OverlayTerm, "term:england")
	require.NotNil(t, sp, "the unrelated term overlay must survive the redaction")
	assert.Equal(t, "England", model.RunsText(sp.Range.ExtractRuns(rb.SourceRuns())))
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
		ID: "e0", Range: model.RangeAnchorForBytes(block.SourceRuns(), 0, 3),
		Value: &model.EntityAnnotation{Text: "Bob", Type: model.EntityPerson},
	})
	dateStart := strings.Index(block.SourceText(), "2024-01-02")
	block.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID: "e1", Range: model.RangeAnchorForBytes(block.SourceRuns(), dateStart, dateStart+len("2024-01-02")),
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
	// The block withholds nothing, and still carries the policy, for the
	// term rules or prior version a request sends beside it.
	entry, ok := model.AnnoAs[*redaction.SecretAnnotation](rb, redaction.SecretAnnotationKey)
	require.True(t, ok)
	assert.Empty(t, entry.Values)
	assert.NotNil(t, entry.Policy)
}

// In external mode the block goes to a file, not to a provider, and carries
// no policy.
func TestRedactTool_ExternalModeLeavesNoPolicy(t *testing.T) {
	tl, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Nonexistent", Category: "person"}},
		VaultPath: filepath.Join(t.TempDir(), "vault.json"),
	})
	require.NoError(t, err)

	block := newBeanBlock()
	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	_, ok := result.Resource.(*model.Block).Anno(redaction.SecretAnnotationKey)
	assert.False(t, ok)
}

func TestRedactConfig_Validate(t *testing.T) {
	cfg := &tools.RedactConfig{Detectors: []string{"bogus"}}
	require.Error(t, cfg.Validate())

	ok := &tools.RedactConfig{Detectors: []string{tools.DetectRules, tools.DetectEntities}}
	assert.NoError(t, ok.Validate())
}

// WholeBlock redacts a block marked not translatable and each translation
// beside the source, each translation under tokens of its own, and unredact
// restores every side.
func TestRedactUnredact_WholeBlock(t *testing.T) {
	const target = "Mr Bean est le nouveau King of England"
	newBlock := func() *model.Block {
		b := newBeanBlock()
		b.Translatable = false
		b.SetTargetText("fr", target)
		return b
	}

	plain, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules}, Rules: beanRules()})
	require.NoError(t, err)
	untouched := newBlock()
	processPart(t, plain, &model.Part{Type: model.PartBlock, Resource: untouched})
	assert.Equal(t, beanSentence, untouched.SourceText(), "without WholeBlock a block marked not translatable is left alone")

	whole, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules}, Rules: beanRules(), WholeBlock: true})
	require.NoError(t, err)
	block := newBlock()
	processPart(t, whole, &model.Part{Type: model.PartBlock, Resource: block})
	assert.NotContains(t, block.SourceText(), "Mr Bean")
	assert.NotContains(t, model.RunsText(block.TargetRuns("fr")), "Mr Bean")
	ann, ok := model.AnnoAs[*redaction.SecretAnnotation](block, redaction.SecretAnnotationKey)
	require.True(t, ok)
	assert.Len(t, ann.Values, 4, "the source's two originals and the translation's two, each under a token of its own")

	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, beanSentence, block.SourceText())
	assert.Equal(t, target, block.TargetText("fr"))
}

// A translation redact and unredact went around keeps its runs, status and
// origin when the tool between them left it alone; one the tool changed is
// the tool's draft.
func TestRedactUnredact_KeepsATranslationTheToolLeftAlone(t *testing.T) {
	const fr, de = "Mr Bean est le nouveau King of England", "Mr Bean ist der neue King of England"
	human := model.Origin{Kind: "human", Reference: "xliff"}
	block := newBeanBlock()
	block.SetTargetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: fr}}}, Status: model.Status(model.TargetStatusEstablished), Origin: human})
	block.SetTargetEdition(model.Variant("de"), model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: de}}}, Status: model.Status(model.TargetStatusEstablished), Origin: human})
	block.SetTargetEdition(model.Variant("es"), model.Edition{Runs: []model.Run{{Text: &model.TextRun{Text: "Mr Bean es el nuevo King of England"}}}, Status: model.Status(model.TargetStatusTranslated)})
	sourceBefore := model.CanonicalRunsJSON(block.SourceRuns())
	frBefore, _ := block.TargetEdition("fr")

	whole, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules}, Rules: beanRules(), WholeBlock: true})
	require.NoError(t, err)
	processPart(t, whole, &model.Part{Type: model.PartBlock, Resource: block})
	// The tool between the steps rewrites the German, placeholders kept.
	redacted := block.TargetRuns("de")
	changed := append([]model.Run{{Text: &model.TextRun{Text: "Neu: "}}}, redacted...)
	v := tool.NewVariantView(block)
	v.SetTargetRuns("de", changed)

	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})

	assert.Equal(t, string(sourceBefore), string(model.CanonicalRunsJSON(block.SourceRuns())), "the source goes back as it was read")
	frAfter, ok := block.TargetEdition("fr")
	require.True(t, ok)
	assert.Equal(t, string(model.CanonicalRunsJSON(frBefore.Runs)), string(model.CanonicalRunsJSON(frAfter.Runs)))
	assert.Equal(t, model.Status(model.TargetStatusEstablished), frAfter.Status, "established stays established")
	assert.Equal(t, human, frAfter.Origin, "the origin is the one it was read with")

	esAfter, ok := block.TargetEdition("es")
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusTranslated), esAfter.Status)
	assert.Equal(t, model.Origin{}, esAfter.Origin, "a translation read with no origin takes none from redact")

	deAfter, ok := block.TargetEdition("de")
	require.True(t, ok)
	assert.Equal(t, "Neu: "+de, model.RunsText(deAfter.Runs))
	assert.Equal(t, model.Status(model.TargetStatusDraft), deAfter.Status, "a translation the tool changed is a draft")
	_, ok = block.Anno(redaction.SecretAnnotationKey)
	assert.False(t, ok)
}

// Detection reads the text runs redact carves, and every plural form is
// redacted under tokens of its own; unredact restores inside the forms.
func TestRedactUnredact_PluralForms(t *testing.T) {
	plural := func() *model.Block {
		b := model.NewBlock("p1", "")
		b.SourceLocale = "en"
		b.SetSourceRuns([]model.Run{
			{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {{Text: &model.TextRun{Text: "one file for Mr Bean"}}},
				model.PluralOther: {{Text: &model.TextRun{Text: "many files for Mr Bean"}}},
			}}},
			{Text: &model.TextRun{Text: " sent to Mr Bean, the King of England."}},
		})
		return b
	}
	block := plural()
	before := model.CanonicalRunsJSON(block.SourceRuns())
	whole, err := tools.NewRedactTool(&tools.RedactConfig{Detectors: []string{tools.DetectRules}, Rules: beanRules(), WholeBlock: true})
	require.NoError(t, err)
	processPart(t, whole, &model.Part{Type: model.PartBlock, Resource: block})

	flat := string(model.CanonicalRunsJSON(block.SourceRuns()))
	assert.NotContains(t, flat, "Bean", "no form keeps the name")
	assert.NotContains(t, flat, "England")
	assert.Contains(t, block.SourceRuns()[1].Text.Text, " sent to ", "the text after the plural is carved where the name is")
	ann, ok := model.AnnoAs[*redaction.SecretAnnotation](block, redaction.SecretAnnotationKey)
	require.True(t, ok)
	assert.Len(t, ann.Values, 4, "one name in each form, a name and a role after the plural")

	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredact, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, string(before), string(model.CanonicalRunsJSON(block.SourceRuns())))
}
