package tools

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runPlaceholder(t *testing.T, src, tgt string, flagExtra bool) []check.Finding {
	t.Helper()
	loc := model.LocaleID("de")
	b := &model.Block{ID: "b", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: src}}}}
	tool.NewVariantView(b).SetTargetText(loc, tgt)
	cfg := NewPlaceholderCheckConfig(loc)
	cfg.FlagExtra = flagExtra
	tl := NewPlaceholderCheckTool(cfg)
	require.NoError(t, tl.Annotate(tool.NewBlockView(b)))
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return nil
	}
	return ann.Findings
}

func TestPlaceholderCheck_Preserved(t *testing.T) {
	assert.Empty(t, runPlaceholder(t,
		"Hello {name}, you have {count} messages",
		"Hallo {name}, Sie haben {count} Nachrichten", true))
	assert.Empty(t, runPlaceholder(t,
		"Loaded %d items in %s", "%d Elemente in %s geladen", true))
	assert.Empty(t, runPlaceholder(t,
		"Click <0>here</0>", "Klicken Sie <0>hier</0>", true))
}

func TestPlaceholderCheck_Dropped(t *testing.T) {
	f := runPlaceholder(t,
		"Hello {name}, you have {count} messages",
		"Hallo, Sie haben {count} Nachrichten", true)
	require.Len(t, f, 1)
	assert.Equal(t, "placeholder", f[0].Category)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
	assert.Equal(t, "{name}", f[0].OriginalText)
}

func TestPlaceholderCheck_DroppedPrintf(t *testing.T) {
	f := runPlaceholder(t, "Loaded %d items in %s", "Elemente geladen", true)
	require.Len(t, f, 2) // %d and %s both dropped
	for _, x := range f {
		assert.Equal(t, check.SeverityCritical, x.Severity)
	}
}

func TestPlaceholderCheck_Extra(t *testing.T) {
	f := runPlaceholder(t,
		"Hello {name}", "Hallo {name} {stray}", true)
	require.Len(t, f, 1)
	assert.Equal(t, check.SeverityMajor, f[0].Severity)
	assert.Equal(t, "{stray}", f[0].OriginalText)

	// FlagExtra off → no finding for the stray.
	assert.Empty(t, runPlaceholder(t, "Hello {name}", "Hallo {name} {stray}", false))
}

func TestPlaceholderCheck_DoubleBraceTokenization(t *testing.T) {
	// {{x}} must tokenize as one token, not as {x}.
	c := placeholderCounts("{{x}} and {y}")
	assert.Equal(t, 1, c["{{x}}"])
	assert.Equal(t, 1, c["{y}"])
	assert.Equal(t, 0, c["{x}"])
}

// runPlaceholderRuns drives the check over Run sequences rather than flat
// strings, so inline-code placeholders (Ph / PcOpen / PcClose) are in play.
func runPlaceholderRuns(t *testing.T, src, tgt []model.Run, flagExtra bool) []check.Finding {
	t.Helper()
	loc := model.LocaleID("de")
	b := &model.Block{ID: "b", Translatable: true, Source: src}
	tool.NewVariantView(b).SetTargetRuns(loc, tgt)
	cfg := NewPlaceholderCheckConfig(loc)
	cfg.FlagExtra = flagExtra
	tl := NewPlaceholderCheckTool(cfg)
	require.NoError(t, tl.Annotate(tool.NewBlockView(b)))
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)
	if !ok {
		return nil
	}
	return ann.Findings
}

// TestPlaceholderCheck_InlineCodes: a placeholder lifted out of the text into a
// Ph run contributes no characters to the text projection, so a text-only check
// cannot see it go missing. The check compares inline codes too — this is the
// detection half of the recycled-placeholder-loss guard.
func TestPlaceholderCheck_InlineCodes(t *testing.T) {
	phRun := func(equiv string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: "1", Equiv: equiv, Data: "{" + equiv + "}"}}
	}
	text := func(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

	// Preserved: no findings.
	assert.Empty(t, runPlaceholderRuns(t,
		[]model.Run{phRun("count"), text(" documented formats")},
		[]model.Run{phRun("count"), text(" dokumentierte Formate")}, true))

	// Dropped: critical.
	f := runPlaceholderRuns(t,
		[]model.Run{phRun("documentedCount"), text(" documented formats")},
		[]model.Run{text(" dokumentierte Formate")}, true)
	require.Len(t, f, 1)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
	assert.Equal(t, "ph:documentedCount", f[0].OriginalText)
	assert.Contains(t, f[0].Message, "missing from the de target")

	// Invented: major, and only when FlagExtra is on.
	f = runPlaceholderRuns(t,
		[]model.Run{text("Install")},
		[]model.Run{phRun("count"), text(" Installieren")}, true)
	require.Len(t, f, 1)
	assert.Equal(t, check.SeverityMajor, f[0].Severity)
	assert.Equal(t, "ph:count", f[0].OriginalText)
	assert.Empty(t, runPlaceholderRuns(t,
		[]model.Run{text("Install")},
		[]model.Run{phRun("count"), text(" Installieren")}, false))
}
