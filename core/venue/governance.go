package venue

import "github.com/neokapi/neokapi/core/model"

// The shape of what a venue's review governance did with a push, in transit.
//
// A push moves content. Whether a rung above translated may be written,
// whether a verdict may be recorded, and whether an established unit the venue
// holds may be withdrawn, is the venue's to answer: it holds the permissions, the
// workspace policy and the authorship that answer needs. So a push can carry an
// approval the venue declines to accept, or a demotion it declines to apply,
// and the report of what it declined travels back to the producer that sent
// it, both for the person reading the push and so the project's own record can
// follow the venue rather than sending the same refused claim again on every
// push.

// Review states a decision record can carry.
const (
	ReviewStateApproved = "approved"
	ReviewStateRejected = "rejected"
)

// Refusal reasons, as a venue reports them.
const (
	// RefusedNoReviewPermission: the pusher does not hold review permission for
	// that language in that project.
	RefusedNoReviewPermission = "no review permission"
	// RefusedSeparationOfDuties: the workspace policy refuses a verdict on work
	// the decider wrote themselves.
	RefusedSeparationOfDuties = "separation of duties"
	// RefusedEstablishedWithdrawal: the push lowers a target the venue holds
	// at established, keeping the translation and the source the decision
	// blessed, and the pusher does not hold review permission for that
	// language. The venue keeps the unit established; withdrawing that is a
	// review-level action.
	RefusedEstablishedWithdrawal = "withdrawing an established unit needs review permission"
	// RefusedStaleRejection: the push carries a rejection of a translation the
	// venue has since replaced. A rejection judges one translation, so it
	// changes nothing about the one the venue holds now; the venue keeps its
	// own record and sends it back when it holds one.
	RefusedStaleRejection = "the rejection names a translation the platform no longer holds"
)

// Kinds of claim a refusal counts.
const (
	VerdictApproval = "approval"
	// VerdictDemotion is a pushed rung below the one the venue holds: an
	// un-review or a rejection, as the review surfaces call them.
	VerdictDemotion = "demotion"
)

// DecisionRefusal counts one language's refused claims of one kind, for one
// reason. Counts rather than a list, because the line a person reads is
// "2 approvals not accepted for fr-FR: no review permission" however many units
// are behind it.
type DecisionRefusal struct {
	Locale string `json:"locale"`
	Kind   string `json:"kind"`   // approval | demotion
	Reason string `json:"reason"` // one of the Refused* reasons
	Count  int    `json:"count"`
}

// RefusedUnit names one unit whose claim a venue did not accept. It is what
// lets the producer bring its own record into line with the venue's, unit by
// unit, rather than guessing from the counts.
type RefusedUnit struct {
	ItemName string `json:"item"`
	Unit     string `json:"unit"`
	Variant  string `json:"variant"`
	Reason   string `json:"reason"`
	// Held is the record the venue kept when the refusal left a standing
	// verdict in place rather than withholding a pushed one: an established
	// unit the push tried to withdraw. The producer writes it into its own record, so
	// the two agree without a pull. Nil for a refusal that withheld a verdict,
	// where the producer computes the basis itself.
	Held *UnitDecision `json:"held,omitempty"`
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
	// The fingerprint records the context the refused verdict was made under,
	// so it leaves with the verdict.
	d.GoverningFingerprint = ""
	return d
}

// CarriesVerdict reports whether a decision record claims something only a
// reviewer may claim: a review state above a plain basis, or a rung above
// translated.
//
// Both halves matter. The rung is what a ship gate counts, and a record can
// carry one with no review state at all (a producer writes the unit's rung
// beside its basis), so a gate reading only ReviewState would let
// "established" through on a record that claims to be nobody's decision.
func (d UnitDecision) CarriesVerdict() bool {
	if d.ReviewState == ReviewStateApproved {
		return true
	}
	return model.TargetStatus(d.Status).Rank() > model.TargetStatusTranslated.Rank()
}

// IsDecision reports whether a record decides something about its unit: a
// review state, a rung at reviewed or above, a parked unit, an assignee or a
// note. A record that says only what was produced (its target, the source it
// was written for and a rung below reviewed) is a basis, which a producer
// writes for every unit it drafts.
func (d UnitDecision) IsDecision() bool {
	return d.ReviewState != "" || d.CarriesVerdict() || d.Parked || d.Assignee != "" || d.Note != ""
}

// CarriesDecision reports whether any record decides something.
func CarriesDecision(decisions []UnitDecision) bool {
	for _, d := range decisions {
		if d.IsDecision() {
			return true
		}
	}
	return false
}

