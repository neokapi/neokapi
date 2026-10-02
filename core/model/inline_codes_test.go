package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInlineCodesPreserved(t *testing.T) {
	open := func(id string) Run { return PcOpenR(PcOpenRun{ID: id}) }
	closeRun := func(id string) Run { return PcCloseR(PcCloseRun{ID: id}) }
	paired := []Run{open("b"), open("i"), TextR("text"), closeRun("i"), closeRun("b")}
	plural := func(one, other []Run) Run {
		return PluralR(PluralRun{Pivot: "count", Forms: map[PluralForm][]Run{
			PluralOne: one, PluralOther: other,
		}})
	}
	selectRun := func(pivot string, other []Run) Run {
		return SelectR(SelectRun{Pivot: pivot, Cases: map[string][]Run{"other": other}})
	}
	nested := []Run{plural([]Run{TextR("one")}, []Run{selectRun("kind", paired)})}
	tests := []struct {
		name   string
		source []Run
		edited []Run
		want   bool
	}{
		{name: "plain text", source: []Run{TextR("old")}, edited: []Run{TextR("new")}, want: true},
		{name: "nested markup", source: paired, edited: paired, want: true},
		{name: "crossed markup", source: paired,
			edited: []Run{open("b"), open("i"), TextR("text"), closeRun("b"), closeRun("i")}},
		{name: "invalid union", source: []Run{TextR("old")},
			edited: []Run{{Text: &TextRun{Text: "new"}, Ph: &PlaceholderRun{ID: "hidden"}}}},
		{name: "nested content survives", source: nested, edited: nested, want: true},
		{name: "flattened plural", source: nested, edited: []Run{TextR("many")}},
		{name: "removed plural form", source: nested,
			edited: []Run{PluralR(PluralRun{Pivot: "count", Forms: map[PluralForm][]Run{PluralOther: paired}})}},
		{name: "changed pivot", source: []Run{selectRun("kind", paired)},
			edited: []Run{selectRun("different", paired)}},
		{name: "dropped nested code", source: nested,
			edited: []Run{plural([]Run{TextR("one")}, []Run{selectRun("kind", []Run{TextR("many")})})}},
		{name: "invented branch", source: []Run{TextR("old")}, edited: nested},
		{name: "nested wording edit", source: nested,
			edited: []Run{plural([]Run{TextR("a single item")}, []Run{selectRun("kind", paired)})}, want: true},
		{name: "codes cannot move across branches",
			source: []Run{plural(paired, []Run{TextR("other")})},
			edited: []Run{plural([]Run{TextR("one")}, paired)}},
		// XLIFF 1.2 and TMX <bpt>/<ept> pairs may overlap in the source. An
		// edit that keeps the overlap is a text edit, and a close still may
		// not precede its open.
		{name: "overlapping source keeps its overlap",
			source: []Run{open("1"), TextR("bold "), open("2"), TextR("both"), closeRun("1"), TextR(" italic"), closeRun("2")},
			edited: []Run{open("1"), TextR("gras "), open("2"), TextR("les deux"), closeRun("1"), TextR(" italique"), closeRun("2")},
			want:   true},
		{name: "overlapping source rejects a close before its open",
			source: []Run{open("1"), TextR("bold "), open("2"), TextR("both"), closeRun("1"), TextR(" italic"), closeRun("2")},
			edited: []Run{closeRun("1"), open("1"), TextR("text"), open("2"), closeRun("2")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InlineCodesPreserved(tt.source, tt.edited))
		})
	}
}
