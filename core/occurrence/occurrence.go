// Package occurrence answers where a term is actually used.
//
// "Which blocks use this term, in which collection and document, and in which
// language?" is the first cross-layer question the merged project store makes
// answerable ([C-03]). The terms and the blocks live in one file, so the
// question is a join rather than two lookups and a merge in application code —
// and it is answerable with no server and no account, which is the point: a
// connected project asks bowrain the same question over more dimensions, not a
// different question.
//
// # The materialized graph
//
// The live join above is one answer; the same occurrences also settle into the
// project's property graph. BuildGraph (graph.go) computes the uses_term /
// in_collection subgraph — block nodes keyed on content key (F-03), concept
// and collection nodes on their ids — which a host writer upserts into
// `graph_nodes` / `graph_edges`. UsesInCollections then answers "term → blocks →
// collection" by traversal, a walk the two-table join has no shape for. The
// identity vocabulary the graph keys on lives in graph.go, settled: a block is
// its content, a coordinate is edge data.
//
// [C-03]: https://neokapi.org/docs/contribute/architecture/context/c-03-context-store-and-graph
package occurrence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/terms"
)

// SourceLocale is the locale key an occurrence in a block's source text carries.
// It is empty because a block cache holds no opinion about the source language;
// the recipe does.
const SourceLocale = blockstore.SourceLocale

// Sources are the two subsystems the query joins. In a project both come off
// one `.kapi/work/store.db` handle; standalone stores and the browser build's
// in-memory backends satisfy the same interfaces.
type Sources struct {
	Terms  terms.Terminology
	Blocks blockstore.Store
}

// Query asks where a term is used.
type Query struct {
	// Subject is the term text or the concept id to look for. A concept is
	// tried first, then term text, so one positional argument serves both.
	Subject string
	// Locales restricts which text is searched: nil means every locale,
	// SourceLocale means the block's own text. An empty non-nil slice matches
	// nothing.
	Locales []string
	// Collection restricts the search to one collection.
	Collection string
	// Limit caps the occurrences returned. Zero means no cap.
	Limit int
}

// Occurrence is one use of one term in one block's text.
type Occurrence struct {
	// ConceptID is the concept the matched term belongs to.
	ConceptID string `json:"concept_id"`
	// Term is the term text as the terms store holds it, and TermLocale the
	// language it is a term in. They are not necessarily how the text spells
	// it: matching folds case.
	Term       string `json:"term"`
	TermLocale string `json:"term_locale"`

	// BlockHash is the block's durable content key — the identity an edge
	// would key on, so a re-parse that renumbers a document does not orphan
	// it.
	BlockHash string `json:"block_hash"`
	// BlockID is the block's structural name within its document.
	BlockID string `json:"block_id,omitempty"`
	// Document is the file the block was read from.
	Document string `json:"document,omitempty"`
	// Collection is the collection the block belongs to, where the store can
	// derive it.
	Collection string `json:"collection,omitempty"`
	// Locale is the language of the text the term was found in: SourceLocale
	// for the block's own text, otherwise the target locale.
	Locale string `json:"locale"`

	// Start and End are byte offsets of the match within that text.
	Start int `json:"start"`
	End   int `json:"end"`
	// Matched is the text exactly as it appears, which may differ from Term
	// in case and in the whitespace between words.
	Matched string `json:"matched"`
	// Snippet is the match in context, elided at both ends.
	Snippet string `json:"snippet"`
}

// Result is what a query found.
type Result struct {
	// Subject is the query as asked.
	Subject string `json:"subject"`
	// ConceptIDs are the concepts the subject resolved to, in id order.
	ConceptIDs []string `json:"concept_ids,omitempty"`
	// Terms are the term texts searched for, in the order they were tried.
	Terms []string `json:"terms,omitempty"`
	// Occurrences are the uses found, ordered by document, block, locale and
	// position.
	Occurrences []Occurrence `json:"occurrences"`
	// Total is len(Occurrences) before Limit was applied.
	Total int `json:"total"`
	// Truncated reports that Limit cut the list short.
	Truncated bool `json:"truncated,omitempty"`
	// Blocks is how many distinct blocks the occurrences fall in.
	Blocks int `json:"blocks"`
}

// ErrUnknownSubject reports that the subject names neither a concept nor a term
// in this store. It is distinct from finding no occurrences: a term nobody uses
// is a useful answer, a typo is not.
var ErrUnknownSubject = errors.New("occurrence: no such term or concept")

// Find returns every use of the query's term in the project's blocks.
//
// # Matching semantics
//
// Precisely, because a near-miss here reads as a bug in the terms store:
//
//   - The subject resolves to a set of terms. A concept id takes every term the
//     concept carries, in every language; a term text takes every term whose
//     text matches it after normalization (NFC, trimmed, case-folded, internal
//     whitespace collapsed), which may span several concepts.
//   - Matching is case-insensitive, always. Terms are recorded in one casing
//     and written in another; a term that only matched its own capitalization
//     would answer almost nothing.
//   - A multi-word term matches across any run of whitespace: "content memory"
//     is found in "content  memory" and across a line break, because flattened
//     text keeps whatever the source had.
//   - A match must sit on a word boundary. "AI" is not found inside "again".
//     The rule is applied per edge and only where the writing system has
//     boundaries to speak of: a term in Han, Kana, Thai, Lao, Khmer, Myanmar or
//     Tibetan is matched without it, since those scripts do not separate words.
//   - Matches of one term do not overlap: scanning is left to right, and each
//     match resumes after the last. Matches of DIFFERENT terms may overlap, and
//     both are reported — "memory" and "content memory" are two findings about
//     the same words, and collapsing them would hide one.
//   - Both the source text and every target text are searched, unless Locales
//     says otherwise. A term's own language does not restrict where it is
//     looked for: finding an English term still standing in a Norwegian target
//     is exactly the kind of thing worth finding.
//
// This is deliberately stricter than terms.LookupAll, which is a plain
// case-folding substring scan with no boundary rule. The two answer different
// questions: lookup asks which terms might apply to a text and is scored and
// over-inclusive by design, while this asks which blocks use a term and is read
// as a fact.
func Find(ctx context.Context, src Sources, q Query) (*Result, error) {
	if src.Terms == nil {
		return nil, errors.New("occurrence: no terms store")
	}
	subject := strings.TrimSpace(q.Subject)
	if subject == "" {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSubject, q.Subject)
	}

	targets, conceptIDs, err := resolveSubject(ctx, src.Terms, subject)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrUnknownSubject, subject)
	}

	res := &Result{Subject: subject, ConceptIDs: conceptIDs}
	for _, t := range targets {
		res.Terms = append(res.Terms, t.text)
	}
	if src.Blocks == nil {
		return res, nil
	}

	blocks := map[string]bool{}
	for _, t := range targets {
		hits, err := blockstore.SearchText(ctx, src.Blocks, t.text, blockstore.TextSearchOptions{
			Collection: q.Collection,
			Locales:    q.Locales,
		})
		if err != nil {
			return nil, fmt.Errorf("occurrence: search blocks for %q: %w", t.text, err)
		}
		matcher := newMatcher(t.text)
		for _, h := range hits {
			for _, span := range matcher.find(h.Text) {
				res.Occurrences = append(res.Occurrences, Occurrence{
					ConceptID:  t.conceptID,
					Term:       t.text,
					TermLocale: t.locale,
					BlockHash:  h.Hash,
					BlockID:    blockID(h.Block),
					Document:   documentOf(h.Block),
					Collection: h.Collection,
					Locale:     h.Locale,
					Start:      span.start,
					End:        span.end,
					Matched:    h.Text[span.start:span.end],
					Snippet:    snippet(h.Text, span.start, span.end),
				})
				blocks[h.Hash] = true
			}
		}
	}

	sortOccurrences(res.Occurrences)
	res.Total = len(res.Occurrences)
	res.Blocks = len(blocks)
	if q.Limit > 0 && len(res.Occurrences) > q.Limit {
		res.Occurrences = res.Occurrences[:q.Limit]
		res.Truncated = true
	}
	return res, nil
}

// target is one term to look for, and the concept it speaks for.
type target struct {
	conceptID string
	text      string
	locale    string
}

// resolveSubject turns the argument into the terms to search for. A concept id
// is tried first: ids are opaque and unlikely to collide with a term someone
// would type, and resolving the id first means `terms occurrences <id>` works
// straight off a `terms search` result.
func resolveSubject(ctx context.Context, tb terms.Terminology, subject string) ([]target, []string, error) {
	if c, ok, err := tb.GetConcept(ctx, subject); err != nil {
		return nil, nil, fmt.Errorf("occurrence: read concept %q: %w", subject, err)
	} else if ok {
		return termsOf(c), []string{c.ID}, nil
	}

	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("occurrence: list concepts: %w", err)
	}
	want := terms.NormalizeTerm(subject)
	var out []target
	seenConcept := map[string]bool{}
	var ids []string
	for _, c := range concepts {
		for _, t := range c.Terms {
			if terms.NormalizeTerm(t.Text) != want {
				continue
			}
			out = append(out, target{conceptID: c.ID, text: t.Text, locale: string(t.Locale)})
			if !seenConcept[c.ID] {
				seenConcept[c.ID] = true
				ids = append(ids, c.ID)
			}
		}
	}
	sort.Strings(ids)
	return dedupeTargets(out), ids, nil
}

func termsOf(c terms.Concept) []target {
	out := make([]target, 0, len(c.Terms))
	for _, t := range c.Terms {
		out = append(out, target{conceptID: c.ID, text: t.Text, locale: string(t.Locale)})
	}
	return dedupeTargets(out)
}

// dedupeTargets drops terms that would search for the same text on behalf of
// the same concept — a concept holding the same wording in two languages
// searches once, and reports the first locale.
func dedupeTargets(in []target) []target {
	seen := map[string]bool{}
	out := in[:0]
	for _, t := range in {
		if strings.TrimSpace(t.text) == "" {
			continue
		}
		key := t.conceptID + "\x00" + terms.NormalizeTerm(t.text)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// sortOccurrences puts the list in the order a reader wants it: by document,
// then block, then locale, then position — so re-running a query produces the
// same list, and a truncated list is truncated from a stable end.
func sortOccurrences(occ []Occurrence) {
	sort.SliceStable(occ, func(i, j int) bool {
		a, b := occ[i], occ[j]
		if a.Document != b.Document {
			return a.Document < b.Document
		}
		if a.BlockID != b.BlockID {
			return a.BlockID < b.BlockID
		}
		if a.BlockHash != b.BlockHash {
			return a.BlockHash < b.BlockHash
		}
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Term < b.Term
	})
}

func blockID(b *blockstore.Block) string {
	if b == nil {
		return ""
	}
	return b.ID
}

func documentOf(b *blockstore.Block) string {
	if b == nil {
		return ""
	}
	return b.Properties.File
}
