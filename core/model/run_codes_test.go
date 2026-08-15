package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ph(id, equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Equiv: equiv, Data: "{" + equiv + "}"}}
}

func txt(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

// The gate every reader, writer, translator and editor uses to decide whether
// a block can travel as a string. Anything that is not text counts as a code,
// including the structured constructs, so a block holding one is never
// flattened.
func TestRunsHaveInlineCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		runs []model.Run
		want bool
	}{
		{"no runs at all", nil, false},
		{"empty slice", []model.Run{}, false},
		{"one text run", []model.Run{model.TextR("plain")}, false},
		{"several text runs", []model.Run{model.TextR("a"), model.TextR("b")}, false},
		{"an empty text run is still text", []model.Run{model.TextR("")}, false},
		{"a placeholder", []model.Run{model.PhR(model.PlaceholderRun{ID: "1"})}, true},
		{"text then a placeholder", []model.Run{model.TextR("Hi "), model.PhR(model.PlaceholderRun{ID: "1"})}, true},
		{"a paired open", []model.Run{model.PcOpenR(model.PcOpenRun{ID: "b"})}, true},
		{"a paired close", []model.Run{model.PcCloseR(model.PcCloseRun{ID: "b"})}, true},
		{"a subflow reference", []model.Run{model.SubR(model.SubRun{ID: "s"})}, true},
		{"a plural construct", []model.Run{model.PluralR(model.PluralRun{})}, true},
		{"a select construct", []model.Run{model.SelectR(model.SelectRun{})}, true},
		{"a zero Run sets no discriminator and counts as a code", []model.Run{{}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, model.RunsHaveInlineCodes(tc.runs))
		})
	}
}

func TestRunCodeSignature(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  model.Run
		want string
		ok   bool
	}{
		{name: "placeholder keys on equiv", run: ph("1", "count"), want: "ph:count", ok: true},
		{
			name: "placeholder without equiv falls back to id",
			run:  model.Run{Ph: &model.PlaceholderRun{ID: "7"}},
			want: "ph:7", ok: true,
		},
		{
			name: "paired open keys on id",
			run:  model.Run{PcOpen: &model.PcOpenRun{ID: "2"}},
			want: "pc-open:2", ok: true,
		},
		{
			name: "paired close keys on id",
			run:  model.Run{PcClose: &model.PcCloseRun{ID: "2"}},
			want: "pc-close:2", ok: true,
		},
		{
			name: "subblock keys on id",
			run:  model.Run{Sub: &model.SubRun{ID: "s1"}},
			want: "sub:s1", ok: true,
		},
		{name: "text carries no code", run: txt("hello")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := model.RunCodeSignature(tc.run)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDiffRunCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		source      []model.Run
		target      []model.Run
		wantMissing []string
		wantExtra   []string
	}{
		{
			name:   "identical codes are balanced",
			source: []model.Run{ph("1", "count"), txt(" documented formats")},
			target: []model.Run{ph("1", "count"), txt(" dokumenterte formater")},
		},
		{
			name:   "reordered codes are balanced",
			source: []model.Run{ph("1", "count"), txt(" formats")},
			target: []model.Run{txt("formater "), ph("1", "count")},
		},
		{
			name:        "target dropped the placeholder",
			source:      []model.Run{ph("1", "documentedCount"), txt(" documented formats")},
			target:      []model.Run{txt(" dokumenterte formater")},
			wantMissing: []string{"ph:documentedCount"},
		},
		{
			name:      "target invented a placeholder",
			source:    []model.Run{txt("Install")},
			target:    []model.Run{ph("1", "count"), txt(" Installer")},
			wantExtra: []string{"ph:count"},
		},
		{
			name:   "code-free on both sides is balanced",
			source: []model.Run{txt("Install")},
			target: []model.Run{txt("Installer")},
		},
		{
			name:        "repeated placeholder used once is a partial loss",
			source:      []model.Run{ph("1", "n"), txt(" of "), ph("1", "n")},
			target:      []model.Run{ph("1", "n"), txt(" av total")},
			wantMissing: []string{"ph:n"},
		},
		{
			name:   "unbalanced paired code is reported per side",
			source: []model.Run{{PcOpen: &model.PcOpenRun{ID: "0"}}, txt("bold"), {PcClose: &model.PcCloseRun{ID: "0"}}},
			target: []model.Run{{PcOpen: &model.PcOpenRun{ID: "0"}}, txt("fet")},
			// The close tag vanished: the writer would emit an unterminated pair.
			wantMissing: []string{"pc-close:0"},
		},
		{
			name: "plural branches fold by highest per-branch use",
			source: []model.Run{{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {txt("1 file")},
				model.PluralOther: {ph("1", "n"), txt(" files")},
			}}}},
			target: []model.Run{{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {txt("1 fil")},
				model.PluralOther: {ph("1", "n"), txt(" filer")},
			}}}},
		},
		{
			name: "plural target that dropped the pivot placeholder is lossy",
			source: []model.Run{{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
				model.PluralOther: {ph("1", "n"), txt(" files")},
			}}}},
			target: []model.Run{{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
				model.PluralOther: {txt("filer")},
			}}}},
			wantMissing: []string{"ph:n"},
		},
		{
			name:        "empty target loses every source code",
			source:      []model.Run{ph("1", "a"), ph("2", "b")},
			target:      nil,
			wantMissing: []string{"ph:a", "ph:b"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := model.DiffRunCodes(tc.source, tc.target)
			assert.Equal(t, tc.wantMissing, d.MissingCodes())
			assert.Equal(t, tc.wantExtra, d.ExtraCodes())
			assert.Equal(t, len(tc.wantMissing) == 0 && len(tc.wantExtra) == 0, d.Balanced())
			assert.Equal(t, len(tc.wantMissing) > 0, d.Lossy())
		})
	}
}

func TestRunCodeCountsMultiset(t *testing.T) {
	t.Parallel()
	counts := model.RunCodeCounts([]model.Run{ph("1", "n"), txt(" of "), ph("1", "n"), ph("2", "total")})
	require.Len(t, counts, 2)
	assert.Equal(t, 2, counts["ph:n"])
	assert.Equal(t, 1, counts["ph:total"])
}
