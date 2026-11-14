package terms

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// knownTermStatuses is the term lifecycle vocabulary (model.Term* constants).
var knownTermStatuses = map[model.TermStatus]bool{
	model.TermProposed:   true,
	model.TermApproved:   true,
	model.TermPreferred:  true,
	model.TermAdmitted:   true,
	model.TermDeprecated: true,
	model.TermForbidden:  true,
}

// KnownTermStatus reports whether s is one of the model.Term* lifecycle statuses.
func KnownTermStatus(s model.TermStatus) bool {
	return knownTermStatuses[s]
}

// ValidateTransition validates a term status transition under the framework's
// transition policy (AD-021). The policy disallows no transition outright —
// history is the guard, not a trap — so the only rejections are unknown
// statuses on either side. A no-op transition (same → same) is valid.
//
// ValidateTransition applies to *transitions*: a term whose status changes
// from one lifecycle state to another. Imports (TBX, CSV, JSON, ktb) and
// AddConcept set state directly without passing through this policy — an
// import reproduces an externally authored terms as-is rather than
// re-deriving its history, so AddConcept only checks that each status is a
// known value (see validateConceptTermStatuses).
//
// Whether a valid transition additionally requires governance (a reviewed
// change-set on the platform) is a separate question answered by
// IsGovernedTransition.
func ValidateTransition(from, to model.TermStatus) error {
	if !KnownTermStatus(from) {
		return fmt.Errorf("unknown term status %q", from)
	}
	if !KnownTermStatus(to) {
		return fmt.Errorf("unknown term status %q", to)
	}
	return nil
}

// IsGovernedTransition reports whether a term status transition is governed —
// that is, whether a platform must route it through a reviewed change-set
// rather than applying it directly (AD-021). Governed transitions are any
// transition *to* forbidden or preferred (banning a term, changing the
// preferred term) and any transition *from* forbidden (a banned term cannot
// silently return to use). A no-op transition (same → same) is never
// governed. Callers must validate the transition with ValidateTransition
// first; unknown statuses are reported as not governed.
func IsGovernedTransition(from, to model.TermStatus) bool {
	if from == to {
		return false
	}
	if to == model.TermForbidden || to == model.TermPreferred {
		return true
	}
	return from == model.TermForbidden
}

// IsGovernedStatus reports whether a term status is one a platform can only
// reach through a reviewed change-set — forbidden (the term is banned) and
// preferred (the term is the one to use). Every transition into either is
// governed (IsGovernedTransition), so a term resting at one of them carries the
// evidence that a decision was reviewed, which is what a return leg back into
// git needs to distinguish a decision from ordinary workspace state.
func IsGovernedStatus(s model.TermStatus) bool {
	return s == model.TermForbidden || s == model.TermPreferred
}

// GovernedCreate reports whether creating a concept with these terms is a
// governed transition: a term created already forbidden or preferred.
func GovernedCreate(terms []Term) bool {
	for _, t := range terms {
		if IsGovernedStatus(t.Status) {
			return true
		}
	}
	return false
}

// GovernedUpdate reports whether replacing oldTerms with newTerms entails a
// governed status transition: an added or changed term moving to or from
// forbidden or preferred, or a forbidden term removed. Terms are matched by
// locale and lowercased text, the identity a change-set op names a term by.
func GovernedUpdate(oldTerms, newTerms []Term) bool {
	oldByKey := make(map[string]Term, len(oldTerms))
	for _, t := range oldTerms {
		oldByKey[TermIdentity(t)] = t
	}
	newByKey := make(map[string]Term, len(newTerms))
	for _, t := range newTerms {
		newByKey[TermIdentity(t)] = t
		from := model.TermStatus("")
		if prev, ok := oldByKey[TermIdentity(t)]; ok {
			from = prev.Status
		}
		if IsGovernedTransition(from, t.Status) {
			return true
		}
	}
	for k, prev := range oldByKey {
		if _, ok := newByKey[k]; !ok && prev.Status == model.TermForbidden {
			return true
		}
	}
	return false
}

// TermIdentity keys a term by locale and lowercased text.
func TermIdentity(t Term) string {
	return string(t.Locale) + "|" + strings.ToLower(t.Text)
}
