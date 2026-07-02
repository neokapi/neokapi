package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unredactFixtures pair a sentence with the detection rules that hit it —
// literal terms and regex patterns of the kind redact ships for (emails,
// API keys, phone-like digits) — including repeated categories, which force
// per-category indexed placeholders.
var unredactFixtures = []struct {
	name  string
	text  string
	rules []redaction.Rule
}{
	{
		name:  "literal terms",
		text:  "Mr Bean is the new King of England",
		rules: []redaction.Rule{{Term: "Mr Bean", Category: "person"}, {Term: "King of England", Category: "role"}},
	},
	{
		name:  "email pattern",
		text:  "Contact alice@example.com or bob@example.com for access",
		rules: []redaction.Rule{{Pattern: `[a-z]+@example\.com`, Category: "email"}},
	},
	{
		name: "api key pattern",
		text: "Use sk-live-4eC39HqLyjWDarjtT1zdp7dc when calling the API",
		rules: []redaction.Rule{
			{Pattern: `sk-live-[A-Za-z0-9]+`, Category: "credential"},
		},
	},
	{
		name: "mixed terms and patterns, repeated categories",
		text: "Alice (alice@example.com) and Bob (bob@example.com) share key sk-live-abc123",
		rules: []redaction.Rule{
			{Term: "Alice", Category: "person"},
			{Term: "Bob", Category: "person"},
			{Pattern: `[a-z]+@example\.com`, Category: "email"},
			{Pattern: `sk-live-[a-z0-9]+`, Category: "credential"},
		},
	},
	{
		name:  "secret at string boundaries",
		text:  "root@example.com started it and root@example.com ended it",
		rules: []redaction.Rule{{Pattern: `[a-z]+@example\.com`, Category: "email"}},
	},
}

// TestUnredact_RoundTripProperty is the redact→unredact round-trip property:
// for every fixture, redaction removes each secret from the visible text and
// unredaction restores the block byte-for-byte — in the source and in a
// target that carried the placeholders through a simulated translation —
// while stripping the in-process secret annotation.
func TestUnredact_RoundTripProperty(t *testing.T) {
	for _, fx := range unredactFixtures {
		t.Run(fx.name, func(t *testing.T) {
			redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
				Detectors: []string{tools.DetectRules},
				Rules:     fx.rules,
			})
			require.NoError(t, err)

			block := model.NewBlock("b1", fx.text)
			block.SourceLocale = "en"
			processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})

			// Redaction must have removed every secret from the visible text.
			redactedFlat := model.FlattenRuns(block.SourceRuns())
			detector, err := redaction.NewRuleDetector(fx.rules)
			require.NoError(t, err)
			matches, err := detector.Detect(t.Context(), fx.text, "en")
			require.NoError(t, err)
			require.NotEmpty(t, matches, "fixture must actually match")
			for _, m := range matches {
				assert.NotContains(t, redactedFlat, m.Original, "secret %q must not survive redaction", m.Original)
			}

			// Simulate a placeholder-preserving translation into two locales.
			block.SetTargetRuns("fr", block.SourceRuns())
			block.SetTargetRuns("de", block.SourceRuns())

			unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
			require.NoError(t, err)
			processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})

			assert.Equal(t, fx.text, block.SourceText(), "source restores byte-for-byte")
			assert.Equal(t, fx.text, block.TargetText("fr"), "fr target restores byte-for-byte")
			assert.Equal(t, fx.text, block.TargetText("de"), "de target restores byte-for-byte")
			_, hasAnn := block.Anno(redaction.SecretAnnotationKey)
			assert.False(t, hasAnn, "secret annotation must be removed after unredact")
		})
	}
}

// TestUnredact_RestoresFlattenedTargetText covers the write-path variant
// where a format flattened the placeholder Ph runs to their visible token
// text: unredact restores by the token string, not only by placeholder id.
func TestUnredact_RestoresFlattenedTargetText(t *testing.T) {
	rules := []redaction.Rule{{Term: "Mr Bean", Category: "person"}}
	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     rules,
	})
	require.NoError(t, err)

	const sentence = "Mr Bean is the new King of England"
	block := model.NewBlock("b1", sentence)
	block.SourceLocale = "en"
	processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})

	// Flatten the redacted source into a plain text target the way a
	// markup-splicing writer would (RenderRunsWithData) — the placeholder
	// survives only as its visible "[REDACTED:Person]" string.
	flat := model.RenderRunsWithData(block.SourceRuns())
	require.Contains(t, flat, "[REDACTED:Person]")
	block.SetTargetText("fr", flat)

	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})

	assert.Equal(t, sentence, block.SourceText())
	assert.Equal(t, sentence, block.TargetText("fr"), "flattened token text must restore too")
}

// TestUnredact_NoAnnotationPassThrough: a block that was never redacted (and
// no vault is configured) passes through unchanged.
func TestUnredact_NoAnnotationPassThrough(t *testing.T) {
	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)

	block := model.NewBlock("b1", "Nothing secret here")
	block.SetTargetText("fr", "Rien de secret ici")
	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})

	assert.Equal(t, "Nothing secret here", block.SourceText())
	assert.Equal(t, "Rien de secret ici", block.TargetText("fr"))
}

// TestUnredact_Idempotent: running unredact twice is safe — the second pass
// finds no annotation and changes nothing.
func TestUnredact_Idempotent(t *testing.T) {
	rules := []redaction.Rule{{Pattern: `[a-z]+@example\.com`, Category: "email"}}
	redactTool, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     rules,
	})
	require.NoError(t, err)

	const sentence = "Mail alice@example.com today"
	block := model.NewBlock("b1", sentence)
	block.SourceLocale = "en"
	processPart(t, redactTool, &model.Part{Type: model.PartBlock, Resource: block})

	unredactTool, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})
	require.Equal(t, sentence, block.SourceText())

	processPart(t, unredactTool, &model.Part{Type: model.PartBlock, Resource: block})
	assert.Equal(t, sentence, block.SourceText(), "second unredact pass is a no-op")
}
