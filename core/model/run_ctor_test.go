package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunConstructors(t *testing.T) {
	tests := []struct {
		name string
		run  Run
		kind RunKind
	}{
		{"TextR", TextR("Hello"), RunKindText},
		{"PhR", PhR(PlaceholderRun{ID: "1", Equiv: "name"}), RunKindPh},
		{"PcOpenR", PcOpenR(PcOpenRun{ID: "1", Data: "<b>"}), RunKindPcOpen},
		{"PcCloseR", PcCloseR(PcCloseRun{ID: "1", Data: "</b>"}), RunKindPcClose},
		{"SubR", SubR(SubRun{ID: "1", Ref: "sub-1"}), RunKindSub},
		{"PluralR", PluralR(PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{PluralOther: {TextR("items")}}}), RunKindPlural},
		{"SelectR", SelectR(SelectRun{Pivot: "g", Cases: map[string][]Run{"other": {TextR("they")}}}), RunKindSelect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.run.Valid(), "constructor must produce a valid Run")
			assert.Equal(t, tt.kind, tt.run.Kind())
		})
	}

	t.Run("constructors copy their argument", func(t *testing.T) {
		p := PlaceholderRun{ID: "1"}
		r := PhR(p)
		p.ID = "mutated"
		assert.Equal(t, "1", r.Ph.ID, "the Run must not alias the caller's value")
	})

	t.Run("TextR round-trips through FlattenRuns", func(t *testing.T) {
		require.Equal(t, "Hello world", FlattenRuns([]Run{TextR("Hello "), TextR("world")}))
	})
}

func TestRunValid(t *testing.T) {
	t.Run("zero Run is invalid", func(t *testing.T) {
		assert.False(t, Run{}.Valid())
		assert.Equal(t, RunKind(""), Run{}.Kind())
	})
	t.Run("two discriminators are invalid", func(t *testing.T) {
		r := Run{Text: &TextRun{Text: "x"}, Ph: &PlaceholderRun{ID: "1"}}
		assert.False(t, r.Valid())
	})
	t.Run("each single discriminator is valid", func(t *testing.T) {
		runs := []Run{
			{Text: &TextRun{}}, {Ph: &PlaceholderRun{}}, {PcOpen: &PcOpenRun{}},
			{PcClose: &PcCloseRun{}}, {Sub: &SubRun{}}, {Plural: &PluralRun{}}, {Select: &SelectRun{}},
		}
		for i, r := range runs {
			assert.True(t, r.Valid(), "run %d", i)
		}
	})
}
