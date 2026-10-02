package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// The text transforms (search-replace, case-transform) read an edition's text
// and rewrite it. They wrote the result back as one plain text run, on the
// source and on every target, so a replacement anywhere in a block deleted its
// inline codes and flattened a plural. They now edit the text in place: the
// codes, the structure and the run flags the edit does not touch are kept.

func boldRuns(before, bold, after string) []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: before}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		{Text: &model.TextRun{Text: bold}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
		{Text: &model.TextRun{Text: after}},
	}
}

func varRuns(before, after string) []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: before}},
		{Ph: &model.PlaceholderRun{ID: "2", Type: "x-variable", Data: "{count}"}},
		{Text: &model.TextRun{Text: after}},
	}
}

func runTransform(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, b *model.Block) *model.Block {
	t.Helper()
	return processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b}).Resource.(*model.Block)
}

func TestSearchReplaceKeepsInlineCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		pairs  []tools.ReplacePair
		regex  bool
		target []model.Run
		want   string
	}{
		{
			name:   "a word after a paired code",
			pairs:  []tools.ReplacePair{{Search: "continuer", Replace: "poursuivre"}},
			target: boldRuns("Cliquez sur ", "Enregistrer", " pour continuer"),
			want:   `Cliquez sur <x id="1"/>Enregistrer<x id="/1"/> pour poursuivre`,
		},
		{
			name:   "the whole content of a paired code",
			pairs:  []tools.ReplacePair{{Search: "Enregistrer", Replace: "Sauvegarder"}},
			target: boldRuns("Cliquez sur ", "Enregistrer", " pour continuer"),
			want:   `Cliquez sur <x id="1"/>Sauvegarder<x id="/1"/> pour continuer`,
		},
		{
			name:   "every match around a placeholder",
			pairs:  []tools.ReplacePair{{Search: "article", Replace: "élément"}},
			target: varRuns("article ", " article"),
			want:   `élément <x id="2/"/> élément`,
		},
		{
			name:   "a regular expression with a group",
			pairs:  []tools.ReplacePair{{Search: `(\w+)er\b`, Replace: "${1}ez"}},
			regex:  true,
			target: boldRuns("Cliquer sur ", "Enregistrer", " ici"),
			want:   `Cliquez sur <x id="1"/>Enregistrez<x id="/1"/> ici`,
		},
		{
			name: "two pairs in order",
			pairs: []tools.ReplacePair{
				{Search: "Cliquez", Replace: "Appuyez"},
				{Search: "continuer", Replace: "poursuivre"},
			},
			target: boldRuns("Cliquez sur ", "Enregistrer", " pour continuer"),
			want:   `Appuyez sur <x id="1"/>Enregistrer<x id="/1"/> pour poursuivre`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := model.NewBlock("p", "Click Save to continue")
			b.SetTargetRuns(model.LocaleFrench, tc.target)
			tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
				Pairs: tc.pairs, RegEx: tc.regex, TargetLocale: model.LocaleFrench,
				Target: true, ReplaceAll: true,
			})
			got := runTransform(t, tl, b)
			assert.Equal(t, tc.want, model.RunsPlaceholderText(got.TargetRuns(model.LocaleFrench)))
			assert.Equal(t, "Click Save to continue", got.SourceText(), "the source is out of scope")
		})
	}
}

func TestSearchReplaceKeepsSourceCodesAndOverlays(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "p", Translatable: true}
	b.SetSourceRuns(boldRuns("Click ", "Save", " to continue"))
	b.AddOverlaySpan(model.OverlayTerm, model.Span{
		ID:    "term:save",
		Range: model.RangeAnchorForBytes(b.SourceRuns(), len("Click "), len("Click Save")),
	})
	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs:  []tools.ReplacePair{{Search: "continue", Replace: "go on"}},
		Source: true, ReplaceAll: true,
	})
	got := runTransform(t, tl, b)
	assert.Equal(t, `Click <x id="1"/>Save<x id="/1"/> to go on`, model.RunsPlaceholderText(got.SourceRuns()))
	sp := got.OverlaySpan(model.OverlayTerm, "term:save")
	require.NotNil(t, sp, "an overlay the edit does not touch follows it")
	assert.Equal(t, "Save", model.RunsText(sp.Range.ExtractRuns(got.SourceRuns())))
}

func TestSearchReplaceKeepsAPluralTarget(t *testing.T) {
	t.Parallel()
	plural := []model.Run{{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   varRuns("", " article"),
		model.PluralOther: varRuns("", " articles"),
	}}}}
	b := model.NewBlock("cart", "{count} items")
	b.SetTargetRuns(model.LocaleFrench, plural)
	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{{Search: "article", Replace: "élément"}}, TargetLocale: model.LocaleFrench,
		Target: true, ReplaceAll: true,
	})
	got := runTransform(t, tl, b).TargetRuns(model.LocaleFrench)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Plural, "the plural is kept")
	assert.Equal(t, `<x id="2/"/> élément`, model.RunsPlaceholderText(got[0].Plural.Forms[model.PluralOne]))
	assert.Equal(t, `<x id="2/"/> éléments`, model.RunsPlaceholderText(got[0].Plural.Forms[model.PluralOther]))
}

func TestCaseTransformKeepsInlineCodes(t *testing.T) {
	t.Parallel()
	b := &model.Block{ID: "p", Translatable: true}
	b.SetSourceRuns(boldRuns("Click ", "Save", " to continue"))
	b.SetTargetRuns(model.LocaleFrench, varRuns("Il reste ", " articles"))
	tl := tools.NewCaseTransformTool(&tools.CaseTransformConfig{
		Mode: tools.CaseUpper, ApplySource: true, ApplyTarget: true, TargetLocale: model.LocaleFrench,
	})
	got := runTransform(t, tl, b)
	assert.Equal(t, `CLICK <x id="1"/>SAVE<x id="/1"/> TO CONTINUE`, model.RunsPlaceholderText(got.SourceRuns()))
	assert.Equal(t, `IL RESTE <x id="2/"/> ARTICLES`, model.RunsPlaceholderText(got.TargetRuns(model.LocaleFrench)))
}

func TestSearchReplaceKeepsNoTranslateText(t *testing.T) {
	t.Parallel()
	b := model.NewBlock("p", "Use the Acme console")
	b.SetTargetRuns(model.LocaleFrench, []model.Run{
		{Text: &model.TextRun{Text: "Utilisez la console "}},
		{Text: &model.TextRun{Text: "Acme", NoTranslate: true}},
	})
	tl := tools.NewSearchReplaceTool(&tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{{Search: "Utilisez", Replace: "Ouvrez"}}, TargetLocale: model.LocaleFrench,
		Target: true, ReplaceAll: true,
	})
	got := runTransform(t, tl, b).TargetRuns(model.LocaleFrench)
	require.Len(t, got, 2)
	assert.Equal(t, "Ouvrez la console ", got[0].Text.Text)
	assert.True(t, got[1].Text.NoTranslate, "the flag on text the edit leaves alone survives")
}
