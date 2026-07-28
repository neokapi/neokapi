package host

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// Context retrieval, by content. See AD-037.
//
// One question — *what do we know about this?* — asked once and answered from
// every store the scope holds. Callers do not name a store: which one holds the
// answer is an implementation detail, and making a caller choose forces it to
// know the answer's location before it can ask.
//
// This is the single implementation behind BOTH the `kapi context search` verb
// and the `context_search` MCP tool. The two surfaces are wrappers over this
// function, so they cannot drift — the fragmentation AD-037 records (six CLI
// retrieval verbs, three MCP tools, no rule for which got exposed) came from
// each surface growing its own retrieval path.

// ContextScope names how much of the graph a result set could have come from.
// A caller must be able to tell "this project holds no answer" from "this scope
// cannot hold one", so a narrow scope is reported rather than left implied.
type ContextScope string

const (
	// ScopeProject is a local kapi project: a terms store, a content memory and
	// a profile. No concept relations, no revisions, no market scoping.
	ScopeProject ContextScope = "project"
	// ScopeWorkspace is a Bowrain-connected workspace: the full concept graph.
	ScopeWorkspace ContextScope = "workspace"
)

// ContextSearchRequest is one question put to the retrieval surface.
type ContextSearchRequest struct {
	Query string
	// Locale narrows term results to a language when set; empty searches all.
	Locale model.LocaleID
	// Limit caps each group independently, not the total — groups are ranked
	// within themselves and never merged, so one shared budget would let a
	// productive group starve the others.
	Limit int
}

// DefaultContextSearchLimit caps each group when a caller names no limit.
const DefaultContextSearchLimit = 10

// ContextSearchResult is the answer, grouped by kind.
//
// Deliberately NOT one ranked list. A term match scores on lexical closeness to
// a concept; a memory match scores on segment similarity. The two numbers are
// not comparable, so blending them would impose an order that means nothing.
// Grouping keeps each ranking honest and lets the caller weigh the kinds.
type ContextSearchResult struct {
	Query string `json:"query"`
	// Scope says how much could have been searched, so an empty result is
	// readable: a project scope that finds nothing has not consulted a concept
	// graph, because it has none.
	Scope ContextScope `json:"scope"`
	// Terms are concepts whose terms or definition match — what the project
	// calls this, and whether it is discouraged.
	Terms []ContextTermHit `json:"terms,omitempty"`
	// Precedent is wording the project has already approved that matches the
	// query — "how have we said this before?".
	//
	// This is NOT translation recycling. Recycling is the loop's job and is
	// invisible by design (`up` pre-fills from content memory as the structural
	// cost control), so offering "search memory for prior translations" would
	// invite a caller to hand-crank work already done. Precedent is a different
	// question: it serves a caller AUTHORING source content who wants its own
	// writing to sound like the project.
	Precedent []ContextPrecedentHit `json:"precedent,omitempty"`
	// Notes carries scope-shaped caveats — e.g. that a store the query would
	// have consulted is not bound. Present so "nothing found" is never
	// ambiguous between "no answer" and "nowhere to look".
	Notes []string `json:"notes,omitempty"`
}

// FormatText renders the grouped answer for a human, implementing
// output.TextFormatter. It lives on the shared type rather than in the CLI so
// the text and JSON renderings of a context answer are defined once — the
// surfaces wrap this, they do not each decide what an answer looks like.
func (r *ContextSearchResult) FormatText(w io.Writer) error {
	if len(r.Terms) == 0 && len(r.Precedent) == 0 {
		fmt.Fprintf(w, "Nothing in this project's context matches %q.\n", r.Query)
	}

	if len(r.Terms) > 0 {
		fmt.Fprintln(w, "Terms")
		for _, t := range r.Terms {
			// Lead with the verdict: the caller is asking "may I use this
			// word?", so the answer belongs before the metadata.
			verdict := "ok"
			if t.Discouraged {
				verdict = "discouraged"
				if t.Replacement != "" {
					verdict += ` — say "` + t.Replacement + `"`
				}
			}
			fmt.Fprintf(w, "  %-24s %s", t.Term, verdict)
			if t.Status != "" {
				fmt.Fprintf(w, " (%s)", t.Status)
			}
			if t.ValidTo != "" {
				fmt.Fprintf(w, " until %s", t.ValidTo)
			}
			fmt.Fprintln(w)
			if t.Definition != "" {
				fmt.Fprintf(w, "  %-24s %s\n", "", t.Definition)
			}
		}
	}

	if len(r.Precedent) > 0 {
		fmt.Fprintln(w, "\nAlready approved wording")
		for _, p := range r.Precedent {
			fmt.Fprintf(w, "  %s\n", p.Text)
		}
	}

	// Notes last and always: they are what makes an empty or partial answer
	// readable rather than ambiguous.
	if len(r.Notes) > 0 {
		fmt.Fprintln(w)
		for _, n := range r.Notes {
			fmt.Fprintf(w, "  note: %s\n", n)
		}
	}
	return nil
}

// ContextTermHit is one concept the query matched.
type ContextTermHit struct {
	ConceptID  string `json:"concept_id"`
	Term       string `json:"term"`
	Locale     string `json:"locale,omitempty"`
	Status     string `json:"status,omitempty"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	// Replacement is what to say instead, when the matched term is discouraged.
	// Derived, not stored: a concept groups its terms per locale, so the
	// replacement for a deprecated word is the preferred word for the same
	// concept in the same language. A caller asking "may I use this?" needs the
	// answer and the alternative together, not a second lookup.
	Replacement string `json:"replacement,omitempty"`
	// Discouraged is the one-glance answer to "may I use this word?", derived
	// from Status so a caller does not have to know the status vocabulary.
	Discouraged bool `json:"discouraged"`
	// ValidFrom/ValidTo carry the term's own time scoping when it has any.
	// A rule that starts or stops on a date is what lets a context survive a
	// rename without being rewritten, so a caller must be able to see that the
	// answer it just got has an expiry — "recognise the old name until March"
	// is a different instruction from "never use it".
	ValidFrom string `json:"valid_from,omitempty"`
	ValidTo   string `json:"valid_to,omitempty"`
}

// ContextPrecedentHit is one piece of previously-approved wording.
type ContextPrecedentHit struct {
	EntryID string `json:"entry_id"`
	Text    string `json:"text"`
	Locale  string `json:"locale,omitempty"`
	Note    string `json:"note,omitempty"`
}

// ContextSearchSources are the stores a search consults. A nil member is a
// store the caller has not bound; the search proceeds over the rest and says so
// in Notes rather than failing, because a project with terms but no content
// memory is ordinary, not broken.
type ContextSearchSources struct {
	Terms  terms.Terminology
	Memory memory.Store
	Scope  ContextScope
}

// SearchContext answers one context question from every bound store.
//
// A store that errors degrades to a note rather than failing the whole call:
// half an answer plus a statement of what was unreachable is more useful to a
// caller mid-task than an error, and silently dropping the failure would make
// an incomplete answer look complete — the exact defect AD-037 records in the
// surface this replaces.
func SearchContext(ctx context.Context, src ContextSearchSources, req ContextSearchRequest) (*ContextSearchResult, error) {
	if req.Query == "" {
		return nil, fmt.Errorf("context search: empty query")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultContextSearchLimit
	}
	scope := src.Scope
	if scope == "" {
		scope = ScopeProject
	}

	res := &ContextSearchResult{Query: req.Query, Scope: scope}

	if src.Terms != nil {
		concepts, _, err := src.Terms.Search(ctx, req.Query, req.Locale, "", 0, limit)
		if err != nil {
			res.Notes = append(res.Notes, "terms store could not be searched: "+err.Error())
		} else {
			res.Terms = termHits(concepts, req.Locale)
		}
	} else {
		res.Notes = append(res.Notes, "no terms store is bound, so terminology was not consulted")
	}

	if src.Memory != nil {
		entries, _, err := src.Memory.SearchEntries(ctx, memory.SearchParams{
			Query: req.Query,
			Limit: limit,
		})
		if err != nil {
			res.Notes = append(res.Notes, "content memory could not be searched: "+err.Error())
		} else {
			res.Precedent = precedentHits(entries, req.Locale, limit)
		}
	} else {
		res.Notes = append(res.Notes, "no content memory is bound, so prior wording was not consulted")
	}

	if scope == ScopeProject {
		res.Notes = append(res.Notes,
			"project scope: concept relations, revisions and market scoping live in a connected workspace")
	}

	return res, nil
}

// termHits projects concepts into the flat, answer-shaped hits a caller wants:
// what this is called, whether it is discouraged, and what to say instead.
func termHits(concepts []terms.Concept, locale model.LocaleID) []ContextTermHit {
	var out []ContextTermHit
	for _, c := range concepts {
		for _, t := range c.Terms {
			if locale != "" && t.Locale != locale {
				continue
			}
			hit := ContextTermHit{
				ConceptID:   c.ID,
				Term:        t.Text,
				Locale:      string(t.Locale),
				Status:      string(t.Status),
				Definition:  c.Definition,
				Domain:      c.Domain,
				Discouraged: isDiscouraged(t.Status),
			}
			if hit.Discouraged {
				hit.Replacement = preferredTerm(c, t.Locale)
			}
			if v := t.Validity; v != nil {
				if v.ValidFrom != nil {
					hit.ValidFrom = v.ValidFrom.UTC().Format(time.RFC3339)
				}
				if v.ValidTo != nil {
					hit.ValidTo = v.ValidTo.UTC().Format(time.RFC3339)
				}
			}
			out = append(out, hit)
		}
	}
	return out
}

// preferredTerm finds the concept's preferred wording in one locale — what to
// say instead of a discouraged term. Returns "" when the concept records no
// preference, which is a real state: a term can be banned without an anointed
// replacement, and inventing one would be worse than saying nothing.
func preferredTerm(c terms.Concept, locale model.LocaleID) string {
	for _, t := range c.Terms {
		if t.Locale == locale && t.Status == model.TermPreferred {
			return t.Text
		}
	}
	return ""
}

// isDiscouraged answers "may I use this word?" from a term's status, so a
// caller does not need the status vocabulary to act on the result.
func isDiscouraged(s model.TermStatus) bool {
	switch s {
	case model.TermForbidden, model.TermDeprecated:
		return true
	default:
		return false
	}
}

// precedentHits projects memory entries into approved wording. Each entry is
// multilingual; the requested locale wins, otherwise the entry's source-language
// hint, so a caller authoring in one language is not handed another's text.
func precedentHits(entries []memory.Entry, locale model.LocaleID, limit int) []ContextPrecedentHit {
	var out []ContextPrecedentHit
	for _, e := range entries {
		if len(out) >= limit {
			break
		}
		pick := locale
		if pick == "" {
			pick = e.HintSrcLang
		}
		runs, ok := e.Variants[pick]
		if !ok {
			// No variant in the wanted language: this entry has nothing to say
			// to this caller, and offering another language's text as precedent
			// would be actively misleading.
			continue
		}
		out = append(out, ContextPrecedentHit{
			EntryID: e.ID,
			Text:    model.RunsText(runs),
			Locale:  string(pick),
			Note:    e.Note,
		})
	}
	return out
}
