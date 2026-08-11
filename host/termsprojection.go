package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/terms"
)

// The terminology return leg: reviewed decisions taken away from this checkout
// come home into the committed terms source, so `git diff` stays the review
// surface for terminology the way it is for everything else.
//
// Two properties make the projection safe to run unattended.
//
// It is UPSERT-ONLY, at every level. A concept the decision-carrying set does
// not mention survives; a term it does not mention survives; nothing is ever
// removed. The obvious implementation — export the whole terms store over the
// authored file — is a whole-store snapshot, and with the store holding only
// what the last pull brought it would delete every concept the workspace has
// not adopted, silently. Removal is a decision an author lands deliberately.
//
// It is BYTE-STABLE. A projection with nothing to say writes nothing at all:
// the merged document is compared against the bytes on disk, and an identical
// serialization is not rewritten. A file that churned on its own each night
// would put a change under `.kapi/` with no decision behind it — and a change
// under `.kapi/` is exactly what the erasure gate reads as the backing for a
// derived artifact.

// TermsProjectionResult reports what a projection into the committed terms
// source amounted to. Written is false when the merge produced the bytes
// already on disk, which is the ordinary outcome of a run with no new
// decisions.
type TermsProjectionResult struct {
	// Added and Updated count the concepts the projection introduced and the
	// ones whose decision-bearing content it changed.
	Added   int `json:"added,omitempty"`
	Updated int `json:"updated,omitempty"`
	// Relations counts the governed edges the projection added.
	Relations int `json:"relations,omitempty"`
	// Considered counts the concepts that carried a reviewed decision — the
	// input to the merge, whether or not it changed anything.
	Considered int `json:"considered,omitempty"`
	// Written reports that the authored file's bytes moved.
	Written bool `json:"written,omitempty"`
	// Path is the project-relative committed source that was projected into.
	Path string `json:"path,omitempty"`
}

// Changed reports whether the projection put anything in the working tree.
func (r TermsProjectionResult) Changed() bool { return r.Written }

// ProjectReviewedConcepts merges the reviewed decisions among a workspace's
// concepts into the project's committed terms source, then recompiles the
// source into the project store so the two agree.
//
// A concept projects when it carries at least one GOVERNED term — forbidden or
// preferred (terms.IsGovernedStatus). Those statuses are the ones a platform
// refuses on its direct write path and admits only from a reviewed change-set,
// so a term resting at one is evidence a reviewer approved the wording. A
// concept whose terms are all ungoverned is ordinary workspace state — an
// editor's draft, a scan proposal — and stays out of git until a decision
// touches it.
//
// A relation projects when it is governed (a REPLACED_BY edge, likewise
// change-set-only) and both its endpoints are in the resulting document; an
// edge with a dangling endpoint would not compile.
//
// Both slices are the pull's snapshot of the workspace, handed in rather than
// re-fetched: the caller has just read them. recipePath names the project — a
// path rather than a Command, because every caller is a venue that already
// resolved its project and none of them is a cobra command.
func (a *App) ProjectReviewedConcepts(ctx context.Context, recipePath string, concepts []terms.Concept, relations []terms.ConceptRelation) (TermsProjectionResult, error) {
	var res TermsProjectionResult

	decided := reviewedConcepts(concepts)
	res.Considered = len(decided)
	if len(decided) == 0 {
		// Nothing was decided, so nothing is bound and nothing is created. A
		// workspace with no reviewed terminology must not cause a recipe edit
		// (the terms-source binding) or an empty bundle to appear in the tree.
		return res, nil
	}
	if recipePath == "" {
		return res, errors.New("project reviewed terminology: no kapi project")
	}
	root := filepath.Dir(recipePath)

	srcPath, err := a.ensureTermsSourceBinding(recipePath, root)
	if err != nil {
		return res, err
	}
	res.Path = relSlash(root, srcPath)

	file, err := loadKTBFile(srcPath)
	if err != nil {
		return res, err
	}

	merged, added, updated := mergeConcepts(file.Concepts, decided)
	file.Concepts = merged
	res.Added, res.Updated = added, updated
	file.Relations, res.Relations = mergeGovernedRelations(file.Relations, relations, merged)

	written, err := writeKTB(srcPath, file)
	if err != nil {
		return res, err
	}
	res.Written = written
	if !written {
		return res, nil
	}
	// The store is the source's projection, so the file that just moved is
	// compiled back through the one import path rather than left to disagree
	// with the store the pull wrote a moment ago.
	if err := a.compileTermsSource(ctx, root, srcPath); err != nil {
		return res, err
	}
	return res, nil
}

// reviewedConcepts keeps the concepts carrying at least one governed term, in a
// stable order so two runs over the same workspace merge in the same sequence.
func reviewedConcepts(concepts []terms.Concept) []terms.Concept {
	out := make([]terms.Concept, 0, len(concepts))
	for _, c := range concepts {
		if conceptCarriesDecision(c) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// conceptCarriesDecision reports whether any of a concept's terms rests at a
// governed status.
func conceptCarriesDecision(c terms.Concept) bool {
	for _, t := range c.Terms {
		if terms.IsGovernedStatus(t.Status) {
			return true
		}
	}
	return false
}

// mergeConcepts upserts every decided concept into the authored set, preserving
// the authored order and appending new concepts at the end (the marshal sorts
// by id, so the order here only has to be deterministic). It returns the merged
// set plus how many concepts were added and how many were updated.
func mergeConcepts(authored, decided []terms.Concept) (merged []terms.Concept, added, updated int) {
	merged = make([]terms.Concept, len(authored))
	copy(merged, authored)

	byID := make(map[string]int, len(merged))
	for i, c := range merged {
		byID[c.ID] = i
	}
	for _, d := range decided {
		i, ok := byID[d.ID]
		if !ok {
			byID[d.ID] = len(merged)
			merged = append(merged, adoptConcept(d))
			added++
			continue
		}
		next, changed := mergeConcept(merged[i], d)
		if changed {
			merged[i] = next
			updated++
		}
	}
	return merged, added, updated
}

// adoptConcept prepares a concept the authored file does not have yet. Only the
// source marker is filled in: a workspace concept carries no TermSource on the
// wire, and an authored bundle records terminology as such.
func adoptConcept(d terms.Concept) terms.Concept {
	if d.Source == "" {
		d.Source = terms.TermSourceTerminology
	}
	return d
}

// mergeConcept folds a decided concept into its authored counterpart, reporting
// whether anything decision-bearing moved.
//
// Every field the workspace does not carry is kept from the authored concept:
// the terms source is the older and richer record — it holds the term source
// marker and the creation stamp the wire has no field for — so taking the
// workspace's value for a field it left empty would erase authored content in
// the name of an update. Terms are upserted by (locale, text): a decided term
// replaces its authored twin and a new one is appended, but an authored term
// the workspace does not have SURVIVES. Removing a term is a decision, not a
// consequence of the workspace holding a smaller set.
func mergeConcept(authored, decided terms.Concept) (terms.Concept, bool) {
	next := authored
	if decided.Domain != "" {
		next.Domain = decided.Domain
	}
	if decided.Definition != "" {
		next.Definition = decided.Definition
	}
	if len(decided.Properties) > 0 {
		next.Properties = decided.Properties
	}
	if decided.ProjectID != "" {
		next.ProjectID = decided.ProjectID
	}
	next.Terms = mergeTerms(authored.Terms, decided.Terms)

	if conceptDecisionSignature(next) == conceptDecisionSignature(authored) {
		return authored, false
	}
	// The stamp moves only with the content, so an ordinary workspace edit that
	// changed nothing a reviewer decided cannot produce a timestamp-only diff.
	if !decided.UpdatedAt.IsZero() {
		next.UpdatedAt = decided.UpdatedAt
	}
	return next, true
}

// mergeTerms upserts the decided terms into the authored list, keyed by locale
// and case-folded text — the same identity the platform's governed transitions
// use, so a decision about "Hyperlink" lands on the authored "hyperlink".
func mergeTerms(authored, decided []terms.Term) []terms.Term {
	out := make([]terms.Term, len(authored))
	copy(out, authored)
	byKey := make(map[string]int, len(out))
	for i, t := range out {
		byKey[termKey(t)] = i
	}
	for _, d := range decided {
		if i, ok := byKey[termKey(d)]; ok {
			out[i] = d
			continue
		}
		byKey[termKey(d)] = len(out)
		out = append(out, d)
	}
	return out
}

func termKey(t terms.Term) string {
	return string(t.Locale) + "|" + strings.ToLower(t.Text)
}

// conceptDecisionSignature canonicalizes everything about a concept a reviewer
// decides — its metadata and its terms — so an unchanged decision compares
// equal whatever order the workspace returned its terms in.
func conceptDecisionSignature(c terms.Concept) string {
	parts := make([]string, 0, len(c.Terms)+1)
	props := make([]string, 0, len(c.Properties))
	for k, v := range c.Properties {
		props = append(props, k+"="+v)
	}
	sort.Strings(props)
	parts = append(parts, strings.Join([]string{
		c.ID, c.ProjectID, c.Domain, c.Definition, string(c.Source), strings.Join(props, ","),
	}, "\x1f"))
	terms := make([]string, 0, len(c.Terms))
	for _, t := range c.Terms {
		terms = append(terms, strings.Join([]string{
			string(t.Locale), t.Text, string(t.Status), t.Note, t.PartOfSpeech, t.Gender,
		}, "\x1f"))
	}
	sort.Strings(terms)
	parts = append(parts, terms...)
	return strings.Join(parts, "\x1e")
}

// mergeGovernedRelations upserts the governed edges among relations into the
// authored set, keeping only those whose endpoints both exist in merged. It
// returns the merged edge set and how many were added.
//
// A REPLACED_BY edge is the one relation a platform routes through review, so it
// is the only kind that projects; every other edge is ordinary graph state. An
// edge whose endpoints are not both in the document is dropped rather than
// written, because the terms store refuses a dangling relation and the file
// would stop compiling.
func mergeGovernedRelations(authored, pulled []terms.ConceptRelation, merged []terms.Concept) ([]terms.ConceptRelation, int) {
	present := make(map[string]bool, len(merged))
	for _, c := range merged {
		present[c.ID] = true
	}
	out := make([]terms.ConceptRelation, len(authored))
	copy(out, authored)
	byID := make(map[string]int, len(out))
	for i, r := range out {
		byID[r.ID] = i
	}

	candidates := make([]terms.ConceptRelation, 0, len(pulled))
	for _, r := range pulled {
		if r.ID == "" || r.RelationType != graph.LabelReplacedBy {
			continue
		}
		if !present[r.SourceID] || !present[r.TargetID] {
			continue
		}
		candidates = append(candidates, r)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	added := 0
	for _, r := range candidates {
		if i, ok := byID[r.ID]; ok {
			out[i] = r
			continue
		}
		byID[r.ID] = len(out)
		out = append(out, r)
		added++
	}
	return out, added
}

// FormatTermsProjection renders a projection result as the one line a sync
// prints, or the empty string when nothing moved.
func FormatTermsProjection(r TermsProjectionResult) string {
	if !r.Written {
		return ""
	}
	var parts []string
	if r.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", r.Added))
	}
	if r.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", r.Updated))
	}
	if r.Relations > 0 {
		parts = append(parts, fmt.Sprintf("%d relation(s)", r.Relations))
	}
	path := r.Path
	if path == "" {
		path = "the committed terms source"
	}
	return fmt.Sprintf("Projected reviewed terminology into %s: %s.", path, strings.Join(parts, ", "))
}
