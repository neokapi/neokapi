package model

// SourceStatus is the authoring lifecycle state of a Block's source content —
// the source-side counterpart of TargetStatus. Where a Target progresses
// draft → translated → reviewed → signed-off, a source progresses
// authored → checked → approved: written, then cleared of brand/terminology
// findings, then signed off by a human. It is what keeps the author "in check"
// — the source equivalent of translation review.
type SourceStatus string

const (
	// SourceStatusNew ("") means no committed source status yet. It reads as the
	// authored baseline: any present, translatable source is at least authored.
	SourceStatusNew SourceStatus = ""
	// SourceStatusAuthored — source content exists (the presence baseline).
	SourceStatusAuthored SourceStatus = "authored"
	// SourceStatusChecked — source cleared its brand/terminology checks.
	SourceStatusChecked SourceStatus = "checked"
	// SourceStatusApproved — source signed off by a human/agent.
	SourceStatusApproved SourceStatus = "approved"
)

// SourceStatusLadder is the authoring lifecycle order, lowest to highest.
// Membership and order define "at least this status" coverage (used by a source
// gate). New ("") is not listed — it means "no committed status yet" and reads
// as the authored baseline, the lowest rung.
func SourceStatusLadder() []SourceStatus {
	return []SourceStatus{
		SourceStatusAuthored,
		SourceStatusChecked,
		SourceStatusApproved,
	}
}

// Rank returns the 0-based position of s on the ladder, or -1 for New ("") or an
// unknown status. A higher rank is a more advanced authoring state.
func (s SourceStatus) Rank() int {
	for i, t := range SourceStatusLadder() {
		if t == s {
			return i
		}
	}
	return -1
}
