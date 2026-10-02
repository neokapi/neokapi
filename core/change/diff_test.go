package change_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// copyRuns deep-copies runs through their canonical JSON.
func copyRuns(t *testing.T, runs []model.Run) []model.Run {
	t.Helper()
	var out []model.Run
	require.NoError(t, json.Unmarshal(model.CanonicalRunsJSON(runs), &out))
	if out == nil {
		out = []model.Run{}
	}
	return out
}

// copyBlock copies a block's identity and editions, deeply enough that
// changing one copy leaves the other alone.
func copyBlock(t *testing.T, b *model.Block) *model.Block {
	t.Helper()
	c := &model.Block{ID: b.ID, Name: b.Name, Unit: b.Unit, Type: b.Type, Translatable: b.Translatable,
		SourceLocale: b.SourceLocale, SourceStatus: b.SourceStatus, Source: copyRuns(t, b.Source)}
	for k, tg := range b.Targets {
		if tg == nil {
			continue
		}
		c.SetTargetVariant(k, &model.Target{Runs: copyRuns(t, tg.Runs), Status: tg.Status, Origin: tg.Origin, Score: tg.Score})
	}
	return c
}

// editions renders every edition of a block as canonical JSON, by key.
func editions(b *model.Block) map[string]string {
	out := map[string]string{}
	for _, k := range b.Editions() {
		e, _ := b.Edition(k)
		key, _ := k.MarshalText()
		out[string(key)] = string(model.CanonicalRunsJSON(e.Runs))
	}
	return out
}

// mapText rewrites every text run of a run tree, branches included.
func mapText(runs []model.Run, fn func(string) string) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.Text != nil:
			out[i] = model.Run{Text: &model.TextRun{Text: fn(r.Text.Text), NoTranslate: r.Text.NoTranslate}}
		case r.Plural != nil:
			p := *r.Plural
			p.Forms = map[model.PluralForm][]model.Run{}
			for k, f := range r.Plural.Forms {
				p.Forms[k] = mapText(f, fn)
			}
			out[i] = model.Run{Plural: &p}
		case r.Select != nil:
			s := *r.Select
			s.Cases = map[string][]model.Run{}
			for k, c := range r.Select.Cases {
				s.Cases[k] = mapText(c, fn)
			}
			out[i] = model.Run{Select: &s}
		default:
			out[i] = r
		}
	}
	return out
}

// firstWordEdited changes the first word of the first non-empty text run.
func firstWordEdited(runs []model.Run) []model.Run {
	out := append([]model.Run(nil), runs...)
	for i, r := range out {
		if r.Text != nil && strings.TrimSpace(r.Text.Text) != "" {
			out[i] = model.Run{Text: &model.TextRun{Text: "Edited " + r.Text.Text, NoTranslate: r.Text.NoTranslate}}
			return out
		}
	}
	return append(out, model.TextR("Edited"))
}

// withoutFirstDeletableSpan drops the first paired code of a deletable type.
func withoutFirstDeletableSpan(runs []model.Run) []model.Run {
	for _, r := range runs {
		if r.PcOpen == nil || (r.PcOpen.Type != "fmt:bold" && r.PcOpen.Type != "fmt:italic" && r.PcOpen.Type != "fmt:code") {
			continue
		}
		id := r.PcOpen.ID
		var out []model.Run
		for _, x := range runs {
			if (x.PcOpen != nil && x.PcOpen.ID == id) || (x.PcClose != nil && x.PcClose.ID == id) {
				continue
			}
			out = append(out, x)
		}
		return out
	}
	return runs
}

// branchEdited changes the text of the first branch of the first plural or
// select.
func branchEdited(runs []model.Run) []model.Run {
	out := append([]model.Run(nil), runs...)
	for i, r := range out {
		if r.Plural != nil {
			p := *r.Plural
			p.Forms = maps.Clone(r.Plural.Forms)
			for _, k := range []model.PluralForm{model.PluralOne, model.PluralOther} {
				if f, ok := p.Forms[k]; ok {
					p.Forms[k] = mapText(f, func(s string) string { return s + " (changed)" })
					break
				}
			}
			out[i] = model.Run{Plural: &p}
			return out
		}
	}
	return out
}

func upper(s string) string { return strings.ToUpper(s) }

// variants are the whole-block changes a path can arrive with.
var variants = []struct {
	name   string
	change func(t *testing.T, b *model.Block)
}{
	{"the source's first word edited", func(t *testing.T, b *model.Block) { b.Source = firstWordEdited(b.Source) }},
	{"the source as one plain text", func(t *testing.T, b *model.Block) { b.Source = []model.Run{model.TextR("All new.")} }},
	{"a deletable span dropped from the source", func(t *testing.T, b *model.Block) { b.Source = withoutFirstDeletableSpan(b.Source) }},
	{"a plural branch edited", func(t *testing.T, b *model.Block) { b.Source = branchEdited(b.Source) }},
	{"a translation created from the source", func(t *testing.T, b *model.Block) {
		b.SetTargetRuns("de", mapText(copyRuns(t, b.Source), upper))
	}},
	{"a channel edition created", func(t *testing.T, b *model.Block) {
		b.SetTargetVariant(model.EditionKey{Locale: "en", Channel: "short"}, &model.Target{Runs: []model.Run{model.TextR("Short.")}})
	}},
	{"every translation edited", func(t *testing.T, b *model.Block) {
		for _, tg := range b.Targets {
			tg.Runs = mapText(tg.Runs, func(s string) string {
				return strings.Map(func(r rune) rune {
					if unicode.IsLetter(r) {
						return unicode.ToUpper(r)
					}
					return r
				}, s)
			})
		}
	}},
	{"every translation removed", func(t *testing.T, b *model.Block) { b.Targets = nil }},
	{"a translation edited and the source edited", func(t *testing.T, b *model.Block) {
		b.Source = firstWordEdited(b.Source)
		b.SetTargetRuns("de", []model.Run{model.TextR("Neu.")})
	}},
}

// readBlocks reads every block of a document through the registered reader.
func readBlocks(t *testing.T, id registry.FormatID, doc string, target model.LocaleID) []*model.Block {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader(id)
	require.NoError(t, err)
	raw := &model.RawDocument{URI: "fixture." + string(id), SourceLocale: model.LocaleEnglish, TargetLocale: target,
		Reader: io.NopCloser(bytes.NewReader([]byte(doc)))}
	require.NoError(t, reader.Open(context.Background(), raw))
	var out []*model.Block
	for res := range reader.Read(context.Background()) {
		require.NoError(t, res.Error)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Translatable {
			out = append(out, b)
		}
	}
	require.NoError(t, reader.Close())
	return out
}

// diffFixtures are blocks with inline codes, plurals, flags and translations:
// built by hand, and read through the formats that make them.
func diffFixtures(t *testing.T) map[string]*model.Block {
	t.Helper()
	out := map[string]*model.Block{"guide paragraph with a translation": guideBlock()}
	plural := model.NewRunsBlock("cart.items", pluralRuns())
	plural.SourceLocale = "en"
	out["ICU plural"] = plural
	span := model.NewRunsBlock("ci", codeSpanRuns())
	span.SourceLocale = "en"
	out["code span"] = span
	sel := model.NewRunsBlock("greet", []model.Run{model.SelectR(model.SelectRun{Pivot: "gender", Cases: map[string][]model.Run{
		"female": {model.TextR("She replied")}, "other": {model.TextR("They replied")},
	}})})
	sel.SourceLocale = "en"
	out["select"] = sel

	docs := []struct {
		name   string
		format registry.FormatID
		doc    string
		target model.LocaleID
	}{
		{"html", "html", `<html><body><p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>, &amp; <i>enjoy</i>.</p></body></html>`, ""},
		{"markdown", "markdown", "Run `kapi check` in [CI](https://ci.example) before you **ship**.\n", ""},
		{"po with plural", "po", "msgid \"\"\nmsgstr \"\"\n\"Language: fr\\n\"\n\"Plural-Forms: nplurals=2; plural=(n > 1);\\n\"\n\nmsgid \"One file\"\nmsgid_plural \"%d files\"\nmsgstr[0] \"Un fichier\"\nmsgstr[1] \"%d fichiers\"\n\nmsgid \"Save <b>now</b>\"\nmsgstr \"Enregistrer <b>maintenant</b>\"\n", "fr"},
	}
	for _, d := range docs {
		for i, b := range readBlocks(t, d.format, d.doc, d.target) {
			out[d.name+" block "+string(rune('a'+i))] = b
		}
	}
	return out
}

// Applying Diff(a, b) to a yields b, for every fixture and every kind of
// whole-block change.
func TestDiff_ApplyingTheDiffYieldsTheChangedBlock(t *testing.T) {
	for name, base := range diffFixtures(t) {
		for _, v := range variants {
			t.Run(name+"/"+v.name, func(t *testing.T) {
				before := copyBlock(t, base)
				after := copyBlock(t, base)
				v.change(t, after)

				ops := change.Diff(before, after)
				res := change.ApplyBlock(before, ops, tool)
				for _, r := range res {
					require.Contains(t, []change.OpStatus{change.OpApplied, change.OpUnchanged}, r.Status, "%s: %+v", r.Op, r.Error)
				}
				assert.Equal(t, editions(after), editions(before))
				assert.Empty(t, change.Diff(before, after), "nothing is left to change")
			})
		}
	}
}

// The operations Diff writes are a change set a transport can carry: they
// encode, decode strictly, and apply to the same result, whenever the change
// adds no code the block does not hold.
func TestDiff_OperationsTravel(t *testing.T) {
	for name, base := range diffFixtures(t) {
		for _, v := range variants {
			t.Run(name+"/"+v.name, func(t *testing.T) {
				before, after := copyBlock(t, base), copyBlock(t, base)
				v.change(t, after)
				ops := change.Diff(before, after)
				if len(ops) == 0 {
					return
				}
				for i := range ops {
					ops[i].At.Doc = "doc"
				}
				wire, err := json.Marshal(change.Set{Ops: ops})
				require.NoError(t, err)
				set, err := change.Decode(bytes.NewReader(wire))
				require.NoError(t, err, "%s", wire)
				res := change.ApplyBlock(before, set.Ops, tool)
				for _, r := range res {
					require.Contains(t, []change.OpStatus{change.OpApplied, change.OpUnchanged}, r.Status, "%s: %+v", r.Op, r.Error)
				}
				assert.Equal(t, editions(after), editions(before))
			})
		}
	}
}

// A changed text in a flat edition with the same codes is a replace_text, so
// overlays outside the change survive; any other change is a set_content.
func TestDiff_PrefersATextEdit(t *testing.T) {
	a := guideBlock()
	b := copyBlock(t, a)
	b.Source = mapText(b.Source, func(s string) string { return strings.ReplaceAll(s, "shop guide", "handbook") })
	ops := change.Diff(a, b)
	require.Len(t, ops, 1)
	assert.Equal(t, change.KindReplaceText, ops[0].Kind)
	assert.Equal(t, sourceRev(a), ops[0].IfMatch)

	g := guideRuns()
	b.Source = []model.Run{g[5], g[6], g[7], model.TextR(" first, then read "), g[1], g[2], g[3], model.TextR(".")}
	ops = change.Diff(a, b)
	require.Len(t, ops, 1)
	assert.Equal(t, change.KindSetContent, ops[0].Kind, "the codes moved, which no text edit says")
	codes := 0
	for _, r := range ops[0].Body.(*change.SetContent).Runs {
		if r.PcOpen != nil {
			codes++
			assert.Empty(t, r.PcOpen.Data, "a held code travels without its native form")
		}
	}
	assert.Equal(t, 2, codes)
}
