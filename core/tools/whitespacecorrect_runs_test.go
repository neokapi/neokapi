package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// whitespace-correct is the one member of the shape-blind flattening class that
// *writes*. It read the source's edge whitespace from SourceText() — so
// `[ph{p.price}][text " each"]` yielded a leading space the source never had, the
// founder's false positive as a mutation — and it round-tripped the target
// through TargetText() / SetTargetText(), which replaces the whole run sequence
// with one text run and so destroyed every inline code in the target.
//
// These tests hold both ends: the corrections judge run-aware boundaries, and the
// target's runs survive.

func wsTx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func wsPh(id, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "jsx:var", Data: "{" + equiv + "}", Equiv: equiv}}
}

// wsCorrect runs whitespace-correct over a source/target run pair and returns the
// resulting target runs.
func wsCorrect(t *testing.T, cfg *tools.WhitespaceCorrectConfig, sourceRuns, targetRuns []model.Run) []model.Run {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: sourceRuns}
	b.SetTargetRuns(model.LocaleFrench, targetRuns)

	tl := tools.NewWhitespaceCorrectTool(cfg)
	out := processPart(t, tl, &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)
	return blk.TargetRuns(model.LocaleFrench)
}

// runShapes renders a run sequence as one string per run, so a test failure names
// exactly which run changed. Inline codes render as `{ph:equiv}`.
func runShapes(runs []model.Run) []string {
	out := make([]string, 0, len(runs))
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindText:
			out = append(out, "text:"+r.Text.Text)
		case model.RunKindPh:
			out = append(out, "ph:"+r.Ph.Equiv)
		case model.RunKindPcOpen:
			out = append(out, "pcOpen:"+r.PcOpen.ID)
		case model.RunKindPcClose:
			out = append(out, "pcClose:"+r.PcClose.ID)
		case model.RunKindPlural:
			out = append(out, "plural:"+model.RunsText([]model.Run{r}))
		default:
			out = append(out, string(r.Kind()))
		}
	}
	return out
}

// TestWhitespaceCorrect_PreservesTargetRuns is the write-path guard: the tool is
// asked to tidy whitespace and must not consume the target's inline structure
// doing it. Under SetTargetText() every one of these targets came out as a single
// text run with its codes gone — the recycle-path loss of #1456, as a write.
func TestWhitespaceCorrect_PreservesTargetRuns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		cfg    *tools.WhitespaceCorrectConfig
		source []model.Run
		target []model.Run
		want   []string
	}{
		{
			name: "a placeholder survives space normalization",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
			},
			source: []model.Run{wsTx("Hello "), wsPh("1", "name")},
			target: []model.Run{wsTx("Bonjour   "), wsPh("1", "name")},
			want:   []string{"text:Bonjour ", "ph:name"},
		},
		{
			name: "a paired code survives zero-width removal",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, RemoveZeroWidthChars: true,
			},
			source: []model.Run{
				{PcOpen: &model.PcOpenRun{ID: "1", Type: "html:b"}},
				wsTx("Hello"),
				{PcClose: &model.PcCloseRun{ID: "1"}},
			},
			target: []model.Run{
				{PcOpen: &model.PcOpenRun{ID: "1", Type: "html:b"}},
				wsTx("Bon\u200bjour"),
				{PcClose: &model.PcCloseRun{ID: "1"}},
			},
			want: []string{"pcOpen:1", "text:Bonjour", "pcClose:1"},
		},
		{
			name: "a placeholder-only target is left alone",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
				TrimLeading: true, TrimTrailing: true, MatchSourceWhitespace: true,
			},
			source: []model.Run{wsPh("1", "price")},
			target: []model.Run{wsPh("1", "price")},
			want:   []string{"ph:price"},
		},
		{
			name: "punctuation correction does not consume the codes it walks past",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, CorrectFullStop: true,
			},
			source: []model.Run{wsTx("End. "), wsPh("1", "x"), wsTx(" More")},
			target: []model.Run{wsTx("Fin. "), wsPh("1", "x"), wsTx(" Plus")},
			// The space after "Fin." is removed; the space after the placeholder
			// is not (the code is a boundary, so it does not follow the full stop).
			want: []string{"text:Fin.", "ph:x", "text: Plus"},
		},
		{
			name: "a plural construct survives",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
			},
			source: []model.Run{wsTx("You have "), wsPluralRun("one item", "many items")},
			target: []model.Run{wsTx("Vous  avez "), wsPluralRun("un   objet", "des   objets")},
			want:   []string{"text:Vous avez ", "plural:des objets"},
		},
		{
			// A wrapped list item's continuation indent is markup around the
			// translation: collapsing it re-lays-out the target's markdown. It is
			// the same whitespace `hygiene.double-spaces` declines to report
			// (#1854), and the two must not disagree.
			name: "a continuation indent survives space normalization",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
			},
			source: []model.Run{wsTx("one two\n  three four")},
			target: []model.Run{wsTx("un deux\n  trois quatre")},
			want:   []string{"text:un deux\n  trois quatre"},
		},
		{
			// The indent is kept, the prose defect beside it is still collapsed.
			name: "a double space past the indent still collapses",
			cfg: &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
			},
			source: []model.Run{wsTx("one two\n  three four")},
			target: []model.Run{wsTx("un deux\n  trois  quatre")},
			want:   []string{"text:un deux\n  trois quatre"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := wsCorrect(t, tt.cfg, tt.source, tt.target)
			assert.Equal(t, tt.want, runShapes(got))
		})
	}
}

func wsPluralRun(one, other string) model.Run {
	return model.Run{Plural: &model.PluralRun{
		Pivot: "count",
		Forms: map[model.PluralForm][]model.Run{
			model.PluralOne:   {wsTx(one)},
			model.PluralOther: {wsTx(other)},
		},
	}}
}

// TestWhitespaceCorrect_MatchSourceWhitespaceIsRunAware is the founder's reported
// case as a mutation. `[ph{p.price}][text " each"]` has no leading whitespace: the
// placeholder is the content's leading edge and the space beside it separates the
// two. Read through SourceText() the source read as " each" and this tool copied
// that phantom space onto the target.
func TestWhitespaceCorrect_MatchSourceWhitespaceIsRunAware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source []model.Run
		target []model.Run
		want   []string
	}{
		{
			name:   "a leading placeholder is not leading whitespace to copy",
			source: []model.Run{wsPh("1", "p.price"), wsTx(" each")},
			target: []model.Run{wsPh("1", "p.price"), wsTx(" chacun")},
			want:   []string{"ph:p.price", "text: chacun"},
		},
		{
			name:   "a trailing placeholder is not trailing whitespace to copy",
			source: []model.Run{wsTx("Total: "), wsPh("1", "total")},
			target: []model.Run{wsTx("Total : "), wsPh("1", "total")},
			want:   []string{"text:Total : ", "ph:total"},
		},
		{
			// The target's own edge whitespace is still stripped to match a source
			// that has none — the correction still corrects, it just no longer
			// invents the thing it is correcting to.
			name:   "the target's stray edge whitespace is stripped to match a code-edged source",
			source: []model.Run{wsPh("1", "p.price"), wsTx(" each")},
			target: []model.Run{wsTx("  "), wsPh("1", "p.price"), wsTx(" chacun  ")},
			want:   []string{"ph:p.price", "text: chacun"},
		},
		{
			// Real edge whitespace on the source is still copied, and lands beside
			// the target's leading code as its own text run rather than swallowing it.
			name:   "real source edge whitespace is copied around a code-edged target",
			source: []model.Run{wsTx("  Hello  ")},
			target: []model.Run{wsPh("1", "x"), wsTx("Bonjour")},
			want:   []string{"text:  ", "ph:x", "text:Bonjour  "},
		},
		{
			name:   "real source edge whitespace is still copied onto a plain target",
			source: []model.Run{wsTx("  Hello  ")},
			target: []model.Run{wsTx("Bonjour")},
			want:   []string{"text:  Bonjour  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, MatchSourceWhitespace: true,
			}
			got := wsCorrect(t, cfg, tt.source, tt.target)
			assert.Equal(t, tt.want, runShapes(got))
		})
	}
}

// TestWhitespaceCorrect_NormalizeSpacesBoundaries pins the same boundary rule
// `hygiene.double-spaces` reports on (#1465): an inline code between two spaces
// separates them, so neither is redundant and nothing collapses. Two adjacent
// text runs have nothing between them, so the join is real text and does collapse.
func TestWhitespaceCorrect_NormalizeSpacesBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target []model.Run
		want   []string
	}{
		{
			name:   "space either side of a code is not a double space",
			target: []model.Run{wsTx("Bonjour "), wsPh("1", "name"), wsTx(" monde")},
			want:   []string{"text:Bonjour ", "ph:name", "text: monde"},
		},
		{
			name:   "two spaces spanning the join of adjacent text runs collapse",
			target: []model.Run{wsTx("Bonjour "), wsTx(" monde")},
			want:   []string{"text:Bonjour ", "text:monde"},
		},
		{
			name:   "a real double space inside one run still collapses",
			target: []model.Run{wsPh("1", "x"), wsTx(" le  monde")},
			want:   []string{"ph:x", "text: le monde"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench, NormalizeSpaces: true,
			}
			got := wsCorrect(t, cfg, []model.Run{wsTx("Hello world")}, tt.target)
			assert.Equal(t, tt.want, runShapes(got))
		})
	}
}

// TestWhitespaceCorrect_TrimEdgesIsRunAware: a leading or trailing inline code IS
// the content's edge, so there is no edge whitespace there to trim. A
// whitespace-only text run at the edge is edge whitespace and goes.
func TestWhitespaceCorrect_TrimEdgesIsRunAware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target []model.Run
		want   []string
	}{
		{
			name:   "a code-edged target has no edge whitespace to trim",
			target: []model.Run{wsPh("1", "x"), wsTx(" middle "), wsPh("2", "y")},
			want:   []string{"ph:x", "text: middle ", "ph:y"},
		},
		{
			name:   "whitespace-only edge runs are dropped and the code survives",
			target: []model.Run{wsTx("  "), wsPh("1", "x"), wsTx(" middle"), wsTx("\t")},
			want:   []string{"ph:x", "text: middle"},
		},
		{
			name:   "plain edge whitespace is still trimmed",
			target: []model.Run{wsTx("  Bonjour  ")},
			want:   []string{"text:Bonjour"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &tools.WhitespaceCorrectConfig{
				TargetLocale: model.LocaleFrench,
				TrimLeading:  true, TrimTrailing: true,
			}
			got := wsCorrect(t, cfg, []model.Run{wsTx("Hello")}, tt.target)
			assert.Equal(t, tt.want, runShapes(got))
		})
	}
}
