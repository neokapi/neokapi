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
	c := countMatches(placeholderToken, "{{x}} and {y}")
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

// An ICU plural or select carries its sub-messages inside braces, and those
// sub-messages are exactly what a translator rewrites. A check that reads every
// brace span as an opaque placeholder reports a correct translation as having
// dropped every placeholder in the message — the loudest possible finding for
// the one case where nothing is wrong.
func TestPlaceholderCheck_ICUCorrectlyTranslated(t *testing.T) {
	tests := []struct {
		name   string
		source string
		target string
	}{
		{
			name:   "plural",
			source: "{count, plural, one {# berth} other {# berths}} at this terminal.",
			target: "{count, plural, one {# kaiplass} other {# kaiplasser}} på denne terminalen.",
		},
		{
			name:   "select",
			source: "{gender, select, male {He is alongside} female {She is alongside} other {They are alongside}}",
			target: "{gender, select, male {Han ligger til kai} female {Hun ligger til kai} other {De ligger til kai}}",
		},
		{
			name:   "selectordinal",
			source: "The {n, selectordinal, one {#st} two {#nd} few {#rd} other {#th}} berth",
			target: "Den {n, selectordinal, other {#.}} kaiplassen",
		},
		{
			name:   "a target language with more categories than the source",
			source: "{count, plural, one {# berth} other {# berths}}",
			target: "{count, plural, one {# miejsce} few {# miejsca} many {# miejsc} other {# miejsca}}",
		},
		{
			name:   "a target language with fewer categories than the source",
			source: "{count, plural, one {# berth} other {# berths}}",
			target: "{count, plural, other {#個のバース}}",
		},
		{
			name:   "plural nested in select",
			source: "{g, select, male {{n, plural, one {# berth} other {# berths}}} other {none}}",
			target: "{g, select, male {{n, plural, one {# kaiplass} other {# kaiplasser}}} other {ingen}}",
		},
		{
			name:   "an argument inside a sub-message",
			source: "{count, plural, one {{vessel} has # berth} other {{vessel} has # berths}}",
			target: "{count, plural, one {{vessel} har # kaiplass} other {{vessel} har # kaiplasser}}",
		},
		{
			name:   "a sentence frame around the picker",
			source: "Berth {berth} holds {count, plural, one {# vessel} other {# vessels}} today.",
			target: "Kaiplass {berth} rommer {count, plural, one {# fartøy} other {# fartøyer}} i dag.",
		},
		{
			name:   "an offset carried into the target",
			source: "{count, plural, offset:1 =0 {Nobody} one {# other} other {# others}}",
			target: "{count, plural, offset:1 =0 {Ingen} one {# annen} other {# andre}}",
		},
		{
			name:   "escaped braces in a sub-message",
			source: "{count, plural, one {# '{'literal'}'} other {# '{'literals'}'}}",
			target: "{count, plural, one {# '{'bokstavelig'}'} other {# '{'bokstavelige'}'}}",
		},
		{
			name:   "a doubled apostrophe in a sub-message",
			source: "{count, plural, one {It''s # berth} other {It''s # berths}}",
			target: "{count, plural, one {Det''s # kaiplass} other {Det''s # kaiplasser}}",
		},
		{
			name:   "a prose apostrophe beside a picker",
			source: "Don't move {count, plural, one {# vessel} other {# vessels}}.",
			target: "Ikke flytt {count, plural, one {# fartøy} other {# fartøyer}}.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, runPlaceholder(t, tt.source, tt.target, true))
		})
	}
}

// Parsing ICU must not blunt the check. Everything the program reads out of a
// message — the argument name, the picker keyword, the offset, the # a plural
// formats, and every simple argument — is still required, and a plain message
// that lost a placeholder is still critical.
func TestPlaceholderCheck_ICUBreakages(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		target     string
		wantText   string
		wantSever  check.Severity
		wantCount  int
		wantSubstr string
	}{
		{
			name:      "a genuinely dropped simple placeholder is still critical",
			source:    "The {berth} is free.",
			target:    "Kaiplassen er ledig.",
			wantText:  "{berth}",
			wantSever: check.SeverityCritical,
			wantCount: 1,
		},
		{
			name:      "a placeholder dropped from the frame around a picker",
			source:    "Berth {berth} holds {count, plural, one {# vessel} other {# vessels}}.",
			target:    "Kaiplassen rommer {count, plural, one {# fartøy} other {# fartøyer}}.",
			wantText:  "{berth}",
			wantSever: check.SeverityCritical,
			wantCount: 1,
		},
		{
			name:      "a picker flattened away in the target",
			source:    "{count, plural, one {# berth} other {# berths}} here.",
			target:    "{count} kaiplasser her.",
			wantText:  "{count, plural}",
			wantSever: check.SeverityCritical,
			wantCount: 3, // the picker is gone, the # with it, and a bare {count} appeared
		},
		{
			name:      "the argument renamed in the target",
			source:    "{count, plural, one {# berth} other {# berths}}",
			target:    "{antall, plural, one {# kaiplass} other {# kaiplasser}}",
			wantText:  "{count, plural}",
			wantSever: check.SeverityCritical,
			wantCount: 2, // {count, plural} missing, {antall, plural} extra
		},
		{
			name:      "the # dropped from every sub-message",
			source:    "{count, plural, one {# berth} other {# berths}}",
			target:    "{count, plural, one {en kaiplass} other {flere kaiplasser}}",
			wantText:  "#",
			wantSever: check.SeverityCritical,
			wantCount: 1,
		},
		{
			name:      "an argument dropped from every sub-message",
			source:    "{count, plural, one {{vessel} has # berth} other {{vessel} has # berths}}",
			target:    "{count, plural, one {# kaiplass} other {# kaiplasser}}",
			wantText:  "{vessel}",
			wantSever: check.SeverityCritical,
			wantCount: 1,
		},
		{
			name:      "the offset dropped in the target",
			source:    "{count, plural, offset:1 one {# other} other {# others}}",
			target:    "{count, plural, one {# annen} other {# andre}}",
			wantText:  "{count, plural, offset:1}",
			wantSever: check.SeverityCritical,
			wantCount: 2, // the offset picker missing, the offset-free one extra
		},
		{
			name:      "an argument invented in a sub-message",
			source:    "{count, plural, one {# berth} other {# berths}}",
			target:    "{count, plural, one {# kaiplass for {vessel}} other {# kaiplasser}}",
			wantText:  "{vessel}",
			wantSever: check.SeverityMajor,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runPlaceholder(t, tt.source, tt.target, true)
			require.Len(t, f, tt.wantCount)
			var found *check.Finding
			for i := range f {
				if f[i].OriginalText == tt.wantText {
					found = &f[i]
				}
			}
			require.NotNilf(t, found, "no finding for %q in %+v", tt.wantText, f)
			assert.Equal(t, "placeholder", found.Category)
			assert.Equal(t, tt.wantSever, found.Severity)
		})
	}
}

// A message that is not valid ICU keeps the literal comparison: {{name}} and
// ${name} are other interpolation systems that ICU cannot parse, and they must
// still be checked.
func TestPlaceholderCheck_NonICUStylesUnaffected(t *testing.T) {
	assert.Empty(t, runPlaceholder(t,
		"Welcome back, {{name}} — ${count} items",
		"Willkommen zurück, {{name}} — ${count} Artikel", true))

	f := runPlaceholder(t, "Welcome back, {{name}}", "Willkommen zurück", true)
	require.Len(t, f, 1)
	assert.Equal(t, "{{name}}", f[0].OriginalText)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
}

// A printf conversion inside a sub-message is not ICU syntax, and dropping it
// breaks the program just the same.
func TestPlaceholderCheck_ICUKeepsNonBracePlaceholders(t *testing.T) {
	assert.Empty(t, runPlaceholder(t,
		"{count, plural, one {%s berth} other {%s berths}}",
		"{count, plural, one {%s kaiplass} other {%s kaiplasser}}", true))

	f := runPlaceholder(t,
		"{count, plural, one {%s berth} other {%s berths}}",
		"{count, plural, one {en kaiplass} other {flere kaiplasser}}", true)
	require.Len(t, f, 1)
	assert.Equal(t, "%s", f[0].OriginalText)
	assert.Equal(t, check.SeverityCritical, f[0].Severity)
}
