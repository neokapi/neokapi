package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

func itemPlural(one, other string) model.Run {
	return model.Run{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {{Text: &model.TextRun{Text: one}}},
		model.PluralOther: {{Text: &model.TextRun{Text: other}}},
	}}}
}

func searchReplace(pairs []tools.ReplacePair, all bool) *tools.SearchReplaceConfig {
	return &tools.SearchReplaceConfig{Pairs: pairs, Source: true, ReplaceAll: all}
}

func blockFindings(b *model.Block) []check.Finding {
	if a, ok := b.Annotations[check.AnnotationKey].(*check.FindingsAnnotation); ok {
		return a.Findings
	}
	return nil
}

// A match that runs across a plural or select would delete it. The tool
// leaves that text as it was, reports it as a finding that does not fail, and
// makes every other replacement; the run goes on.
func TestSearchReplaceSkipsAMatchAcrossAPlural(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "p", Translatable: true}
	b.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: "You have "}}, itemPlural("# item", "# items"), {Text: &model.TextRun{Text: " left"}}})
	tl := tools.NewSearchReplaceTool(searchReplace([]tools.ReplacePair{
		{Search: `have\s+left`, Replace: "X", IsRegex: true},
		{Search: "You", Replace: "We"},
	}, true))

	got := runTransform(t, tl, b)

	assert.Equal(t, "We have # items left", model.RunsText(got.SourceRuns()))
	require.NotNil(t, got.SourceRuns()[1].Plural, "the plural is kept")
	findings := blockFindings(got)
	require.Len(t, findings, 1)
	assert.Equal(t, "search-replace", findings[0].Category)
	assert.False(t, findings[0].Fails)
	assert.Contains(t, findings[0].Message, "plural or select")
	assert.Equal(t, "have  left", findings[0].OriginalText)
}

// A standalone code inside a match is kept, before the replacement. A paired
// code stays while any of its text is left, and goes with a match that
// replaces all of its text.
func TestSearchReplaceKeepsACodeInsideAMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		runs   []model.Run
		search string
		want   string
	}{
		{"a placeholder", varRuns("You have ", " left"), "have  left", `You <x id="2/"/>X`},
		{"a paired code partly inside", boldRuns("Click ", "Save now", " to go"), "now to", `Click <x id="1"/>Save <x id="/1"/>X go`},
		{"a paired code wholly inside", boldRuns("Click ", "Save", " to go"), "Save to", `Click X go`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &model.Block{ID: "p", Translatable: true}
			b.SetSourceRuns(tc.runs)
			tl := tools.NewSearchReplaceTool(searchReplace([]tools.ReplacePair{{Search: tc.search, Replace: "X"}}, true))
			assert.Equal(t, tc.want, model.RunsPlaceholderText(runTransform(t, tl, b).SourceRuns()))
		})
	}
}

// Replacing the first match only replaces one match in the edition, the first
// in reading order: the text before a plural, then its forms from zero to
// other, then the text after it.
func TestSearchReplaceFirstMatchOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		runs []model.Run
		want string
	}{
		{"before the plural", []model.Run{{Text: &model.TextRun{Text: "file: "}}, itemPlural("one file", "many files"), {Text: &model.TextRun{Text: " file"}}},
			"doc: {one file|many files} file"},
		{"in the first form", []model.Run{{Text: &model.TextRun{Text: "Open "}}, itemPlural("one file", "many files"), {Text: &model.TextRun{Text: " file"}}},
			"Open {one doc|many files} file"},
		{"after the plural", []model.Run{{Text: &model.TextRun{Text: "Open "}}, itemPlural("one", "many"), {Text: &model.TextRun{Text: " file, file"}}},
			"Open {one|many} doc, file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &model.Block{ID: "p", Translatable: true}
			b.SetSourceRuns(tc.runs)
			tl := tools.NewSearchReplaceTool(searchReplace([]tools.ReplacePair{{Search: "file", Replace: "doc"}}, false))
			assert.Equal(t, tc.want, pluralText(runTransform(t, tl, b).SourceRuns()))
		})
	}
}

// pluralText renders runs with a plural as {one|other}.
func pluralText(runs []model.Run) string {
	var out string
	for _, r := range runs {
		switch {
		case r.Text != nil:
			out += r.Text.Text
		case r.Plural != nil:
			out += "{" + model.RunsText(r.Plural.Forms[model.PluralOne]) + "|" + model.RunsText(r.Plural.Forms[model.PluralOther]) + "}"
		}
	}
	return out
}

// case-transform changes a character into one character, so every overlay
// span keeps the characters it covered.
func TestCaseTransformKeepsOverlaySpans(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "p", Translatable: true}
	b.SetSourceRuns(boldRuns("Grind the ", "coffee", " now"))
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "coffee", Range: model.SpanAnchor(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 6})})
	b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "rind", Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 1}, model.RunPos{Run: 0, Offset: 4})})
	tl := tools.NewCaseTransformTool(&tools.CaseTransformConfig{Mode: tools.CaseUpper, ApplySource: true})

	got := runTransform(t, tl, b)

	assert.Equal(t, `GRIND THE <x id="1"/>COFFEE<x id="/1"/> NOW`, model.RunsPlaceholderText(got.SourceRuns()))
	for _, tc := range []struct {
		typ      model.OverlayType
		id, want string
	}{{model.OverlayTerm, "coffee", "COFFEE"}, {model.OverlayEntity, "rind", "RIN"}} {
		sp := got.OverlaySpan(tc.typ, tc.id)
		require.NotNil(t, sp, tc.id)
		assert.Equal(t, tc.want, model.RunsText(sp.Range.ExtractRuns(got.SourceRuns())))
	}
}
