package termbase

import (
	"fmt"

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
// from one lifecycle state to another. Imports (TBX, CSV, JSON, klftb) and
// AddConcept set state directly without passing through this policy — an
// import reproduces an externally authored termbase as-is rather than
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
