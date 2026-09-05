package venue

import (
	"slices"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/terms"
)

// The freshness ref's component definitions.
//
// A component hash only means anything because both sides compute it the same
// way, so each definition lives here once — in the framework-only bowrain/core
// module, beside the block and context hashes — and the push client, the plugin
// and the server all fold with it. A second definition with a different byte
// layout would not fail loudly: it would make two correct sides disagree
// forever and report a governance conflict on every write.
//
// The context component has no function of its own here: it is ContextHashOf
// (context.go), which the push has negotiated on since the context content type
// existed. Naming it twice would be the second definition this file exists to
// prevent.

// DecisionsComponent folds a decision ledger into the ref's decisions
// component: one identity per (item, unit, variant), folded by ref.Fold.
//
// Order-independent by construction, because the fold sorts its keys. That
// matters here: the committed record arrives in shard order and the server's
// ledger arrives in SQL order, and the two must agree whenever they hold the
// same decisions.
func DecisionsComponent(decisions []UnitDecision) string {
	if len(decisions) == 0 {
		return ""
	}
	parts := make(map[string]string, len(decisions))
	for _, d := range decisions {
		if d.Unit == "" || d.Variant == "" {
			continue
		}
		parts[decisionKey(d)] = DecisionIdentity(d)
	}
	if len(parts) == 0 {
		return ""
	}
	return ref.Fold(parts)
}

// decisionKey is the ledger's own primary key, rendered for the fold.
func decisionKey(d UnitDecision) string {
	return d.ItemName + "\x00" + d.Unit + "\x00" + d.Variant
}

// DecisionIdentity hashes one decision over the fields that carry the decision
// itself.
//
// It covers exactly the fields SameDecision compares, which is exactly what a
// store treats as a change — so a record a store would skip as unchanged cannot
// move the decisions component, and a component that moved means a decision
// really did. Updated is excluded for that reason: it orders conflicting
// records for last-writer-wins, and a replay that only bumped it writes
// nothing, so counting it would report a change no reader could find.
func DecisionIdentity(d UnitDecision) string {
	return ref.Identity(decisionFields(d)...)
}

// SameDecision reports whether two records say the same thing about a unit —
// the idempotency test a store applies before writing, and the equality the
// decisions component is folded from.
func SameDecision(a, b UnitDecision) bool {
	return slices.Equal(decisionFields(a), decisionFields(b))
}

// decisionFields lists a decision's identity-bearing fields in a fixed order.
// One list serves both the equality test and the hash, so the two cannot drift.
//
// The governing fingerprint is appended only when the record carries one. A
// record written before the field existed then keeps the identity it always
// had, so upgrading either end moves no project's decisions component; a record
// that gains a fingerprint is a real change a store must write.
func decisionFields(d UnitDecision) []string {
	parked := "false"
	if d.Parked {
		parked = "true"
	}
	fields := []string{
		d.Status,
		d.TargetHash,
		d.ContentHash,
		d.ReviewState,
		d.DecidedBy,
		d.DecidedAt,
		d.Note,
		parked,
		d.Assignee,
	}
	if d.GoverningFingerprint != "" {
		fields = append(fields, d.GoverningFingerprint)
	}
	return fields
}

// TermsComponent folds a terminology set into the ref's terms component: one
// identity per concept and per relation, folded by ref.Fold.
//
// The concept identity covers the fields a concept diff is computed against —
// domain, definition, and each term's locale, text, status and grammatical
// metadata. Volatile bookkeeping (timestamps, server-minted properties) is
// excluded: a component that moved on a touch nobody made would send every
// project back to a full re-fetch for nothing.
func TermsComponent(concepts []terms.Concept, relations []terms.ConceptRelation) string {
	if len(concepts) == 0 && len(relations) == 0 {
		return ""
	}
	parts := make(map[string]string, len(concepts)+len(relations))
	for _, c := range concepts {
		parts["concept\x00"+c.ID] = conceptIdentity(c)
	}
	for _, r := range relations {
		parts["relation\x00"+r.ID] = relationIdentity(r)
	}
	return ref.Fold(parts)
}

// conceptIdentity hashes one concept over its governed surface.
func conceptIdentity(c terms.Concept) string {
	// A store returns a concept's terms in insertion order, which is not an
	// identity: the same concept re-read after an unrelated edit must hash the
	// same, so the term list is folded rather than concatenated in place.
	byTerm := make(map[string]string, len(c.Terms))
	for _, t := range c.Terms {
		key := string(t.Locale) + "\x00" + t.Text
		byTerm[key] = ref.Identity(string(t.Status), t.PartOfSpeech, t.Gender, t.Note)
	}
	return ref.Identity(c.Domain, c.Definition, ref.Fold(byTerm))
}

// relationIdentity hashes one typed relation over its endpoints, its label and
// its note.
func relationIdentity(r terms.ConceptRelation) string {
	return ref.Identity(r.SourceID, r.TargetID, r.RelationType, r.Note)
}
