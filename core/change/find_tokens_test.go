package change_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A find is placeholder text, read as set_content's text is: a token in it
// matches only the code with that id, at that point, and the replacement,
// read the same way, takes the place of the runs the find matched.
func TestApplyBlock_FindNamesCodesByToken(t *testing.T) {
	t.Run("a token matches its code and the replacement keeps it", func(t *testing.T) {
		b := guideBlock()
		res := apply(t, b, person, replace("", sourceRev(b),
			find(`<x id="1"/>shop guide<x id="/1"/>`, `<x id="1"/>handbook<x id="/1"/>`)))
		requireApplied(t, res)
		assert.Equal(t, "Read the <1>handbook</1> before you <2>order</2>.", shape(b.Source))
		// The span opens at the link's opening code and ends after its
		// closing one.
		assert.Equal(t, []change.Resolved{{Start: change.Position{Run: 1}, End: change.Position{Run: 4}}}, res[0].Resolved)
		// The link keeps its native form and attributes.
		require.NotNil(t, b.Source[1].PcOpen)
		assert.Equal(t, `<a href="https://old.example/guide">`, b.Source[1].PcOpen.Data)
	})

	t.Run("text and a token across a run boundary", func(t *testing.T) {
		b := guideBlock()
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b),
			find(`the <x id="1"/>shop`, `the <x id="1"/>store`))))
		assert.Equal(t, "Read the <1>store guide</1> before you <2>order</2>.", shape(b.Source))
	})

	t.Run("a find of text alone behaves as before", func(t *testing.T) {
		b := guideBlock()
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b), find("guide before", "manual, before"))))
		assert.Equal(t, "Read the <1>shop </1>manual, before you <2>order</2>.", shape(b.Source),
			"codes have zero width and stay where model.ApplyTextEdits places them")
	})

	t.Run("a token that is not at that point does not match", func(t *testing.T) {
		b := guideBlock()
		err := requireRefused(t, apply(t, b, person, replace("", sourceRev(b),
			find(`<x id="2"/>shop guide`, "handbook")))[0], change.CodeNotFound)
		assert.Equal(t, "edits/0/find", err.Field)
	})

	t.Run("a token that names no code of the text is refused naming it", func(t *testing.T) {
		b := guideBlock()
		err := requireRefused(t, apply(t, b, person, replace("", sourceRev(b),
			find(`<x id="9"/>shop guide`, "handbook")))[0], change.CodeNotFound)
		assert.Contains(t, err.Message, `<x id="9"/>`)
		assert.Contains(t, err.Message, "a code the text does not hold")
	})

	t.Run("dropping a code that may be deleted lands", func(t *testing.T) {
		b := guideBlock()
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b),
			find(`<x id="2"/>order<x id="/2"/>`, "order"))))
		assert.Equal(t, "Read the <1>shop guide</1> before you order.", shape(b.Source), "bold may be deleted")
	})

	t.Run("dropping a code that may not be deleted is refused", func(t *testing.T) {
		b := model.NewRunsBlock("p", pluralRuns())
		path := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
		edit := find(`<x id="n/"/> item`, "one item")
		edit.Path = path
		err := requireRefused(t, apply(t, b, person, replace("", sourceRev(b), edit))[0], change.CodeGuard)
		assert.Equal(t, change.SubcodeCodesChanged, err.Subcode)
		assert.Contains(t, err.Message, `drops <x id="n/"/>`)
		assert.Equal(t, "You have {count: one={n} item other={n} items} in your basket.", shape(b.Source))
	})

	t.Run("a token in a branch with the branch's path", func(t *testing.T) {
		b := model.NewRunsBlock("p", pluralRuns())
		edit := find(`<x id="n/"/> item`, `<x id="n/"/> thing`)
		edit.Path = model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b), edit)))
		assert.Equal(t, "You have {count: one={n} thing other={n} items} in your basket.", shape(b.Source))
	})

	t.Run("without the path a token find names the branch that holds it", func(t *testing.T) {
		b := model.NewRunsBlock("p", pluralRuns())
		err := requireRefused(t, apply(t, b, person, replace("", sourceRev(b),
			find(`<x id="n/"/> items`, `<x id="n/"/> things`)))[0], change.CodeNotFound)
		assert.Contains(t, err.Message, `it is in the branch at path [1,{"plural":"other"}]`)
	})
}

// arbRuns is an ARB plural as the ARB reader builds it: the ICU argument is a
// placeholder whose native form is its source.
//
//	{count, plural, =0{No new messages} one{{count} new message} other{{count} new messages}}
func arbRuns() []model.Run {
	count := func() model.Run {
		return model.PhR(model.PlaceholderRun{ID: "p1", Type: "icu", Data: "{count}", Equiv: "{count}", Disp: "{count}"})
	}
	return []model.Run{model.PluralR(model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		"=0":              {model.TextR("No new messages")},
		model.PluralOne:   {count(), model.TextR(" new message")},
		model.PluralOther: {count(), model.TextR(" new messages")},
	}})}
}

// In an ARB branch an argument may be named as an ICU writer spells it,
// {count}, where set_content's text accepts it too, or by its token.
func TestApplyBlock_FindNamesAnICUArgumentByItsSource(t *testing.T) {
	one := model.RunPath{{Kind: model.StepIndex, Index: 0}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
	for name, edit := range map[string]change.TextEdit{
		"by source": find("{count} new message", "{count} unread message"),
		"by token":  find(`<x id="p1/"/> new message`, `<x id="p1/"/> unread message`),
		"mixed":     find("{count} new message", `<x id="p1/"/> unread message`),
	} {
		t.Run(name, func(t *testing.T) {
			b := model.NewRunsBlock("inboxCount", arbRuns())
			edit.Path = one
			res := apply(t, b, agent, replace("", sourceRev(b), edit))
			requireApplied(t, res)
			assert.Equal(t, "{count: one={p1} unread message other={p1} new messages}", shape(b.Source))
			assert.Equal(t, "{count}", b.Source[0].Plural.Forms[model.PluralOne][0].Ph.Data, "the argument keeps its native form")
			assert.Equal(t, []change.Resolved{{Path: one, Start: change.Position{}, End: change.Position{Run: 2}}}, res[0].Resolved)
		})
	}
	t.Run("a literal brace still matches literal text", func(t *testing.T) {
		b := model.NewRunsBlock("b", []model.Run{model.TextR("Use {count} as written.")})
		requireApplied(t, apply(t, b, agent, replace("", sourceRev(b), find("{count}", "{n}"))))
		assert.Equal(t, "Use {n} as written.", b.SourceText())
	})
}

// refRuns is "Total:", n codes in a row and " 42 items", as the HTML reader
// keeps "Total:&nbsp;&nbsp;… 42 items": each code is a placeholder a find may
// name by the text it stands for.
func refRuns(n int, data string) []model.Run {
	runs := []model.Run{model.TextR("Total:")}
	for i := range n {
		runs = append(runs, model.PhR(model.PlaceholderRun{ID: fmt.Sprintf("r%d", i), Type: "ref", Data: data}))
	}
	return append(runs, model.TextR(" 42 items"))
}

// withinTime runs f and fails the test if it has not returned by the limit.
func withinTime(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("did not finish within %s", limit)
	}
}

// A code a find may name by its text can be taken that way or passed over, so
// a find over n of them in a row has 2^n ways to match. One that fails must
// still fail in time proportional to the text, not to 2^n.
func TestApplyBlock_FindOverManyNamedCodes(t *testing.T) {
	const n = 64
	for name, data := range map[string]string{
		"nbsp":      "&nbsp;",
		"ampersand": "&amp;",
		"icu":       "{count}",
	} {
		alias := data
		if chars, ok := model.CharacterReference(&model.PlaceholderRun{Data: data}); ok {
			alias = chars
		}
		t.Run(name+": a find that fails", func(t *testing.T) {
			b := model.NewRunsBlock("total", refRuns(n, data))
			// The find a reader copies from the read with one digit changed.
			miss := "Total:" + strings.Repeat(alias, n) + " 43 items"
			var res []change.OpResult
			withinTime(t, 5*time.Second, func() {
				res = change.ApplyBlock(b, []change.Op{replace("", sourceRev(b), find(miss, "x"))}, person)
			})
			require.Len(t, res, 1)
			requireRefused(t, res[0], change.CodeNotFound)
		})
		t.Run(name+": a find that holds", func(t *testing.T) {
			b := model.NewRunsBlock("total", refRuns(n, data))
			hit := "Total:" + strings.Repeat(alias, n) + " 42 items"
			with := "Total:" + strings.Repeat(alias, n) + " 43 items"
			var res []change.OpResult
			withinTime(t, 5*time.Second, func() {
				res = change.ApplyBlock(b, []change.Op{replace("", sourceRev(b), find(hit, with))}, person)
			})
			requireApplied(t, res)
			require.Len(t, b.Source, n+2)
			assert.Equal(t, "Total:", b.Source[0].Text.Text)
			for i := 1; i <= n; i++ {
				require.NotNil(t, b.Source[i].Ph, "run %d", i)
				assert.Equal(t, data, b.Source[i].Ph.Data, "every code keeps its spelling")
			}
			assert.Equal(t, " 43 items", b.Source[n+1].Text.Text)
		})
	}
}
