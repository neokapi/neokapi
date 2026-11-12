package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// TermsConceptResolver answers which concept in tb an entity's text names: the
// concept with a term that matches the text exactly or after normalization, in
// the entity's locale.
func TermsConceptResolver(tb terms.Terminology) memory.ConceptResolver {
	if tb == nil {
		return nil
	}
	return func(ctx context.Context, text string, locale model.LocaleID) (string, bool) {
		matches, err := tb.Lookup(ctx, text, terms.LookupOptions{
			SourceLocale: locale,
			MinScore:     1.0,
			MatchModes:   []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized},
		})
		if err != nil || len(matches) == 0 {
			return "", false
		}
		return matches[0].Concept.ID, true
	}
}

// entityTypes are the entity types a name can give.
var entityTypes = []model.EntityType{
	model.EntityPerson, model.EntityOrganization, model.EntityProduct, model.EntityLocation,
	model.EntityDate, model.EntityTime, model.EntityCurrency, model.EntityMeasurement, model.EntityOther,
}

// EntityTypeOf is the entity type a name gives: "product" and
// "entity:product" both name model.EntityProduct. False for a name that is not
// one of the entity types.
func EntityTypeOf(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, model.EntityPrefix) {
		name = model.EntityPrefix + name
	}
	return name, slices.Contains(entityTypes, model.EntityType(name))
}

// MemoryAnnotateOptions are the options of one kapi memory annotate run.
type MemoryAnnotateOptions struct {
	// Text is the text to mark as an entity.
	Text string
	// Type is the entity type (EntityTypeOf).
	Type string
	// CaseSensitive matches Text in its own case only.
	CaseSensitive bool
	// Entries names the entries to annotate. Empty is every entry whose text
	// contains Text.
	Entries []string
}

// MemoryAnnotateOutput is what kapi memory annotate changed.
type MemoryAnnotateOutput struct {
	Store string `json:"store"`
	memory.AnnotateResult
	// LinkedTerms reports that each new entity was looked up in the project's
	// terms store.
	LinkedTerms bool `json:"linked_terms"`
}

// FormatText writes the counts for a person.
func (o MemoryAnnotateOutput) FormatText(w io.Writer) error {
	if o.EntitiesAdded == 0 {
		_, err := fmt.Fprintln(w, "No entry holds the text, so nothing was marked.")
		return err
	}
	_, err := fmt.Fprintf(w, "Marked %s in %s.\n",
		pluralUnit(o.EntitiesAdded, "entity", "entities"), pluralUnit(o.EntriesUpdated, "entry", "entries"))
	return err
}

// RunMemoryAnnotate is kapi memory annotate: it marks opts.Text as an entity
// of opts.Type in the content memory's entries (memory.AnnotateEntries). In
// a project, each new entity is linked to the concept in the project's terms
// store whose term it matches.
func (a *App) RunMemoryAnnotate(cmd Command, opts MemoryAnnotateOptions) error {
	typ, ok := EntityTypeOf(opts.Type)
	if !ok {
		return fmt.Errorf("%q is not an entity type: use person, organization, product, location, date, time, currency, measurement or other", opts.Type)
	}
	if strings.TrimSpace(opts.Text) == "" {
		return errors.New("name the text to mark as an entity")
	}
	ctx := CmdContext(cmd)
	tm, label, release, err := a.OpenMemorySQLite(cmd)
	if err != nil {
		return err
	}
	defer release()

	ids := opts.Entries
	if len(ids) == 0 {
		if ids, err = entriesHolding(ctx, tm, opts.Text); err != nil {
			return err
		}
	}

	var resolve memory.ConceptResolver
	if sel, serr := a.ResolveMemoryStore(cmd); serr == nil && sel.InProject() {
		if w, perr := a.Projector(ctx, sel.Root); perr == nil {
			if tb := termsWriter(w); tb != nil {
				resolve = TermsConceptResolver(tb)
			}
		}
	}

	res, err := memory.AnnotateEntries(ctx, tm, ids,
		[]memory.EntityPattern{{Text: opts.Text, Type: typ, CaseSensitive: opts.CaseSensitive}}, resolve)
	if err != nil {
		return err
	}
	return output.Print(cmd, MemoryAnnotateOutput{Store: label, AnnotateResult: res, LinkedTerms: resolve != nil})
}

// entriesHolding lists the ids of the entries whose text contains text, a page
// at a time.
func entriesHolding(ctx context.Context, tm memory.Store, text string) ([]string, error) {
	const page = 500
	var ids []string
	for offset := 0; ; offset += page {
		entries, total, err := tm.SearchEntries(ctx, memory.SearchParams{Query: text, Offset: offset, Limit: page})
		if err != nil {
			return nil, fmt.Errorf("search content memory: %w", err)
		}
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		if len(entries) < page || offset+len(entries) >= total {
			return ids, nil
		}
	}
}
