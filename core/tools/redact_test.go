package tools_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
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
	ann, ok := rb.Annotations[redaction.SecretAnnotationKey].(*redaction.SecretAnnotation)
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
	_, ok := block.Annotations[redaction.SecretAnnotationKey]
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
	_, hasAnn := rb.Annotations[redaction.SecretAnnotationKey]
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
	block.Annotations["entity:0"] = &model.EntityAnnotation{
		Text:     "Alice",
		Type:     model.EntityPerson,
		Position: model.TextRange{Start: 0, End: 5},
	}
	block.Annotations["entity:1"] = &model.EntityAnnotation{
		Text:     "Bob",
		Type:     model.EntityPerson,
		Position: model.TextRange{Start: 10, End: 13},
	}

	result := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: block})
	rb := result.Resource.(*model.Block)
	src := rb.SourceText()
	assert.NotContains(t, src, "Alice")
	assert.NotContains(t, src, "Bob")
	ann := rb.Annotations[redaction.SecretAnnotationKey].(*redaction.SecretAnnotation)
	assert.Len(t, ann.Values, 2)
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
	_, ok := rb.Annotations[redaction.SecretAnnotationKey]
	assert.False(t, ok)
}

func TestRedactConfig_Validate(t *testing.T) {
	cfg := &tools.RedactConfig{Detectors: []string{"bogus"}}
	require.Error(t, cfg.Validate())

	ok := &tools.RedactConfig{Detectors: []string{tools.DetectRules, tools.DetectEntities}}
	assert.NoError(t, ok.Validate())
}
