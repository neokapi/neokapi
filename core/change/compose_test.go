package change_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// rangeEdit names text by run positions.
func rangeEdit(start, end model.RunPos, with string) change.TextEdit {
	return change.TextEdit{Selection: change.Selection{Range: &change.Span{Start: start, End: end}}, Text: with}
}

// Positions in a change set name the edition as the set found it: two
// operations that each change one word of a block, at offsets computed
// against the same read, both land where their sender meant, in either
// order and in either form.
func TestApplyBlock_PositionsNameTheEditionTheSetFound(t *testing.T) {
	tests := []struct {
		name string
		runs func() []model.Run
		ops  func(rev string) []change.Op
		want string
	}{
		{
			name: "offsets, the earlier word first",
			runs: func() []model.Run { return []model.Run{model.TextR("We utilize it and utilize them.")} },
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(3, 10, "use")), replace("", rev, span(18, 25, "use"))}
			},
			want: "We use it and use them.",
		},
		{
			name: "offsets, the later word first",
			runs: func() []model.Run { return []model.Run{model.TextR("We utilize it and utilize them.")} },
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(18, 25, "use")), replace("", rev, span(3, 10, "use"))}
			},
			want: "We use it and use them.",
		},
		{
			name: "run ranges across inline codes",
			runs: guideRuns,
			ops: func(rev string) []change.Op {
				return []change.Op{
					replace("", rev, rangeEdit(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 10}, "handbook")),
					replace("", rev, rangeEdit(model.RunPos{Run: 6}, model.RunPos{Run: 6, Offset: 5}, "buy")),
				}
			},
			want: "Read the <1>handbook</1> before you <2>buy</2>.",
		},
		{
			name: "an insertion and a replacement that touch",
			runs: func() []model.Run { return []model.Run{model.TextR("one two")} },
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(4, 7, "2")), replace("", rev, span(4, 4, "and "))}
			},
			want: "one and 2",
		},
		{
			name: "two insertions at one place land in the order sent",
			runs: func() []model.Run { return []model.Run{model.TextR("ab")} },
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(1, 1, "1")), replace("", rev, span(1, 1, "2"))}
			},
			want: "a12b",
		},
		{
			name: "a find reads the text the operations before it left",
			runs: func() []model.Run { return []model.Run{model.TextR("one two three")} },
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(0, 3, "uno")), replace("", rev, find("uno two", "1 2"))}
			},
			want: "1 2 three",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := model.NewRunsBlock("b", tc.runs())
			b.Name = "p"
			rev := sourceRev(b)
			requireApplied(t, apply(t, b, person, tc.ops(rev)...))
			assert.Equal(t, tc.want, shape(b.Source))
		})
	}
}

// A position inside text an earlier operation of the set changed has no place
// in the edition as it stands: the operation is refused as an overlap that
// names both operations, and nothing lands.
func TestApplyBlock_AnOverlapWithAnEarlierOperationIsRefused(t *testing.T) {
	tests := []struct {
		name  string
		ops   func(rev string) []change.Op
		field string
		names string
	}{
		{
			name: "two replacements of the same words",
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(3, 10, "use")), replace("", rev, span(3, 10, "employ"))}
			},
			field: "edits/0",
			names: "operation 1 names a position in text operation 0 changed",
		},
		{
			name: "a replacement across the end of another",
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(3, 10, "use")), replace("", rev, span(8, 13, "x"))}
			},
			field: "edits/0",
			names: "operation 1 names a position in text operation 0 changed",
		},
		{
			name: "an insertion inside replaced text",
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(3, 10, "use")), replace("", rev, span(5, 5, "x"))}
			},
			field: "edits/0",
			names: "operation 1 names a position in text operation 0 changed",
		},
		{
			name: "a replacement around an insertion",
			ops: func(rev string) []change.Op {
				return []change.Op{replace("", rev, span(5, 5, "x")), replace("", rev, span(3, 10, "use"))}
			},
			field: "edits/0",
			names: "operation 1 names a position in text operation 0 changed",
		},
		{
			name: "a position after a set_content of the whole edition",
			ops: func(rev string) []change.Op {
				return []change.Op{setText("", rev, "We use it."), replace("", rev, span(3, 10, "use"))}
			},
			field: "edits/0",
			names: "operation 1 names a position in text operation 0 replaced whole",
		},
		{
			name: "a mark over words an earlier operation changed",
			ops: func(rev string) []change.Op {
				start, end := 3, 10
				return []change.Op{
					replace("", rev, span(3, 10, "use")),
					{Kind: change.KindMark, At: ref(""), IfMatch: rev, Body: &change.Mark{Range: change.Selection{Start: &start, End: &end}, Type: "fmt:bold"}},
				}
			},
			field: "range",
			names: "operation 1 names a position in text operation 0 changed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := model.NewRunsBlock("b", []model.Run{model.TextR("We utilize it and utilize them.")})
			env := person
			env.Format = boldCaps
			res := apply(t, b, env, tc.ops(sourceRev(b))...)
			err := requireRefused(t, res[1], change.CodeGuard)
			assert.Equal(t, change.SubcodeOverlap, err.Subcode)
			assert.Equal(t, tc.field, err.Field)
			assert.Contains(t, err.Message, tc.names)
			assert.Equal(t, change.OpNotApplied, res[0].Status)
			assert.Equal(t, "We utilize it and utilize them.", b.SourceText(), "nothing lands")
		})
	}
}

// boldCaps declares the vocabulary type a mark may create.
var boldCaps = change.DeclaredCapabilities("okf_html", format.EditCapabilities{Synthesizes: []string{"fmt:bold"}})

// A mark after a replacement wraps the words its positions named in the
// edition as the set found it.
func TestApplyBlock_AMarkAfterAReplacementWrapsTheWordsItNamed(t *testing.T) {
	b := model.NewRunsBlock("b", []model.Run{model.TextR("We utilize it and utilize them.")})
	env := person
	env.Format = boldCaps
	rev := sourceRev(b)
	start, end := 18, 25
	requireApplied(t, apply(t, b, env,
		replace("", rev, span(3, 10, "use")),
		change.Op{Kind: change.KindMark, At: ref(""), IfMatch: rev, Body: &change.Mark{Range: change.Selection{Start: &start, End: &end}, Type: "fmt:bold"}}))
	assert.Equal(t, "We use it and <1>utilize</1> them.", shape(b.Source))
}

// A path names a plural by its place in the edition as the set found it: an
// earlier operation that empties the text before the plural moves the plural
// to another run, and a later edit of one of its forms still reaches it.
func TestApplyBlock_APathFollowsItsPluralThroughAnEarlierEdit(t *testing.T) {
	b := model.NewRunsBlock("b", pluralRuns())
	rev := sourceRev(b)
	one := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}
	other := model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOther}}
	res := apply(t, b, person,
		replace("", rev, span(0, 9, "")),
		replace("", rev, change.TextEdit{Selection: change.Selection{Path: one, Find: new("item")}, Text: "article"}),
		replace("", rev, change.TextEdit{Selection: change.Selection{Path: other, Start: new(0), End: new(6)}, Text: " articles"}))
	requireApplied(t, res)
	assert.Equal(t, "{count: one={n} article other={n} articles} in your basket.", shape(b.Source))
	require.Len(t, res[1].Resolved, 1)
	assert.Equal(t, 0, res[1].Resolved[0].Path[0].Index, "the result echoes the path as the edit applied")
}

// Two fixes of one block through the change service land together, and a
// refusal names the operations by their places in the change set.
func TestService_TwoFixesOfOneBlockCompose(t *testing.T) {
	h := newMemHome(map[string][]memBlock{
		"a": {textBlock("one", "We utilize it and utilize them."), textBlock("two", "Other")},
	})
	svc := newMemService(h)
	b := readBlock(t, svc, "a", "one")
	two := readBlock(t, svc, "a", "two")
	fix := func(start, end int, with string) change.Op {
		return change.Op{Kind: change.KindReplaceText, At: b.Ref, IfMatch: b.Rev, Body: &change.ReplaceText{Edits: []change.TextEdit{span(start, end, with)}}}
	}
	res, err := svc.Apply(t.Context(), change.Set{Ops: []change.Op{fix(18, 25, "use"), fix(3, 10, "use")}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Equal(t, "We use it and use them.", readBlock(t, svc, "a", "one").Text)

	b = readBlock(t, svc, "a", "one")
	res, err = svc.Apply(t.Context(), change.Set{Ops: []change.Op{
		fix(3, 6, "employ"),
		edit(two.Ref, two.Rev, "Other, edited"),
		fix(4, 9, "x"),
	}}, svcPerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[2].Error)
	assert.Equal(t, change.SubcodeOverlap, res.Ops[2].Error.Subcode)
	assert.Contains(t, res.Ops[2].Error.Message, "operation 2 names a position in text operation 0 changed")
	assert.Equal(t, "We use it and use them.", readBlock(t, svc, "a", "one").Text)
}

// Random edits that do not overlap, each an operation of its own and sent in
// any order with positions read from one revision, give what the same edits
// give applied one at a time, each against the edition as it then stands.
func TestApplyBlock_EditsComposeAsIfAppliedOneAtATime(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261003, 9))
	for iter := range 400 {
		runs, codes := randomRuns(rng)
		edits := randomEdits(rng, runs, codes)
		if len(edits) == 0 {
			continue
		}
		t.Run(fmt.Sprintf("case %d", iter), func(t *testing.T) {
			set := model.NewRunsBlock("b", runs())
			rev := sourceRev(set)
			ops := make([]change.Op, len(edits))
			for i, e := range edits {
				te := span(e.start, e.end, e.text)
				if rng.IntN(2) == 0 {
					te = rangeEdit(runPos(set.Source, e.start, true), runPos(set.Source, e.end, e.start == e.end), e.text)
				}
				ops[i] = replace("", rev, te)
			}
			rng.Shuffle(len(ops), func(i, j int) { ops[i], ops[j] = ops[j], ops[i] })
			requireApplied(t, apply(t, set, person, ops...))

			one := model.NewRunsBlock("b", runs())
			ordered := slices.Clone(edits)
			slices.SortFunc(ordered, func(a, b plainEdit) int {
				if a.start != b.start {
					return b.start - a.start
				}
				return b.end - a.end
			})
			for _, e := range ordered {
				requireApplied(t, apply(t, one, person, replace("", sourceRev(one), span(e.start, e.end, e.text))))
			}

			assert.Equal(t, shape(one.Source), shape(set.Source))
			assert.Equal(t, string(model.CanonicalRunsJSON(one.Source)), string(model.CanonicalRunsJSON(set.Source)))
			assert.Equal(t, applyPlain(model.SequenceText(runs()), edits), model.SequenceText(set.Source))
		})
	}
}

type plainEdit struct {
	start, end int
	text       string
}

// randomRuns returns a constructor of one random run sequence (text, with
// placeholders and paired codes, the text inside a pair sometimes marked
// do-not-translate), and the own-text offset of every code.
func randomRuns(rng *rand.Rand) (func() []model.Run, []int) {
	words := []string{"ab", "cd", "æøå", "x", "hello", "wörld", " ", "  ", "q"}
	type piece struct {
		text string
		nt   bool
		code model.Run
	}
	var pieces []piece
	var codes []int
	at, id := 0, 0
	text := func(nt bool) {
		s := ""
		for range 1 + rng.IntN(3) {
			s += words[rng.IntN(len(words))]
		}
		pieces = append(pieces, piece{text: s, nt: nt})
		at += utf8.RuneCountInString(s)
	}
	for range 1 + rng.IntN(5) {
		text(false)
		switch rng.IntN(3) {
		case 0:
			id++
			codes = append(codes, at)
			pieces = append(pieces, piece{code: model.PhR(model.PlaceholderRun{ID: fmt.Sprint(id), Type: "code:variable", Data: "{v}"})})
		case 1:
			id++
			codes = append(codes, at)
			pieces = append(pieces, piece{code: model.PcOpenR(model.PcOpenRun{ID: fmt.Sprint(id), Type: "fmt:bold", Data: "<b>"})})
			text(rng.IntN(2) == 0)
			codes = append(codes, at)
			pieces = append(pieces, piece{code: model.PcCloseR(model.PcCloseRun{ID: fmt.Sprint(id), Type: "fmt:bold", Data: "</b>"})})
		}
	}
	text(false)
	return func() []model.Run {
		out := make([]model.Run, 0, len(pieces))
		for _, p := range pieces {
			if p.text != "" {
				out = append(out, model.Run{Text: &model.TextRun{Text: p.text, NoTranslate: p.nt}})
				continue
			}
			c := p.code
			switch {
			case c.Ph != nil:
				ph := *c.Ph
				c = model.Run{Ph: &ph}
			case c.PcOpen != nil:
				o := *c.PcOpen
				c = model.Run{PcOpen: &o}
			case c.PcClose != nil:
				cl := *c.PcClose
				c = model.Run{PcClose: &cl}
			}
			out = append(out, c)
		}
		return out
	}, codes
}

// randomEdits returns edits of the own text of runs that do not overlap, hold
// no code strictly inside them (an edit that would delete one is refused), and
// insert at most once at any place.
func randomEdits(rng *rand.Rand, runs func() []model.Run, codes []int) []plainEdit {
	n := utf8.RuneCountInString(model.SequenceText(runs()))
	words := []string{"", "Z", "yy", "ü", "new text", "-"}
	var out []plainEdit
	for pos := 0; pos <= n; {
		start := pos + rng.IntN(4)
		if start > n {
			break
		}
		end := min(n, start+rng.IntN(5))
		for _, c := range codes {
			if c > start && c < end {
				end = c
				break
			}
		}
		out = append(out, plainEdit{start: start, end: end, text: words[rng.IntN(len(words))]})
		pos = end
		if end == start {
			pos++
		}
	}
	return out
}

// applyPlain applies edits to plain text.
func applyPlain(text string, edits []plainEdit) string {
	r := []rune(text)
	ordered := slices.Clone(edits)
	slices.SortFunc(ordered, func(a, b plainEdit) int {
		if a.start != b.start {
			return b.start - a.start
		}
		return b.end - a.end
	})
	for _, e := range ordered {
		r = slices.Concat(r[:e.start], []rune(e.text), r[e.end:])
	}
	return string(r)
}

// runPos is the run position of an own-text offset: inside the text run that
// holds it, or, at a boundary, the start of the run after the boundary when
// start is set and the end of the text run before it otherwise.
func runPos(runs []model.Run, offset int, start bool) model.RunPos {
	at := 0
	for i, r := range runs {
		if r.Text == nil {
			continue
		}
		n := utf8.RuneCountInString(r.Text.Text)
		if offset < at+n || (!start && offset == at+n) {
			if offset >= at {
				return model.RunPos{Run: i, Offset: offset - at}
			}
		}
		at += n
	}
	return model.RunPos{Run: len(runs)}
}
