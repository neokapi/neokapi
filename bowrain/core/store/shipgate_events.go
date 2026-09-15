package store

import "context"

// The gates a locale scope's ship state is withheld on, as the quality gate
// events name them. Each is one condition DeriveShipState reads.
const (
	// ShipGateNameTranslated is coverage: every translatable block has a target.
	ShipGateNameTranslated = "translated"
	// ShipGateNameChecks is the ship gate's per-block verdict: no block fails a
	// rule-based check or violates the terminology that governs the locale.
	ShipGateNameChecks = "checks"
	// ShipGateNameTerms is the terminology result: where terms govern the locale,
	// every translated block has one.
	ShipGateNameTerms = "terms"
	// ShipGateNameStale is the decision basis: no block holds a decision on source
	// wording the project has since rewritten.
	ShipGateNameStale = "stale"
	// ShipGateNameRejected is the review verdict: no block holds wording a reviewer
	// turned down that the loop has not drafted again.
	ShipGateNameRejected = "rejected"
)

// A ShipGateResult is one gate's standing in a locale scope, as
// EvaluateShipState decided it.
type ShipGateResult struct {
	Gate string
	Met  bool
	// NotChecked marks an unmet gate whose check has no result, as opposed to
	// one whose check found a problem.
	NotChecked bool
	// Actual is the count the gate reads, and Required the count it needs:
	// translated blocks against total blocks for coverage, and blocks at fault
	// against zero for every other gate.
	Actual   int
	Required int
}

// A ShipGateFailure is one unmet gate of a locale scope, as it was announced.
type ShipGateFailure struct {
	ProjectID  string
	Stream     string
	Locale     string
	Gate       string
	NotChecked bool
	Actual     int
	Required   int
}

// ShipGateFailureStore records the gate failures that have been announced, so a
// derivation that repeats announces only what changed. Each method reports
// whether its call changed the record, and of two concurrent calls that would
// make the same change exactly one reports true.
type ShipGateFailureStore interface {
	// OpenShipGateFailure records f. It reports true when no failure was open
	// for the gate or the open one differed in NotChecked.
	OpenShipGateFailure(ctx context.Context, f ShipGateFailure) (bool, error)
	// CloseShipGateFailure removes the open failure for the gate, and reports
	// true when there was one.
	CloseShipGateFailure(ctx context.Context, projectID, stream, locale, gate string) (bool, error)
}

// ShipGateFailures finds the failure record behind a content store, looking
// through any decorator that names what it wraps, as ShipVerdicts does.
func ShipGateFailures(cs ContentStore) (ShipGateFailureStore, bool) {
	for cs != nil {
		if fs, ok := cs.(ShipGateFailureStore); ok {
			return fs, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
