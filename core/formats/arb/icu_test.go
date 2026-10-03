package arb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// icuValues are ICU messages in the shapes ARB catalogs hold them: Flutter's
// own layout, compact syntax with an offset, a select with a plural nested in
// each case, a selectordinal, a multi-line layout with quoting, text around a
// structure, and messages with no structure at all.
var icuValues = []string{
	"{count, plural, =0{No items} =1{1 item} other{{count} items}}",
	"You have {count, plural, one{# item} other{# items}} in your basket.",
	"{count,plural,offset:1 =0{Nobody} one{{name} alone} other{{name} and # others}}",
	"{gender, select, male{He has {count, plural, one{# item} other{# items}}} female{She has {count, plural, one{# item} other{# items}}} other{They have {count, plural, one{# item} other{# items}}}}",
	"{place, selectordinal, one{#st} two{#nd} few{#rd} other{#th}}",
	"{count, plural,\n  one {Don''t show # item}\n  other {Don't show # items '{'quoted'}'}\n}",
	"Hello, {name}!",
	"Use {a} and {a} with '{'braces'}'",
	"",
}

func TestICUMessageWritesBackAsRead(t *testing.T) {
	for _, v := range icuValues {
		t.Run(v, func(t *testing.T) {
			runs := runsFromValue(v)
			original := ""
			if len(readMessage(v).shapes) > 0 {
				original = v
			}
			assert.Equal(t, v, valueFromRuns(runs, original))
		})
	}
}

// Without the value it was read from, a message is written from its runs
// alone, and a structure in Flutter's layout; what Flutter's tools wrote
// comes back unchanged.
func TestICUMessageWritesFromRunsAlone(t *testing.T) {
	for _, v := range []string{
		"{count, plural, =0{No items} =1{1 item} other{{count} items}}",
		"You have {count, plural, one{# item} other{# items}} in your basket.",
		"{gender, select, female{She} male{He} other{They}}",
		"Hello, {name}!",
	} {
		assert.Equal(t, v, valueFromRuns(runsFromValue(v), ""), v)
	}
}

func TestICUMessageReadsBranches(t *testing.T) {
	runs := runsFromValue("You have {count, plural, one{# item from {name}} other{# items from {name}}} for {name}.")
	require.Len(t, runs, 5)
	assert.Equal(t, "You have ", runs[0].Text.Text)
	p := runs[1].Plural
	require.NotNil(t, p)
	assert.Equal(t, "count", p.Pivot)
	one, other := p.Forms[model.PluralOne], p.Forms[model.PluralOther]
	require.Len(t, one, 3)
	require.Len(t, other, 3)
	// # and {name} are placeholders, each with one id across the branches.
	assert.Equal(t, "#", one[0].Ph.Data)
	assert.Equal(t, one[0].Ph.ID, other[0].Ph.ID)
	assert.Equal(t, "{name}", one[2].Ph.Data)
	assert.Equal(t, one[2].Ph.ID, other[2].Ph.ID)
	assert.NotEqual(t, one[0].Ph.ID, one[2].Ph.ID)
	assert.Equal(t, " item from ", one[1].Text.Text)
	// The top-level {name} has an id of its own.
	assert.Equal(t, " for ", runs[2].Text.Text)
	assert.Equal(t, "{name}", runs[3].Ph.Data)
	assert.NotEqual(t, one[2].Ph.ID, runs[3].Ph.ID)
	assert.Equal(t, "You have # items from {name} for {name}.", model.RenderRunsWithData(runs))
}

// A message with no structure reads exactly as it always has: each argument
// a placeholder with an id of its own, quotes kept in the text.
func TestICUMessageWithoutStructure(t *testing.T) {
	runs := runsFromValue("Use {a} and {a} with '{'braces'}'")
	require.Len(t, runs, 5)
	assert.Equal(t, "Use ", runs[0].Text.Text)
	assert.Equal(t, model.PlaceholderRun{ID: "p1", Type: "icu", Data: "{a}", Equiv: "{a}", Disp: "{a}"}, *runs[1].Ph)
	assert.Equal(t, "p2", runs[3].Ph.ID)
	assert.Equal(t, " with '{'braces'}'", runs[4].Text.Text)
	assert.Equal(t, []model.Run{{Text: &model.TextRun{Text: ""}}}, runsFromValue(""))
}

// A picker the run model cannot hold as written stays one placeholder.
func TestICUMessageKeepsUnreadablePickersWhole(t *testing.T) {
	for _, v := range []string{
		"{count, plural, single{x} other{y}}",
		"{count, plural, one{x} one{y} other{z}}",
		"{count, plural, one{x} other{y}",
		"{price, number, ::currency/EUR}",
	} {
		runs := runsFromValue(v)
		for _, r := range runs {
			assert.Nil(t, r.Plural, v)
			assert.Nil(t, r.Select, v)
		}
		assert.Equal(t, v, valueFromRuns(runs, ""), v)
	}
}

// setBranch replaces the branch a key path names in a copy of runs: the run
// index of a structure, then a branch key, repeated.
func setBranch(t *testing.T, runs []model.Run, branch []model.Run, path ...any) []model.Run {
	t.Helper()
	out := append([]model.Run(nil), runs...)
	i := path[0].(int)
	key := path[1].(string)
	r := out[i]
	switch {
	case r.Plural != nil:
		p := *r.Plural
		p.Forms = map[model.PluralForm][]model.Run{}
		for k, v := range r.Plural.Forms {
			p.Forms[k] = v
		}
		if len(path) > 2 {
			p.Forms[model.PluralForm(key)] = setBranch(t, p.Forms[model.PluralForm(key)], branch, path[2:]...)
		} else if branch == nil {
			delete(p.Forms, model.PluralForm(key))
		} else {
			p.Forms[model.PluralForm(key)] = branch
		}
		out[i] = model.Run{Plural: &p}
	case r.Select != nil:
		s := *r.Select
		s.Cases = map[string][]model.Run{}
		for k, v := range r.Select.Cases {
			s.Cases[k] = v
		}
		if len(path) > 2 {
			s.Cases[key] = setBranch(t, s.Cases[key], branch, path[2:]...)
		} else if branch == nil {
			delete(s.Cases, key)
		} else {
			s.Cases[key] = branch
		}
		out[i] = model.Run{Select: &s}
	default:
		t.Fatalf("run %d is no structure", i)
	}
	return out
}

func text(s string) []model.Run { return []model.Run{model.TextR(s)} }

// An edit to one branch changes that branch's bytes and nothing else: the
// syntax, the whitespace and every other branch are written as read.
func TestICUMessageBranchEdit(t *testing.T) {
	// hash is the first placeholder of the branch path names: the #.
	hash := func(runs []model.Run, path ...any) model.Run {
		t.Helper()
		seq, ok := model.ResolveRunPath(runs, pathOf(path...))
		require.True(t, ok)
		for _, r := range seq {
			if r.Ph != nil {
				return r
			}
		}
		t.Fatalf("no placeholder at %v", path)
		return model.Run{}
	}
	cases := []struct {
		name, value string
		edit        func(runs []model.Run) []model.Run
		want        string
	}{
		{
			name:  "explicit value",
			value: "{count, plural, =0{No items} =1{1 item} other{{count} items}}",
			edit:  func(r []model.Run) []model.Run { return setBranch(t, r, text("One item"), 0, "=1") },
			want:  "{count, plural, =0{No items} =1{One item} other{{count} items}}",
		},
		{
			name:  "category with the number",
			value: "You have {count, plural, one{# item} other{# items}} in your basket.",
			edit: func(r []model.Run) []model.Run {
				return setBranch(t, r, []model.Run{hash(r, 1, "one"), model.TextR(" article")}, 1, "one")
			},
			want: "You have {count, plural, one{# article} other{# items}} in your basket.",
		},
		{
			name:  "compact syntax with an offset",
			value: "{count,plural,offset:1 =0{Nobody} one{{name} alone} other{{name} and # others}}",
			edit:  func(r []model.Run) []model.Run { return setBranch(t, r, text("No one"), 0, "=0") },
			want:  "{count,plural,offset:1 =0{No one} one{{name} alone} other{{name} and # others}}",
		},
		{
			name:  "a plural inside a select",
			value: "{gender, select, male{He has {count, plural, one{# item} other{# items}}} female{She has {count, plural, one{# item} other{# items}}} other{They have {count, plural, one{# item} other{# items}}}}",
			edit: func(r []model.Run) []model.Run {
				return setBranch(t, r, []model.Run{hash(r, 0, "female", 1, "one"), model.TextR(" thing")}, 0, "female", 1, "one")
			},
			want: "{gender, select, male{He has {count, plural, one{# item} other{# items}}} female{She has {count, plural, one{# thing} other{# items}}} other{They have {count, plural, one{# item} other{# items}}}}",
		},
		{
			name:  "selectordinal",
			value: "{place, selectordinal, one{#st} two{#nd} few{#rd} other{#th}}",
			edit: func(r []model.Run) []model.Run {
				return setBranch(t, r, []model.Run{hash(r, 0, "few"), model.TextR("rd place")}, 0, "few")
			},
			want: "{place, selectordinal, one{#st} two{#nd} few{#rd place} other{#th}}",
		},
		{
			name:  "multi-line layout with quoting",
			value: "{count, plural,\n  one {Don''t show # item}\n  other {Don't show # items '{'quoted'}'}\n}",
			edit: func(r []model.Run) []model.Run {
				return setBranch(t, r, []model.Run{model.TextR("Hide "), hash(r, 0, "one"), model.TextR(" item")}, 0, "one")
			},
			want: "{count, plural,\n  one {Hide # item}\n  other {Don't show # items '{'quoted'}'}\n}",
		},
		{
			name:  "a branch removed",
			value: "{count, plural, =0{No items} =1{1 item} other{{count} items}}",
			edit:  func(r []model.Run) []model.Run { return setBranch(t, r, nil, 0, "=0") },
			want:  "{count, plural, =1{1 item} other{{count} items}}",
		},
		{
			name:  "the first branch removed",
			value: "{count, plural,\n  one {# item}\n  other {# items}\n}",
			edit:  func(r []model.Run) []model.Run { return setBranch(t, r, nil, 0, "one") },
			want:  "{count, plural,\n  other {# items}\n}",
		},
		{
			name:  "a branch added",
			value: "{count, plural, one{# item} other{# items}}",
			edit:  func(r []model.Run) []model.Run { return setBranch(t, r, text("No items"), 0, "zero") },
			want:  "{count, plural, one{# item} other{# items} zero{No items}}",
		},
		{
			name:  "text around the structure",
			value: "You have {count, plural, one{# item} other{# items}} in your basket.",
			edit: func(r []model.Run) []model.Run {
				out := append([]model.Run(nil), r...)
				out[2] = model.TextR(" in your cart.")
				return out
			},
			want: "You have {count, plural, one{# item} other{# items}} in your cart.",
		},
		{
			name:  "a structure over another argument",
			value: "{count, plural, one{# item} other{# items}}",
			edit: func(r []model.Run) []model.Run {
				return []model.Run{{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
					model.PluralOther: text("many"), model.PluralOne: text("one"), "=0": text("none"),
				}}}}
			},
			want: "{n, plural, =0{none} one{one} other{many}}",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs := tc.edit(runsFromValue(tc.value))
			got := valueFromRuns(runs, tc.value)
			assert.Equal(t, tc.want, got)
			block := &model.Block{ID: "tu1", Name: "msg"}
			assert.NoError(t, checkMessage(block, runs, got))
		})
	}
}

// A translation is written in the shape of the message it was made from.
func TestICUMessageTranslationTakesTheReadShape(t *testing.T) {
	src := "{count,plural, =0{No items} =1{1 item} other{{count} items}}"
	runs := runsFromValue(src)
	count := runs[0].Plural.Forms[model.PluralOther][0]
	fr := []model.Run{{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		"=0":              text("Aucun article"),
		"=1":              text("1 article"),
		model.PluralOther: {count, model.TextR(" articles")},
		model.PluralMany:  {count, model.TextR(" d'articles")},
	}}}}
	assert.Equal(t, "{count,plural, =0{Aucun article} =1{1 article} other{{count} articles} many{{count} d'articles}}",
		valueFromRuns(fr, src))
}

func pathOf(steps ...any) model.RunPath {
	var p model.RunPath
	for i := 0; i < len(steps); i += 2 {
		p = append(p, model.RunPathStep{Kind: model.StepIndex, Index: steps[i].(int)})
		if i+1 < len(steps) {
			key := steps[i+1].(string)
			if pluralForm(key) {
				p = append(p, model.RunPathStep{Kind: model.StepPlural, PluralForm: model.PluralForm(key)})
			} else {
				p = append(p, model.RunPathStep{Kind: model.StepSelect, SelectValue: key})
			}
		}
	}
	return p
}

// Text that would end a branch early is refused rather than written.
func TestICUMessageRefusesTextThatBreaksTheSyntax(t *testing.T) {
	src := "{count, plural, one{# item} other{# items}}"
	runs := setBranch(t, runsFromValue(src), text("an {open item"), 0, "one")
	value := valueFromRuns(runs, src)
	err := checkMessage(&model.Block{ID: "tu1", Name: "itemCount"}, runs, value)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errInvalidMessage))
	assert.Contains(t, err.Error(), "itemCount")

	// A message with no structure is written as it reads, as it always was.
	assert.NoError(t, checkMessage(&model.Block{ID: "tu2"}, text("an {open item"), "an {open item"))
}
