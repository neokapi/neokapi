// Package expand proposes the surface forms each term takes in its own
// language (the plural, the definite form, a case ending) and writes them onto
// the terms. A declared form is how a check recognises a rendering that
// changes the word. `kapi terms expand` and the platform's term-forms job both
// run it.
package expand

import (
	"context"
	"fmt"
	"slices"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
)

// Options selects the terms Propose asks about.
type Options struct {
	// SourceLocale is the language the source terms are written in.
	SourceLocale model.LocaleID
	// Locales restricts expansion to terms in these locales, where a bare
	// language matches every region of it. Empty selects the target terms:
	// every language except the source language.
	Locales []model.LocaleID
	// Overwrite asks again about terms that already declare forms, and replaces
	// what they declare.
	Overwrite bool
	// Concepts restricts expansion to the concepts with these ids. Empty
	// selects every concept.
	Concepts []string
}

// Proposal is what expansion proposes for one term.
type Proposal struct {
	ConceptID string
	Locale    model.LocaleID
	Term      string
	// Forms are the proposals that survived every filter, in the model's order.
	Forms []string
	// Rejected are the proposals a filter dropped, each with its reason.
	Rejected []aitools.RejectedForm
}

// batchSize caps how many terms of one language go into one model call,
// so a large store is answered in several structured responses of a size a
// model completes reliably.
const batchSize = 40

type expansionAsk struct {
	conceptID string
	locale    model.LocaleID
	text      string
}

// Count counts the terms Propose would ask about, so a caller
// can skip building a provider when there are none.
func Count(concepts []terms.Concept, opts Options) int {
	_, asks := expansionAsks(concepts, opts)
	n := 0
	for _, a := range asks {
		n += len(a)
	}
	return n
}

// expansionAsks selects the terms to ask about, grouped by locale in first-seen
// order. A do-not-translate concept's terms are names that stay as written, so
// they are never selected.
func expansionAsks(concepts []terms.Concept, opts Options) ([]model.LocaleID, map[model.LocaleID][]expansionAsk) {
	var order []model.LocaleID
	byLocale := map[model.LocaleID][]expansionAsk{}
	for _, c := range concepts {
		if c.DoNotTranslate || (len(opts.Concepts) > 0 && !slices.Contains(opts.Concepts, c.ID)) {
			continue
		}
		for _, t := range c.Terms {
			if strings.TrimSpace(t.Text) == "" || !selectedForExpansion(t.Locale, opts) {
				continue
			}
			if len(t.Forms) > 0 && !opts.Overwrite {
				continue
			}
			loc := model.NormalizeLocale(t.Locale)
			if _, ok := byLocale[loc]; !ok {
				order = append(order, loc)
			}
			byLocale[loc] = append(byLocale[loc], expansionAsk{c.ID, loc, t.Text})
		}
	}
	return order, byLocale
}

func selectedForExpansion(loc model.LocaleID, opts Options) bool {
	if len(opts.Locales) == 0 {
		return terms.BaseLanguage(loc) != terms.BaseLanguage(opts.SourceLocale)
	}
	norm := model.NormalizeLocale(loc)
	for _, want := range opts.Locales {
		w := model.NormalizeLocale(want)
		if w == norm || (!strings.ContainsAny(string(w), "-_") && strings.EqualFold(string(w), terms.BaseLanguage(norm))) {
			return true
		}
	}
	return false
}

// Propose asks p for the surface forms of each selected term, one
// language at a time, and returns what survives filtering. It changes no
// concept; Apply does that.
//
// The forms pass aitools.ExpandTermForms's filters (a form opens with the
// term's first three characters, is no duplicate, and names a term that was
// asked about). One more filter applies here: a form spelled the same as another
// term in the same language is dropped, because that word would then be a use of
// two entries.
func Propose(ctx context.Context, p aiprovider.LLMProvider, concepts []terms.Concept, opts Options) ([]Proposal, error) {
	spellings := map[string]map[string]string{}
	for _, c := range concepts {
		for _, t := range c.Terms {
			lang, key := terms.BaseLanguage(t.Locale), strings.ToLower(strings.TrimSpace(t.Text))
			if lang == "" || key == "" {
				continue
			}
			if spellings[lang] == nil {
				spellings[lang] = map[string]string{}
			}
			if _, ok := spellings[lang][key]; !ok {
				spellings[lang][key] = t.Text
			}
		}
	}

	order, byLocale := expansionAsks(concepts, opts)
	var out []Proposal
	for _, loc := range order {
		asks := byLocale[loc]
		var texts []string
		asked := map[string]bool{}
		for _, a := range asks {
			if k := strings.ToLower(a.text); !asked[k] {
				asked[k] = true
				texts = append(texts, a.text)
			}
		}

		got := map[string]aitools.TermExpansion{}
		for start := 0; start < len(texts); start += batchSize {
			batch := texts[start:min(start+batchSize, len(texts))]
			expansions, err := aitools.ExpandTermForms(ctx, p, batch, string(loc))
			if err != nil {
				return nil, fmt.Errorf("expand %s terms: %w", loc, err)
			}
			for _, e := range expansions {
				got[strings.ToLower(e.Term)] = e
			}
		}

		lang := terms.BaseLanguage(loc)
		for _, a := range asks {
			e := got[strings.ToLower(a.text)]
			prop := Proposal{ConceptID: a.conceptID, Locale: loc, Term: a.text, Rejected: slices.Clone(e.Rejected)}
			for _, f := range e.Forms {
				if other, ok := spellings[lang][strings.ToLower(f)]; ok && !strings.EqualFold(other, a.text) {
					prop.Rejected = append(prop.Rejected, aitools.RejectedForm{Form: f, Reason: fmt.Sprintf("spelled the same as the term %q", other)})
					continue
				}
				prop.Forms = append(prop.Forms, f)
			}
			out = append(out, prop)
		}
	}
	return out, nil
}

// Apply writes each proposal's forms onto its term, and returns
// the ids of the concepts that changed and how many terms did. A term that
// already declares forms keeps them unless overwrite is set.
func Apply(concepts []terms.Concept, proposals []Proposal, overwrite bool) (map[string]bool, int) {
	type key struct {
		conceptID string
		locale    model.LocaleID
		text      string
	}
	want := map[key][]string{}
	for _, p := range proposals {
		if len(p.Forms) > 0 {
			want[key{p.ConceptID, model.NormalizeLocale(p.Locale), strings.ToLower(p.Term)}] = p.Forms
		}
	}

	changed := map[string]bool{}
	count := 0
	for i := range concepts {
		c := &concepts[i]
		for j := range c.Terms {
			t := &c.Terms[j]
			forms, ok := want[key{c.ID, model.NormalizeLocale(t.Locale), strings.ToLower(t.Text)}]
			if !ok || (len(t.Forms) > 0 && !overwrite) {
				continue
			}
			next := terms.NormalizeForms(t.Text, forms)
			if slices.Equal(next, t.Forms) {
				continue
			}
			t.Forms = next
			changed[c.ID] = true
			count++
		}
	}
	return changed, count
}
