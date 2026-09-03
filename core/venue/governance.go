package venue

import "github.com/neokapi/neokapi/core/model"

// The shape of what a venue's review governance did with a push, in transit.
//
// A push moves content. Whether a rung above translated may be written, and
// whether a verdict may be recorded, is the venue's to answer: it holds the
// permissions, the workspace policy and the authorship that answer needs. So a
// push can carry an approval the venue declines to accept, and the report of
// what it declined travels back to the producer that sent it, both for the
// person reading the push and so the project's own record can follow the venue
// rather than sending the same refused verdict again on every push.

// Review states a decision record can carry.
const (
	ReviewStateApproved  = "approved"
	ReviewStateSignedOff = "signed-off"
	ReviewStateRejected  = "rejected"
)

// Refusal reasons, as a venue reports them.
const (
	// RefusedNoReviewPermission: the pusher does not hold review permission for
	// that language in that project.
	RefusedNoReviewPermission = "no review permission"
	// RefusedSeparationOfDuties: the workspace policy refuses a verdict on work
	// the decider wrote themselves.
	RefusedSeparationOfDuties = "separation of duties"
)

// Kinds of verdict a refusal counts.
const (
	VerdictApproval = "approval"
	VerdictSignOff  = "sign-off"
)

// DecisionRefusal counts one language's refused verdicts of one kind, for one
// reason. Counts rather than a list, because the line a person reads is
// "2 approvals not accepted for fr-FR: no review permission" however many units
// are behind it.
type DecisionRefusal struct {
	Locale string `json:"locale"`
	Kind   string `json:"kind"`   // approval | sign-off
	Reason string `json:"reason"` // no review permission | separation of duties
	Count  int    `json:"count"`
}

// RefusedUnit names one unit whose verdict a venue did not accept. It is what
// lets the producer bring its own record into line with the venue's, unit by
// unit, rather than guessing from the counts.
type RefusedUnit struct {
	ItemName string `json:"item"`
	Unit     string `json:"unit"`
	Variant  string `json:"variant"`
	Reason   string `json:"reason"`
}

// PushGovernance is a venue's answer about the verdicts a push carried: what it
// refused, why, and which units. Empty when a push carried no verdict, or when
// every verdict it carried was accepted.
type PushGovernance struct {
	Refusals []DecisionRefusal `json:"refusals,omitempty"`
	// Units names the refused units so the producer can retire exactly those
	// records. Bounded: a push that refuses more units than the bound names the
	// first RefusedUnitLimit of them and says so, and the counts above stay
	// exact either way.
	Units          []RefusedUnit `json:"units,omitempty"`
	UnitsTruncated bool          `json:"units_truncated,omitempty"`
}

// RefusedUnitLimit bounds the per-unit list a venue sends back. The report is
// read by a person and applied by a producer; neither needs an unbounded list,
// and an unbounded one is a push refusing a corpus writing that corpus into the
// job record.
const RefusedUnitLimit = 5000

// Empty reports whether any verdict was refused.
func (g PushGovernance) Empty() bool { return len(g.Refusals) == 0 }

// AsBasis returns the record a venue keeps when it refuses the verdict the
// record carried: the basis, without the verdict.
//
// A basis is not a decision. It says which source a translation was written
// for, which is true whoever wrote it and whether or not anybody has approved
// it. It is exactly what a producer records for every unit nobody has decided.
// So a refused approval is kept as the record it would have been had nobody
// approved it, at the rung the content actually landed on. Both ends compute
// it, which is what lets the producer's record and the venue's ledger agree
// again without another round trip.
func (d UnitDecision) AsBasis(status model.TargetStatus) UnitDecision {
	d.Status = string(status)
	d.ReviewState = ""
	d.DecidedBy = ""
	d.DecidedAt = ""
	return d
}

// CarriesVerdict reports whether a decision record claims something only a
// reviewer may claim: a review state above a plain basis, or a rung above
// translated.
//
// Both halves matter. The rung is what a ship gate counts, and a record can
// carry one with no review state at all (a producer writes the unit's rung
// beside its basis), so a gate reading only ReviewState would let "signed-off"
// through on a record that claims to be nobody's decision.
func (d UnitDecision) CarriesVerdict() bool {
	switch d.ReviewState {
	case ReviewStateApproved, ReviewStateSignedOff:
		return true
	}
	return model.TargetStatus(d.Status).Rank() > model.TargetStatusTranslated.Rank()
}

// VerdictKind names what a decision record claims, for the refusal it may earn.
func (d UnitDecision) VerdictKind() string {
	if d.ReviewState == ReviewStateSignedOff || model.TargetStatus(d.Status) == model.TargetStatusSignedOff {
		return VerdictSignOff
	}
	return VerdictApproval
}
