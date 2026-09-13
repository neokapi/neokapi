package host

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Surface forms for the terms store. `kapi terms expand` proposes the forms each
// term takes and writes them onto the terms; `kapi terms validate` reports the
// target terms that still lack them. Both read the same concepts.

// TermsFormsTarget is a set of concepts read for expansion or validation, and
// the place expanded forms are written back to.
type TermsFormsTarget struct {
	// Label names the target for the user: a bundle path or a store path.
	Label   string
	bundle  string
	file    *ktb.File
	store   terms.Terminology
	release func()
}

// OpenTermsFormsTarget resolves the concepts `kapi terms expand` and `kapi
// terms validate` work on. In order: a bundle named on the command line; a
// .terms.json named by --file; a store named by --name, --local or --file; the
// project's committed terms source; otherwise the store every `kapi terms`
// subcommand uses.
//
// The committed source comes before the project store because the store is a
// projection of it, rebuilt when the source changes, so forms written only to
// the store would not last.
func (a *App) OpenTermsFormsTarget(cmd Command, bundle string) (*TermsFormsTarget, error) {
	file, _ := cmd.Flags().GetString("file")
	if bundle == "" && ktb.IsBundlePath(file) {
		bundle = file
	}
	if bundle != "" {
		if !ktb.IsBundlePath(bundle) {
			return nil, fmt.Errorf("%s is not a terms bundle: expected a %s file", bundle, ktb.Ext)
		}
		return openBundleTarget(bundle)
	}

	name, _ := cmd.Flags().GetString("name")
	local, _ := cmd.Flags().GetBool("local")
	if a.TermsBackend == nil && name == "" && file == "" && !local {
		src, err := a.resolveProjectTermsSourcePath(cmd, project.GovernancePoint{})
		if err != nil {
			return nil, err
		}
		if src != "" {
			return openBundleTarget(src)
		}
	}

	tb, label, release, err := a.OpenTermsSQLite(cmd)
	if err != nil {
		return nil, err
	}
	return &TermsFormsTarget{Label: label, store: tb, release: release}, nil
}

func openBundleTarget(path string) (*TermsFormsTarget, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open terms bundle: %w", err)
	}
	file, err := loadKTBFile(path)
	if err != nil {
		return nil, err
	}
	return &TermsFormsTarget{Label: path, bundle: path, file: file}, nil
}

// Concepts returns the target's concepts. For a bundle they are the document's
// own, so changes to them are what Save writes.
func (t *TermsFormsTarget) Concepts(ctx context.Context) ([]terms.Concept, error) {
	if t.file != nil {
		return t.file.Concepts, nil
	}
	concepts, err := t.store.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	return concepts, nil
}

// Save writes back the concepts whose ids are in changed and reports whether
// anything was written. A bundle is rewritten whole, and only when its
// serialization moved; a store takes each changed concept.
func (t *TermsFormsTarget) Save(ctx context.Context, concepts []terms.Concept, changed map[string]bool) (bool, error) {
	if len(changed) == 0 {
		return false, nil
	}
	if t.file != nil {
		t.file.Concepts = concepts
		return writeKTB(t.bundle, t.file)
	}
	for _, c := range concepts {
		if !changed[c.ID] {
			continue
		}
		if err := t.store.AddConcept(ctx, c); err != nil {
			return false, fmt.Errorf("write concept %s: %w", c.ID, err)
		}
	}
	return true, nil
}

// Close releases the store, when the target holds one.
func (t *TermsFormsTarget) Close() {
	if t.release != nil {
		t.release()
	}
}

// TermsExpandOptions selects the terms `kapi terms expand` asks about.
type TermsExpandOptions struct {
	// SourceLocale is the language the source terms are written in.
	SourceLocale model.LocaleID
	// Locales restricts expansion to terms in these locales, where a bare
	// language matches every region of it. Empty selects the target terms:
	// every language except the source language.
	Locales []model.LocaleID
	// Overwrite asks again about terms that already declare forms, and replaces
	// what they declare.
	Overwrite bool
}

// FormsProposal is what expansion proposes for one term.
type FormsProposal struct {
	ConceptID string
	Locale    model.LocaleID
	Term      string
	// Forms are the proposals that survived every filter, in the model's order.
	Forms []string
	// Rejected are the proposals a filter dropped, each with its reason.
	Rejected []aitools.RejectedForm
}

// termsExpandBatch caps how many terms of one language go into one model call,
// so a large store is answered in several structured responses of a size a
// model completes reliably.
const termsExpandBatch = 40

type expansionAsk struct {
	conceptID string
	locale    model.LocaleID
	text      string
}

// ExpandableTerms counts the terms ProposeTermForms would ask about, so a caller
// can skip building a provider when there are none.
func ExpandableTerms(concepts []terms.Concept, opts TermsExpandOptions) int {
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
func expansionAsks(concepts []terms.Concept, opts TermsExpandOptions) ([]model.LocaleID, map[model.LocaleID][]expansionAsk) {
	var order []model.LocaleID
	byLocale := map[model.LocaleID][]expansionAsk{}
	for _, c := range concepts {
		if c.DoNotTranslate {
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

func selectedForExpansion(loc model.LocaleID, opts TermsExpandOptions) bool {
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

// ProposeTermForms asks p for the surface forms of each selected term, one
// language at a time, and returns what survives filtering. It changes no
// concept; ApplyFormsProposals does that.
//
// The forms pass aitools.ExpandTermForms's filters (a form opens with the
// term's first three characters, is no duplicate, and names a term that was
// asked about). One more filter applies here: a form spelled the same as another
// term in the same language is dropped, because that word would then be a use of
// two entries.
func ProposeTermForms(ctx context.Context, p aiprovider.LLMProvider, concepts []terms.Concept, opts TermsExpandOptions) ([]FormsProposal, error) {
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
	var out []FormsProposal
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
		for start := 0; start < len(texts); start += termsExpandBatch {
			batch := texts[start:min(start+termsExpandBatch, len(texts))]
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
			prop := FormsProposal{ConceptID: a.conceptID, Locale: loc, Term: a.text, Rejected: slices.Clone(e.Rejected)}
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

// ApplyFormsProposals writes each proposal's forms onto its term, and returns
// the ids of the concepts that changed and how many terms did. A term that
// already declares forms keeps them unless overwrite is set.
func ApplyFormsProposals(concepts []terms.Concept, proposals []FormsProposal, overwrite bool) (map[string]bool, int) {
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
