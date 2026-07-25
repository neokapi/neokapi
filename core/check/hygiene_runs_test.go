package check_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// Content hygiene is a judgement about the *shape* of a block: where it begins
// and ends, what sits next to what, whether anything is there at all.
// `SourceText()` drops inline-code runs, which silently changes that shape — a
// block whose first run is a placeholder flattens to text starting with the
// separator space, and reads as "leading whitespace".
//
// These tests hold every shape rule to run-aware boundaries, where a leading or
// trailing placeholder counts as content, while keeping the genuine positives: a
// fix that silences the rule is not a fix.

func hygTx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

// hygJSXVar is the exact run the reported false positive came from: a JSX
// expression container extracted as a placeholder.
func hygJSXVar(id, expr, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "jsx:var", Data: expr, Equiv: equiv}}
}

func hygPcOpen(id string) model.Run {
	return model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: "fmt:bold", Data: "<b>"}}
}

func hygPcClose(id string) model.Run {
	return model.Run{PcClose: &model.PcCloseRun{ID: id, Type: "fmt:bold", Data: "</b>"}}
}

// hygieneCategories runs content-lint over a block built from runs and returns
// the categories it reported.
func hygieneCategories(t *testing.T, runs []model.Run) []string {
	t.Helper()
	b := &model.Block{ID: "u1", Translatable: true, Source: runs, Properties: map[string]string{}}
	out := processPart(t, check.NewContentLintTool(), &model.Part{Type: model.PartBlock, Resource: b})
	blk, ok := out.Resource.(*model.Block)
	require.True(t, ok)

	cats := make([]string, 0, 4)
	for _, f := range blockFindings(blk) {
		cats = append(cats, f.Category)
	}
	return cats
}

func TestContentLint_RunAwareBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		runs    []model.Run
		want    []string
		wantNot []string
	}{
		{
			// The reported bug, verbatim: `{p.price} each` in a JSX catalog.
			name:    "leading placeholder then text — the space is a separator",
			runs:    []model.Run{hygJSXVar("1", "{p.price}", "p.price"), hygTx(" each")},
			wantNot: []string{"leading-whitespace", "trailing-whitespace", "empty"},
		},
		{
			name:    "text then trailing placeholder",
			runs:    []model.Run{hygTx("Total: "), hygJSXVar("1", "{p.total}", "p.total")},
			wantNot: []string{"trailing-whitespace", "leading-whitespace", "empty"},
		},
		{
			name:    "placeholder on both edges",
			runs:    []model.Run{hygJSXVar("1", "{a}", "a"), hygTx(" of "), hygJSXVar("2", "{b}", "b")},
			wantNot: []string{"leading-whitespace", "trailing-whitespace", "empty"},
		},
		{
			name:    "placeholder-only block is content, not empty",
			runs:    []model.Run{hygJSXVar("1", "{p.price}", "p.price")},
			wantNot: []string{"empty", "leading-whitespace", "trailing-whitespace"},
		},
		{
			name:    "paired inline code wrapping the whole content",
			runs:    []model.Run{hygPcOpen("1"), hygTx("Bold text"), hygPcClose("1")},
			wantNot: []string{"leading-whitespace", "trailing-whitespace", "empty"},
		},
		{
			name:    "space, placeholder, space is not a double space",
			runs:    []model.Run{hygTx("Hello "), hygJSXVar("1", "{name}", "name"), hygTx(" world")},
			wantNot: []string{"double-spaces"},
		},
		{
			name:    "the same word either side of a placeholder is not a doubled word",
			runs:    []model.Run{hygTx("the "), hygJSXVar("1", "{x}", "x"), hygTx(" the end")},
			wantNot: []string{"doubled-word"},
		},
		{
			name:    "a placeholder is not a control character",
			runs:    []model.Run{hygJSXVar("1", "{x}", "x"), hygTx("ok")},
			wantNot: []string{"control-char"},
		},

		// Genuine positives, unchanged.
		{
			name: "genuine leading whitespace still fires",
			runs: []model.Run{hygTx(" Hello world")},
			want: []string{"leading-whitespace"},
		},
		{
			name: "genuine trailing whitespace still fires",
			runs: []model.Run{hygTx("Hello world ")},
			want: []string{"trailing-whitespace"},
		},
		{
			name: "leading whitespace before a placeholder still fires",
			runs: []model.Run{hygTx(" "), hygJSXVar("1", "{x}", "x"), hygTx(" each")},
			want: []string{"leading-whitespace"},
		},
		{
			name: "trailing whitespace after a placeholder still fires",
			runs: []model.Run{hygTx("Total: "), hygJSXVar("1", "{x}", "x"), hygTx("  ")},
			want: []string{"trailing-whitespace"},
		},
		{
			name: "a genuine double space inside one text run still fires",
			runs: []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" two  spaces")},
			want: []string{"double-spaces"},
		},
		{
			name: "a genuinely doubled word still fires",
			runs: []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" the the end")},
			want: []string{"doubled-word"},
		},
		{
			name: "a genuinely empty block still fires",
			runs: []model.Run{hygTx("   ")},
			want: []string{"empty"},
		},
		{
			name: "a block with no runs at all is empty",
			runs: nil,
			want: []string{"empty"},
		},
		{
			name: "a genuine control character still fires",
			runs: []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" Hello\x07world")},
			want: []string{"control-char"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hygieneCategories(t, tt.runs)
			for _, w := range tt.want {
				assert.Contains(t, got, w, "missing expected finding")
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, got, w, "false positive")
			}
		})
	}
}

// TestHygieneTextRendersCodesAsASentinel pins the flattening the rules depend
// on: inline code occupies exactly one position and contributes no text, so a
// placeholder's own equivalent can never leak into a word- or character-level
// judgement.
func TestHygieneTextRendersCodesAsASentinel(t *testing.T) {
	sentinel := string(model.ObjectReplacement)

	tests := []struct {
		name string
		runs []model.Run
		want string
	}{
		{
			name: "placeholder then text",
			runs: []model.Run{hygJSXVar("1", "{p.price}", "p.price"), hygTx(" each")},
			want: sentinel + " each",
		},
		{
			name: "paired code contributes one sentinel per side",
			runs: []model.Run{hygPcOpen("1"), hygTx("x"), hygPcClose("1")},
			want: sentinel + "x" + sentinel,
		},
		{
			name: "the placeholder's equiv text never appears",
			runs: []model.Run{hygJSXVar("1", "{p.price}", "p.price")},
			want: sentinel,
		},
		{
			name: "text-only runs are unchanged",
			runs: []model.Run{hygTx("Hello "), hygTx("world")},
			want: "Hello world",
		},
		{
			name: "no runs",
			runs: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, check.HygieneText(tt.runs))
		})
	}
}

// TestHygieneTextLeavesRunsTextAlone: RunsText is the coordinate space every
// stand-off overlay, content hash and TM key is anchored in, so the hygiene
// flattening must be a second, additive view — never a change to it.
func TestHygieneTextLeavesRunsTextAlone(t *testing.T) {
	runs := []model.Run{hygTx("Click "), hygJSXVar("1", "{x}", "x"), hygTx(" here")}
	assert.Equal(t, "Click  here", model.RunsText(runs),
		"RunsText must keep dropping inline code — overlays are anchored to it")
	assert.NotEqual(t, model.RunsText(runs), check.HygieneText(runs))
}
