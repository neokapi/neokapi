package change_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

func apply(t *testing.T, b *model.Block, env change.BlockEnv, ops ...change.Op) []change.OpResult {
	t.Helper()
	res := change.ApplyBlock(b, ops, env)
	require.Len(t, res, len(ops))
	return res
}

func requireApplied(t *testing.T, res []change.OpResult) {
	t.Helper()
	for _, r := range res {
		require.Contains(t, []change.OpStatus{change.OpApplied, change.OpUnchanged}, r.Status, "op %d: %+v", r.I, r.Error)
	}
}

func requireRefused(t *testing.T, r change.OpResult, code change.Code) change.Error {
	t.Helper()
	require.Equal(t, change.OpRefused, r.Status)
	require.NotNil(t, r.Error)
	require.Equal(t, code, r.Error.Code, r.Error.Message)
	return *r.Error
}

func sourceRev(b *model.Block) string { return model.EditionRevision(b, model.EditionKey{}) }

func editionRev(b *model.Block, e string) string {
	k, _ := model.ParseEditionKey(e)
	return model.EditionRevision(b, k)
}

// Text is a form of runs, read against the codes of the content it replaces.
func TestApplyBlock_SetContentText(t *testing.T) {
	b := guideBlock()
	res := apply(t, b, person, setText("", sourceRev(b),
		`Read the <x id="1"/>handbook<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.`))
	requireApplied(t, res)
	assert.Equal(t, "Read the <1>handbook</1> before you <2>order</2>.", shape(b.Source))
	assert.Equal(t, `<a href="https://old.example/guide">`, b.Source[1].PcOpen.Data, "the code keeps its native form")
	assert.Equal(t, res[0].After, sourceRev(b))
	assert.NotEqual(t, res[0].Before, res[0].After)
}

// Creating an edition from text keeps the markup of the authoritative edition.
func TestApplyBlock_CreatesAnEditionWithTheAuthoritativeCodes(t *testing.T) {
	b := guideBlock()
	res := apply(t, b, agent, setText("de", model.AbsentRevision,
		`Lies den <x id="1"/>Ladenführer<x id="/1"/>, bevor du <x id="2"/>bestellst<x id="/2"/>.`))
	requireApplied(t, res)
	de := b.TargetRuns("de")
	assert.Equal(t, "Lies den <1>Ladenführer</1>, bevor du <2>bestellst</2>.", shape(de))
	assert.Equal(t, `<a href="https://old.example/guide">`, de[1].PcOpen.Data)
	assert.Equal(t, sourceRev(b), res[0].Basis, "the basis is the authoritative revision the edition was made from")
	assert.Equal(t, model.AbsentRevision, res[0].Before)

	again := apply(t, b, agent, setText("de", model.AbsentRevision, "Hallo"))
	requireRefused(t, again[0], change.CodeStale)
	assert.Equal(t, editionRev(b, "de"), again[0].Current.Rev)
}

func TestApplyBlock_InlineCodeGuards(t *testing.T) {
	br := model.PhR(model.PlaceholderRun{ID: "br", Type: "struct:break", Data: "<br/>"})
	withBreak := []model.Run{model.TextR("Line one"), br, model.TextR("line two")}
	tests := []struct {
		name  string
		runs  []model.Run
		text  string
		code  change.Code
		sub   change.Subcode
		shape string
	}{
		{name: "a deletable code may go", runs: guideRuns(),
			text:  `Read the <x id="1"/>handbook<x id="/1"/> before you order.`,
			shape: "Read the <1>handbook</1> before you order."},
		{name: "a code that may not be deleted stays", runs: withBreak,
			text: "Line one line two", code: change.CodeGuard, sub: change.SubcodeCodesChanged},
		{name: "a code that may not be copied is not repeated", runs: withBreak,
			text: `Line one<x id="br/"/><x id="br/"/>line two`, code: change.CodeGuard, sub: change.SubcodeCodesChanged},
		{name: "a deletable, cloneable code may repeat", runs: guideRuns(),
			text:  `<x id="2"/>Read<x id="/2"/> the <x id="1"/>guide<x id="/1"/> before you <x id="2"/>order<x id="/2"/>.`,
			shape: "<2>Read</2> the <1>guide</1> before you <2>order</2>."},
		{name: "a code the edition does not hold", runs: guideRuns(),
			text: `Read the <x id="9"/>guide<x id="/9"/>.`, code: change.CodeGuard, sub: change.SubcodeCodesChanged},
		{name: "crossed paired codes", runs: guideRuns(),
			text: `Read the <x id="1"/>shop <x id="2"/>guide<x id="/1"/> order<x id="/2"/>.`, code: change.CodeGuard, sub: change.SubcodeCodesChanged},
		{name: "a code that may not be reordered keeps its place",
			runs: []model.Run{model.PcOpenR(model.PcOpenRun{ID: "b", Type: "fmt:bold", Data: "<b>"}), model.TextR("A"), model.PcCloseR(model.PcCloseRun{ID: "b", Data: "</b>"}), br, model.TextR("B")},
			text: `<x id="br/"/>B <x id="b"/>A<x id="/b"/>`, code: change.CodeGuard, sub: change.SubcodeCodesChanged},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := model.NewRunsBlock("b", tc.runs)
			before := shape(b.Source)
			res := apply(t, b, person, setText("", sourceRev(b), tc.text))
			if tc.code == "" {
				requireApplied(t, res)
				assert.Equal(t, tc.shape, shape(b.Source))
				return
			}
			err := requireRefused(t, res[0], tc.code)
			assert.Equal(t, tc.sub, err.Subcode)
			assert.Equal(t, before, shape(b.Source), "nothing is written")
		})
	}
}

// A tool's draft lands with a guard violation among its findings, to meet the
// ship gates later.
func TestApplyBlock_ReportDispositionLandsWithFindings(t *testing.T) {
	br := model.PhR(model.PlaceholderRun{ID: "br", Type: "struct:break", Data: "<br/>"})
	b := model.NewRunsBlock("b", []model.Run{model.TextR("one"), br, model.TextR("two")})
	res := apply(t, b, tool, setRuns("fr", model.AbsentRevision, []model.Run{model.TextR("un deux")}))
	requireApplied(t, res)
	require.Len(t, res[0].Findings, 1)
	assert.Equal(t, "guard.codes_changed", res[0].Findings[0].Rule)
	assert.True(t, res[0].Findings[0].Fails)
	assert.Equal(t, "un deux", b.TargetText("fr"))
}

// Structure is kept: text replaces a branch named by path, and a whole
// structure only as runs.
func TestApplyBlock_PluralStructure(t *testing.T) {
	onePath := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
	fewPath := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralFew}}

	t.Run("text over the whole edition is refused", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		res := apply(t, b, person, setText("", sourceRev(b), "You have items in your basket."))
		err := requireRefused(t, res[0], change.CodeGuard)
		assert.Equal(t, change.SubcodeStructureLost, err.Subcode)
	})
	t.Run("text replaces the branch a path names", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		op := setText("", sourceRev(b), `<x id="n/"/> article`)
		op.Body.(*change.SetContent).Path = onePath
		requireApplied(t, apply(t, b, person, op))
		assert.Equal(t, "You have {count: one={n} article other={n} items} in your basket.", shape(b.Source))
		assert.Equal(t, "#", b.Source[1].Plural.Forms[model.PluralOne][0].Ph.Data, "the placeholder keeps its native form")
	})
	t.Run("a path to a missing form adds it", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		op := setText("", sourceRev(b), `<x id="n/"/> items (few)`)
		op.Body.(*change.SetContent).Path = fewPath
		requireApplied(t, apply(t, b, person, op))
		assert.Equal(t, "You have {count: one={n} item few={n} items (few) other={n} items} in your basket.", shape(b.Source))
	})
	t.Run("a path that reaches nothing", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		op := setText("", sourceRev(b), "x")
		op.Body.(*change.SetContent).Path = model.RunPath{{Kind: model.StepIndex, Index: 0}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
		requireRefused(t, apply(t, b, person, op)[0], change.CodeNotFound)
	})
	t.Run("runs replace the whole structure", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		n := model.PhR(model.PlaceholderRun{ID: "n", Type: "code:variable"})
		requireApplied(t, apply(t, b, person, setRuns("", sourceRev(b), []model.Run{model.TextR("Items in your basket: "), n})))
		assert.Equal(t, "Items in your basket: {n}", shape(b.Source))
		assert.Equal(t, "#", b.Source[1].Ph.Data)
	})
	t.Run("runs that drop a variable the structure held", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		err := requireRefused(t, apply(t, b, person, setRuns("", sourceRev(b), []model.Run{model.TextR("Your basket")}))[0], change.CodeGuard)
		assert.Equal(t, change.SubcodeCodesChanged, err.Subcode)
	})
	t.Run("replace_text inside a branch", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		e := find("item", "article")
		e.Path = onePath
		res := apply(t, b, person, replace("", sourceRev(b), e))
		requireApplied(t, res)
		assert.Equal(t, "You have {count: one={n} article other={n} items} in your basket.", shape(b.Source))
		assert.Equal(t, []change.Resolved{{Path: onePath, Start: model.RunPos{Run: 1, Offset: 1}, End: model.RunPos{Run: 2}}}, res[0].Resolved)
	})
	t.Run("a find that spans the plural", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		err := requireRefused(t, apply(t, b, person, replace("", sourceRev(b), find("have  in", "own in")))[0], change.CodeGuard)
		assert.Equal(t, change.SubcodeStructureLost, err.Subcode)
	})
	t.Run("text around the plural is edited in place", func(t *testing.T) {
		b := model.NewRunsBlock("b", pluralRuns())
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b), find("basket", "cart"))))
		assert.Equal(t, "You have {count: one={n} item other={n} items} in your cart.", shape(b.Source))
	})
}

// A runs payload names codes by id and carries no native form; the applier
// takes it from the code the edition holds.
func TestApplyBlock_SetContentRuns(t *testing.T) {
	wire := func(href string) []model.Run {
		return []model.Run{
			model.TextR("Read the "),
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Attrs: map[string]string{"href": href}}),
			model.TextR("handbook"),
			model.PcCloseR(model.PcCloseRun{ID: "1"}),
			model.TextR("."),
		}
	}
	t.Run("a held code takes its native form", func(t *testing.T) {
		b := guideBlock()
		requireApplied(t, apply(t, b, person, setRuns("", sourceRev(b), wire("https://old.example/guide"))))
		assert.Equal(t, "Read the <1>handbook</1>.", shape(b.Source))
		assert.Equal(t, `<a href="https://old.example/guide">`, b.Source[1].PcOpen.Data)
		assert.Equal(t, "</a>", b.Source[3].PcClose.Data)
	})
	t.Run("a changed attribute with no native form is refused", func(t *testing.T) {
		b := guideBlock()
		err := requireRefused(t, apply(t, b, person, setRuns("", sourceRev(b), wire("https://new.example")))[0], change.CodeUnsupported)
		assert.Equal(t, "set_attribute", err.Capability)
		assert.Equal(t, "Read the <1>shop guide</1> before you <2>order</2>.", shape(b.Source))
	})
	t.Run("a new code with no native form is refused", func(t *testing.T) {
		b := guideBlock()
		runs := append(wire("https://old.example/guide"), model.PhR(model.PlaceholderRun{ID: "9", Type: "media:image"}))
		err := requireRefused(t, apply(t, b, person, setRuns("", sourceRev(b), runs))[0], change.CodeUnsupported)
		assert.Equal(t, "synthesize:media:image", err.Capability)
	})
	t.Run("a new code an in-process caller spells is kept", func(t *testing.T) {
		b := guideBlock()
		runs := append(wire("https://old.example/guide"), model.PhR(model.PlaceholderRun{ID: "9", Type: "x-redacted", Data: "{{EMAIL}}"}))
		requireApplied(t, apply(t, b, tool, setRuns("", sourceRev(b), runs)))
		assert.Equal(t, "{{EMAIL}}", b.Source[5].Ph.Data)
	})
	t.Run("a translation takes back a code the source holds", func(t *testing.T) {
		b := guideBlock()
		res := apply(t, b, person, setText("nb", editionRev(b, "nb"), `Les <x id="1"/>håndboka<x id="/1"/> før du <x id="2"/>bestiller<x id="/2"/>.`))
		requireApplied(t, res)
		b.Target("nb").Runs = []model.Run{model.TextR("Les håndboka.")}
		res = apply(t, b, person, setText("nb", editionRev(b, "nb"), `Les <x id="1"/>håndboka<x id="/1"/>.`))
		requireApplied(t, res)
		assert.Equal(t, "Les <1>håndboka</1>.", shape(b.TargetRuns("nb")))
		assert.Equal(t, `<a href="https://old.example/guide">`, b.TargetRuns("nb")[1].PcOpen.Data)
	})
}

// The three position forms resolve to the same edit, and the result echoes
// the run positions under RangeAnchor's attribution.
func TestApplyBlock_ReplaceTextPositions(t *testing.T) {
	// "shop guide" is [9, 19) of the text. A boundary at the end of a text run
	// is the start of the run after it, so the span opens before the link's
	// opening code and closes before its closing one, as RangeAnchor puts it.
	a := model.RangeAnchor(guideRuns(), 9, 19)
	want := []change.Resolved{{Start: a.Start, End: a.End}}
	require.Equal(t, model.RunPos{Run: 1}, a.Start)
	require.Equal(t, model.RunPos{Run: 3}, a.End)
	rng := change.TextEdit{Range: &change.Span{Start: model.RunPos{Run: 2}, End: model.RunPos{Run: 2, Offset: 10}}, Text: "handbook"}
	for name, edit := range map[string]change.TextEdit{
		"find":           find("shop guide", "handbook"),
		"start and end":  span(9, 19, "handbook"),
		"run positions":  rng,
		"find, the only": {Find: new("shop guide"), Occurrence: 1, Text: "handbook"},
	} {
		t.Run(name, func(t *testing.T) {
			b := guideBlock()
			res := apply(t, b, person, replace("", sourceRev(b), edit))
			requireApplied(t, res)
			assert.Equal(t, "Read the <1>handbook</1> before you <2>order</2>.", shape(b.Source))
			assert.Equal(t, want, res[0].Resolved)
		})
	}
}

func TestApplyBlock_ReplaceTextRefusals(t *testing.T) {
	b := model.NewRunsBlock("b", []model.Run{model.TextR("one two one two one")})
	tests := []struct {
		name  string
		edits []change.TextEdit
		code  change.Code
		sub   change.Subcode
		check func(t *testing.T, err change.Error)
	}{
		{name: "an ambiguous find", edits: []change.TextEdit{find("one", "1")}, code: change.CodeAmbiguous,
			check: func(t *testing.T, err change.Error) {
				require.Len(t, err.Candidates, 3)
				assert.Equal(t, 2, err.Candidates[1].Occurrence)
				assert.Equal(t, model.RunPos{Run: 0, Offset: 8}, err.Candidates[1].At.Start)
			}},
		{name: "a find with no match", edits: []change.TextEdit{find("three", "3")}, code: change.CodeNotFound},
		{name: "an occurrence past the matches", edits: []change.TextEdit{{Find: new("two"), Occurrence: 3, Text: "2"}}, code: change.CodeNotFound},
		{name: "offsets outside the text", edits: []change.TextEdit{span(10, 40, "x")}, code: change.CodeGuard, sub: change.SubcodeBadPosition},
		{name: "a range outside the runs", edits: []change.TextEdit{{Range: &change.Span{Start: model.RunPos{Run: 0}, End: model.RunPos{Run: 3}}}}, code: change.CodeGuard, sub: change.SubcodeBadPosition},
		{name: "overlapping edits", edits: []change.TextEdit{span(0, 5, "x"), span(3, 7, "y")}, code: change.CodeGuard, sub: change.SubcodeOverlap},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := apply(t, b, person, replace("", sourceRev(b), tc.edits...))
			err := requireRefused(t, res[0], tc.code)
			assert.Equal(t, tc.sub, err.Subcode)
			if tc.check != nil {
				tc.check(t, err)
			}
			assert.Equal(t, "one two one two one", b.SourceText())
		})
	}
	res := apply(t, b, person, replace("", sourceRev(b), change.TextEdit{Find: new("two"), Occurrence: 2, Text: "2"}, span(0, 3, "1")))
	requireApplied(t, res)
	assert.Equal(t, "1 two one 2 one", b.SourceText(), "edits resolve against the base and apply sorted")
}

// Run flags survive an edit in either form (main-model P2).
func TestApplyBlock_KeepsNoTranslate(t *testing.T) {
	t.Run("replace_text", func(t *testing.T) {
		b := model.NewRunsBlock("b", codeSpanRuns())
		requireApplied(t, apply(t, b, person, replace("", sourceRev(b), find("Run", "Execute"))))
		assert.Equal(t, "Execute <1>[kapi check]</1> in CI.", shape(b.Source))
	})
	t.Run("set_content text", func(t *testing.T) {
		b := model.NewRunsBlock("b", codeSpanRuns())
		requireApplied(t, apply(t, b, person, setText("", sourceRev(b), `Execute <x id="1"/>kapi check<x id="/1"/> in CI.`)))
		assert.Equal(t, "Execute <1>[kapi check]</1> in CI.", shape(b.Source))
	})
	t.Run("offsets count code points", func(t *testing.T) {
		b := model.NewRunsBlock("b", []model.Run{model.TextR("Blåbær og "), {Text: &model.TextRun{Text: "øl", NoTranslate: true}}})
		res := apply(t, b, person, replace("", sourceRev(b), span(7, 9, "eller")))
		requireApplied(t, res)
		assert.Equal(t, "Blåbær eller [øl]", shape(b.Source))
	})
}

// Every if_match is checked against the block as it stood, nothing lands when
// one operation is refused, and later operations see earlier ones.
func TestApplyBlock_PreconditionsAndAtomicity(t *testing.T) {
	t.Run("a refusal leaves the block as it was", func(t *testing.T) {
		b := guideBlock()
		before := shape(b.Source)
		res := apply(t, b, person,
			replace("", sourceRev(b), find("shop guide", "handbook")),
			replace("nb", "r:0000000000000000", find("Les", "Lees")),
			remove("nb", "*"))
		assert.Equal(t, change.OpNotApplied, res[0].Status)
		requireRefused(t, res[1], change.CodeStale)
		assert.Equal(t, "if_match", res[1].Error.Field)
		require.NotNil(t, res[1].Current)
		assert.Equal(t, editionRev(b, "nb"), res[1].Current.Rev)
		assert.Equal(t, model.RunsEditText(b.TargetRuns("nb")), res[1].Current.Text)
		assert.Equal(t, change.OpNotApplied, res[2].Status)
		require.NotNil(t, res[2].BlockedBy)
		assert.Equal(t, 1, *res[2].BlockedBy)
		assert.Equal(t, before, shape(b.Source))
		assert.True(t, b.HasTarget("nb"))
	})
	t.Run("operations see the ones before them", func(t *testing.T) {
		b := guideBlock()
		rev := sourceRev(b)
		res := apply(t, b, person,
			replace("", rev, find("shop guide", "handbook")),
			replace("", rev, find("handbook", "manual")))
		requireApplied(t, res)
		assert.Equal(t, "Read the <1>manual</1> before you <2>order</2>.", shape(b.Source))
		assert.Equal(t, rev, res[1].Before, "every operation is checked against the start")
		assert.Equal(t, sourceRev(b), res[1].After)
	})
	t.Run("a missing if_match", func(t *testing.T) {
		b := guideBlock()
		err := requireRefused(t, apply(t, b, person, replace("", "", find("shop", "x")))[0], change.CodeInvalid)
		assert.Equal(t, "if_match", err.Field)
	})
	t.Run("an edition that does not exist", func(t *testing.T) {
		b := guideBlock()
		requireRefused(t, apply(t, b, person, replace("de", "*", find("x", "y")))[0], change.CodeNotFound)
	})
	t.Run("the same content is unchanged", func(t *testing.T) {
		b := guideBlock()
		b.Target("nb").Status = model.TargetStatusEstablished
		res := apply(t, b, person, setRuns("nb", editionRev(b, "nb"), b.TargetRuns("nb")))
		assert.Equal(t, change.OpUnchanged, res[0].Status)
		assert.Equal(t, model.TargetStatusEstablished, b.Target("nb").Status, "an unchanged edition keeps its decision")
	})
	t.Run("preview changes nothing", func(t *testing.T) {
		b := guideBlock()
		env := person
		env.Preview = true
		res := apply(t, b, env, replace("", sourceRev(b), find("shop guide", "handbook")))
		assert.Equal(t, change.OpPreviewed, res[0].Status)
		assert.NotEqual(t, res[0].Before, res[0].After)
		assert.Equal(t, "Read the <1>shop guide</1> before you <2>order</2>.", shape(b.Source))
	})
	t.Run("a blind write applies", func(t *testing.T) {
		b := guideBlock()
		requireApplied(t, apply(t, b, person, setText("nb", change.AnyRevision, "Les nå.")))
		assert.Equal(t, "Les nå.", b.TargetText("nb"))
	})
}

// The basis is recorded on every derived-edition write and refuses only under
// require_basis.
func TestApplyBlock_Basis(t *testing.T) {
	b := guideBlock()
	src := sourceRev(b)
	res := apply(t, b, person, setText("nb", editionRev(b, "nb"), "Les."))
	requireApplied(t, res)
	assert.Equal(t, src, res[0].Basis)

	moved := "r:0123456789abcdef"
	op := setText("nb", editionRev(b, "nb"), "Les nå.")
	op.Basis = moved
	res = apply(t, b, person, op)
	requireApplied(t, res)
	assert.Equal(t, moved, res[0].Basis, "a basis that moved lands without require_basis")

	strict := person
	strict.RequireBasis = true
	op = setText("nb", editionRev(b, "nb"), "Les igjen.")
	op.Basis = moved
	res = apply(t, b, strict, op)
	err := requireRefused(t, res[0], change.CodeStale)
	assert.Equal(t, "basis", err.Field)
	assert.Equal(t, src, res[0].Current.Rev)

	res = apply(t, b, strict, setText("nb", editionRev(b, "nb"), "Les igjen."))
	assert.Equal(t, "basis", requireRefused(t, res[0], change.CodeInvalid).Field)

	op = setText("", src, "Read.")
	op.Basis = src
	assert.Equal(t, "basis", requireRefused(t, apply(t, b, person, op)[0], change.CodeInvalid).Field)

	// A derived edition written after the authoritative edition changed in
	// the same set is made against the changed one, which is its basis.
	b = guideBlock()
	res = apply(t, b, person,
		replace("", sourceRev(b), find("shop guide", "handbook")),
		setText("nb", editionRev(b, "nb"), `Les <x id="1"/>håndboka<x id="/1"/> før du <x id="2"/>bestiller<x id="/2"/>.`))
	requireApplied(t, res)
	assert.Equal(t, res[0].After, res[1].Basis)
	assert.Equal(t, sourceRev(b), res[1].Basis)
}

// An edit to the authoritative edition names the derived editions it leaves
// on an older basis.
func TestApplyBlock_InvalidatesDerivedEditions(t *testing.T) {
	b := guideBlock()
	res := apply(t, b, agent, replace("", sourceRev(b), find("shop guide", "handbook")))
	requireApplied(t, res)
	assert.Equal(t, []change.Invalidation{{Edition: "nb", Reason: change.ReasonBasisMoved}}, res[0].Invalidates)

	res = apply(t, b, agent, replace("nb", editionRev(b, "nb"), find("Les", "Lees")))
	requireApplied(t, res)
	assert.Empty(t, res[0].Invalidates, "a derived edition invalidates nothing")
}

// An edition key reaches one edition however it is spelled (main-model P7).
func TestApplyBlock_CanonicalEditionKeys(t *testing.T) {
	b := guideBlock()
	op := setText("", model.AbsentRevision, "Hei.")
	op.At.Edition = model.EditionKey{Locale: "nb_NO"}
	requireApplied(t, apply(t, b, person, op))
	assert.Equal(t, "Hei.", b.TargetText("nb-NO"))
	assert.Nil(t, b.Targets[model.EditionKey{Locale: "nb_NO"}], "no edition under the spelling the sender used")

	op = setText("", model.AbsentRevision, "Hei igjen.")
	op.At.Edition = model.EditionKey{Locale: "NB-no"}
	requireRefused(t, apply(t, b, person, op)[0], change.CodeStale)
}

// Overlays on the edited edition follow the edit, on every edition (main-model
// P6), and overlays on other editions are untouched.
func TestApplyBlock_RebasesOverlaysOnTheEditedEdition(t *testing.T) {
	b := guideBlock()
	nb := model.Variant("nb")
	nbRuns := b.TargetRuns("nb")
	b.SetSegmentation(&nb, []model.Span{{ID: "s1", Range: model.RangeAnchor(nbRuns, 0, 3)}, {ID: "s2", Range: model.RangeAnchor(nbRuns, 4, 16)}})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "order", Range: model.RangeAnchor(b.Source, 31, 36)})

	requireApplied(t, apply(t, b, person, replace("nb", editionRev(b, "nb"), find("butikkguiden", "håndboka"))))
	seg := b.SegmentationFor(&nb)
	require.NotNil(t, seg)
	require.Len(t, seg.Spans, 2, "the segment holding the edit is resized, not dropped")
	assert.Equal(t, "Les", model.RunsText(seg.Spans[0].Range.ExtractRuns(b.TargetRuns("nb"))))
	assert.Equal(t, "håndboka", model.RunsText(seg.Spans[1].Range.ExtractRuns(b.TargetRuns("nb"))))
	_, ok := b.OverlaysInBounds(&nb, b.TargetRuns("nb"))
	assert.True(t, ok)

	requireApplied(t, apply(t, b, person, setText("nb", editionRev(b, "nb"), "Bestill nå.")))
	assert.Nil(t, b.SegmentationFor(&nb), "a rewrite across the segments drops the layer whole")

	requireApplied(t, apply(t, b, person, replace("", sourceRev(b), find("shop guide", "handbook"))))
	sp := b.OverlaySpan(model.OverlayTerm, "order")
	require.NotNil(t, sp, "a source span outside the edit follows it")
	assert.Equal(t, "order", model.RunsText(sp.Range.ExtractRuns(b.Source)))
}

func remove(edition, ifMatch string) change.Op {
	return change.Op{Kind: change.KindRemoveEdition, At: ref(edition), IfMatch: ifMatch, Body: &change.RemoveEdition{}}
}

func TestApplyBlock_RemoveEdition(t *testing.T) {
	b := guideBlock()
	nb := model.Variant("nb")
	b.SetSegmentation(&nb, []model.Span{{ID: "s1", Range: model.RangeAnchor(b.TargetRuns("nb"), 0, 3)}})
	res := apply(t, b, person, remove("nb", editionRev(b, "nb")))
	requireApplied(t, res)
	assert.Equal(t, model.AbsentRevision, res[0].After)
	assert.False(t, b.HasTarget("nb"))
	assert.Nil(t, b.SegmentationFor(&nb), "the edition's overlays go with it")

	assert.Equal(t, change.OpUnchanged, apply(t, b, person, remove("nb", "*"))[0].Status)
	requireRefused(t, apply(t, b, person, remove("nb", "r:0123456789abcdef"))[0], change.CodeStale)
	requireRefused(t, apply(t, b, person, remove("", sourceRev(b)))[0], change.CodeInvalid)
	requireRefused(t, apply(t, b, person, remove("de", model.AbsentRevision))[0], change.CodeInvalid)
}

func TestApplyBlock_AnnotateAndUnannotate(t *testing.T) {
	b := guideBlock()
	note := func(id string, anchor *model.Anchor, value string) change.Op {
		return change.Op{Kind: change.KindAnnotate, At: ref("nb"), Body: &change.Annotate{Type: "note", ID: id, Anchor: anchor, Value: json.RawMessage(value)}}
	}
	res := apply(t, b, person, note("", nil, `{"text":"Check the link"}`))
	requireApplied(t, res)
	assert.Equal(t, "note-1", res[0].ID)
	assert.Equal(t, res[0].Before, res[0].After, "an annotation is not content")

	nb := model.Variant("nb")
	var spans []model.Span
	for _, o := range b.Overlays {
		if o.Type == "note" && o.Variant != nil && *o.Variant == nb {
			spans = o.Spans
		}
	}
	require.Len(t, spans, 1)
	assert.Equal(t, model.BlockAnchor(), spans[0].Range)

	rng := model.SpanAnchor(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 4})
	assert.Equal(t, change.OpApplied, apply(t, b, person, note("note-1", &rng, `{"text":"Check the link"}`))[0].Status, "the same id is replaced")
	assert.Equal(t, change.OpUnchanged, apply(t, b, person, note("note-1", &rng, `{"text":"Check the link"}`))[0].Status)

	out := model.SpanAnchor(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 40})
	err := requireRefused(t, apply(t, b, person, note("n2", &out, `{}`))[0], change.CodeGuard)
	assert.Equal(t, change.SubcodeBadPosition, err.Subcode)

	entity := change.Op{Kind: change.KindAnnotate, At: ref(""), Body: &change.Annotate{Type: "entity", Anchor: &rng, Value: json.RawMessage(`{"Text":"guide","Type":"PRODUCT","Locale":"","DNT":false,"Source":"manual"}`)}}
	requireApplied(t, apply(t, b, person, entity))
	sp := b.OverlaySpan(model.OverlayEntity, "entity-1")
	require.NotNil(t, sp)
	_, typed := sp.Value.(*model.EntityAnnotation)
	assert.True(t, typed, "a registered payload decodes to its type when nothing is lost")

	unannotate := func(id string) change.Op {
		return change.Op{Kind: change.KindUnannotate, At: ref("nb"), Body: &change.Unannotate{Type: "note", ID: id}}
	}
	requireApplied(t, apply(t, b, person, unannotate("note-1")))
	for _, o := range b.Overlays {
		assert.NotEqual(t, model.OverlayType("note"), o.Type, "an empty overlay is removed")
	}
	requireRefused(t, apply(t, b, person, unannotate("note-1"))[0], change.CodeNotFound)
}

// A detector that anchors over the flattened text reads a plural through its
// other branch, so a span it finds there ends inside the plural, where no run
// position reaches. A tool's write (Report) leaves such a span out with a
// finding; a segmentation layer holding one is not written at all, because
// the writers read a layer as the whole segment list. Under Enforce the span
// is refused, and a span outside the content is refused either way.
func TestApplyBlock_SpansThatEndInsideAPlural(t *testing.T) {
	annotate := func(typ string, spans ...model.Span) change.Op {
		return change.Op{Kind: change.KindAnnotate, At: ref(""), Body: &change.Annotate{Type: typ, Spans: spans, Replace: typ == string(model.OverlaySegmentation)}}
	}
	runs := pluralRuns()
	text := model.RunsText(runs)                                            // "You have  items in your basket."
	inside := model.Span{ID: "e1", Range: model.RangeAnchor(runs, 10, 15)}  // "items"
	outside := model.Span{ID: "e2", Range: model.RangeAnchor(runs, 24, 30)} // "basket"
	require.Equal(t, "items", string([]rune(text)[10:15]))
	require.Equal(t, "basket", string([]rune(text)[24:30]))
	require.False(t, inside.Range.Resolves(runs))

	b := model.NewRunsBlock("p", pluralRuns())
	res := apply(t, b, tool, annotate("entity", inside, outside))
	requireApplied(t, res)
	require.Len(t, res[0].Findings, 1)
	assert.Contains(t, res[0].Findings[0].Message, `"e1"`)
	assert.Nil(t, b.OverlaySpan(model.OverlayEntity, "e1"))
	assert.NotNil(t, b.OverlaySpan(model.OverlayEntity, "e2"))

	b = model.NewRunsBlock("p", pluralRuns())
	b.SetSegmentation(nil, []model.Span{{ID: "s1", Range: model.RangeAnchor(runs, 0, len([]rune(text)))}})
	res = apply(t, b, tool, annotate(string(model.OverlaySegmentation),
		model.Span{ID: "s1", Range: model.RangeAnchor(runs, 0, 12)}, model.Span{ID: "s2", Range: model.RangeAnchor(runs, 12, 31)}))
	assert.Equal(t, change.OpUnchanged, res[0].Status)
	require.Len(t, res[0].Findings, 1)
	require.NotNil(t, b.SourceSegmentation())
	assert.Len(t, b.SourceSegmentation().Spans, 1, "the layer the block held is left as it was")

	b = model.NewRunsBlock("p", pluralRuns())
	err := requireRefused(t, apply(t, b, person, annotate("entity", inside))[0], change.CodeGuard)
	assert.Equal(t, change.SubcodeBadPosition, err.Subcode)
	beyond := model.Span{ID: "e3", Range: model.SpanAnchor(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 99})}
	requireRefused(t, apply(t, b, tool, annotate("entity", beyond))[0], change.CodeGuard)
}

// A code the reference does not hold and no native form spells is new: no
// format here can write it, so a sender's change is refused as unsupported. A
// tool's change lands with the code kept as sent and a finding, because a
// translation that names a placeholder the source lacks is for the checks to
// flag, not a reason to fail the flow.
func TestApplyBlock_NewCodes(t *testing.T) {
	invented := []model.Run{model.TextR("Les "), model.PhR(model.PlaceholderRun{ID: "9"})}

	b := guideBlock()
	err := requireRefused(t, apply(t, b, agent, setRuns("nb", editionRev(b, "nb"), invented))[0], change.CodeUnsupported)
	assert.Equal(t, "synthesize:ph", err.Capability)
	assert.NotContains(t, err.Message, "  ")

	res := apply(t, b, tool, setRuns("nb", "*", invented))
	requireApplied(t, res)
	assert.Equal(t, "Les {9}", shape(b.TargetRuns("nb")))
	var named bool
	for _, f := range res[0].Findings {
		named = named || strings.Contains(f.Message, `<x id="9/"/>`)
	}
	assert.True(t, named, "the finding names the code: %+v", res[0].Findings)

	b = guideBlock()
	requireRefused(t, apply(t, b, agent, setText("nb", editionRev(b, "nb"), `Les <x id="9/"/>`))[0], change.CodeGuard)
	res = apply(t, b, tool, setText("nb", "*", `Les <x id="9/"/>`))
	requireApplied(t, res)
	assert.NotEmpty(t, res[0].Findings)
}

// A subblock reference is a code like any other: a payload names it by id and
// takes it from the reference. A new one, or one that names another ref, is
// refused, so a payload cannot point a block at content it never held.
func TestApplyBlock_SubblockReferences(t *testing.T) {
	sub := model.Run{Sub: &model.SubRun{ID: "s1", Ref: "tu-alt"}}
	b := model.NewRunsBlock("p", []model.Run{model.TextR("See "), sub})
	b.SourceLocale = "en"
	strict := agent

	inject := []model.Run{model.TextR("Hello "), {Sub: &model.SubRun{ID: "9", Ref: "secret-block"}}}
	requireRefused(t, apply(t, b, strict, setRuns("", sourceRev(b), inject))[0], change.CodeUnsupported)

	retarget := []model.Run{model.TextR("See "), {Sub: &model.SubRun{ID: "s1", Ref: "secret-block"}}}
	requireRefused(t, apply(t, b, strict, setRuns("", sourceRev(b), retarget))[0], change.CodeUnsupported)
	assert.Equal(t, "tu-alt", b.Source[1].Sub.Ref)

	byID := []model.Run{model.TextR("Read "), {Sub: &model.SubRun{ID: "s1"}}}
	requireApplied(t, apply(t, b, strict, setRuns("", sourceRev(b), byID)))
	assert.Equal(t, "tu-alt", b.Source[1].Sub.Ref, "the reference gives the sub its ref")
}

func TestApplyBlock_OperationsAppliedElsewhere(t *testing.T) {
	b := guideBlock()
	for _, op := range []change.Op{
		{Kind: change.KindSetAttribute, At: ref(""), IfMatch: sourceRev(b), Body: &change.SetAttribute{Code: "1", Name: "href", Value: "x"}},
		{Kind: change.KindMark, At: ref(""), IfMatch: sourceRev(b), Body: &change.Mark{Range: change.Selection{Find: new("order")}, Type: "fmt:bold"}},
		{Kind: change.KindDeleteBlock, At: change.Ref{Doc: "d", Block: "p"}, Body: &change.DeleteBlock{IfMatch: map[string]string{"en": sourceRev(b)}}},
	} {
		err := requireRefused(t, apply(t, b, person, op)[0], change.CodeUnsupported)
		assert.Equal(t, string(op.Kind), err.Capability)
	}
	decide := change.Op{Kind: change.KindDecide, At: ref("nb"), IfMatch: editionRev(b, "nb"), Body: &change.Decide{Outcome: change.OutcomeEstablish}}
	requireRefused(t, apply(t, b, person, decide)[0], change.CodeInvalid)
	requireRefused(t, apply(t, b, person, change.Op{Kind: change.KindSetContent, At: ref(""), IfMatch: "*", Body: &change.ReplaceText{}})[0], change.CodeInvalid)
}

func TestApplyBlock_ProvenanceIsAToolsOwn(t *testing.T) {
	b := guideBlock()
	stamp := change.Op{Kind: change.KindProvenance, At: ref("nb"), Body: &change.Provenance{Status: "draft", Origin: model.Origin{Tool: "pseudo-translate"}}}
	requireRefused(t, apply(t, b, agent, stamp)[0], change.CodeNotPermitted)
	requireApplied(t, apply(t, b, tool, stamp))
	assert.Equal(t, model.TargetStatusDraft, b.Target("nb").Status)
	assert.Equal(t, "pseudo-translate", b.Target("nb").Origin.Tool)
	missing := stamp
	missing.At = ref("de")
	assert.Equal(t, change.OpUnchanged, apply(t, b, tool, missing)[0].Status, "an edition that does not exist has nothing to stamp")

	for _, at := range []string{"", "en"} {
		onSource := stamp
		onSource.At = ref(at)
		requireRefused(t, apply(t, b, tool, onSource)[0], change.CodeInvalid)
	}
	assert.Equal(t, model.SourceStatus(""), b.SourceStatus, "no target status reaches the source")
}
