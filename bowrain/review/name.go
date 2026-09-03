package review

import "github.com/neokapi/neokapi/core/model"

// DecisionName labels a rung change for the audit log: what the decider did, in
// the vocabulary the review surfaces use. It matches the ledger's ReviewState,
// so the security trail and the content trail call the same decision by the
// same name, and every route that records one uses this function: the review
// endpoint, the bulk routes and the sync worker that applies a push.
func DecisionName(approved bool, to model.TargetStatus) string {
	switch {
	case approved && to == model.TargetStatusSignedOff:
		return "signed-off"
	case approved:
		return "approved"
	case to == model.TargetStatusDraft:
		return "rejected"
	default:
		return "unreviewed"
	}
}
