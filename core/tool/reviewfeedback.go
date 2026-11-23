package tool

import (
	"context"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
)

// ReviewFeedbackKey is the configuration key a drafting tool and a recycle
// step take review feedback under: a map from a unit's key to the latest
// rejections of its earlier drafts. A host builds it from its own record of
// decisions, on every route, and the tools know nothing of where it came from.
const ReviewFeedbackKey = "review_feedback"

// MaxRejectionsSent is how many of a unit's latest rejections a prompt
// carries; older ones are dropped. MaxRejections is how many rejections of
// drafts of one source a unit takes before kapi stops drafting it and leaves
// it for a person.
const (
	MaxRejectionsSent = 3
	MaxRejections     = 3
)

// ReviewRejection is one rejection of an earlier draft of a unit: what was
// rejected and why. It names no reviewer.
type ReviewRejection struct {
	// Text is the rejected draft, when the host still holds its wording. A
	// rejection whose wording is gone still carries its reasons and note.
	Text string `json:"text,omitempty"`
	// TargetHash identifies the rejected draft, so the rejection is the same
	// fact whether or not its wording is still held.
	TargetHash string `json:"target_hash,omitempty"`
	// Reasons are the reviewer's reason codes (RejectionReasonPhrase renders
	// each as a person reads it).
	Reasons []string `json:"reasons,omitempty"`
	// Note is what the reviewer wrote.
	Note string `json:"note,omitempty"`
}

// UnitReviewFeedback is what a host says about one unit's earlier drafts.
type UnitReviewFeedback struct {
	// Rejections are the latest rejections, oldest first, at most
	// MaxRejectionsSent.
	Rejections []ReviewRejection `json:"rejections,omitempty"`
	// Held marks a unit rejected MaxRejections times: kapi drafts it no
	// more, and a person writes its translation.
	Held bool `json:"held,omitempty"`
}

// ReviewFeedback maps a unit (FeedbackUnitKey) to its feedback.
type ReviewFeedback map[string]UnitReviewFeedback

// FeedbackUnitKey is the key a unit's feedback is filed under: the source
// file the unit is read from, as blockstore.SourceNamespace names it, and the
// unit's key in that document. One run's tools serve several files, and two
// files can hold a unit of the same key.
func FeedbackUnitKey(sourceRel, unit string) string {
	if sourceRel == "" {
		return unit
	}
	return sourceRel + "#" + unit
}

// ForBlock returns the feedback for the unit with key unit in the file the
// call's context names (blockstore.SourceRel).
func (f ReviewFeedback) ForBlock(ctx context.Context, unit string) UnitReviewFeedback {
	if len(f) == 0 {
		return UnitReviewFeedback{}
	}
	return f[FeedbackUnitKey(blockstore.SourceRel(ctx), unit)]
}

// rejectionPhrases renders each reason code as the words a person reads. A
// code missing here reaches a prompt as "another reason", never as a bare
// identifier. TestReviewFeedback_EveryReasonHasAPhrase holds every code in
// venue.DecisionReasons to a phrase.
var rejectionPhrases = map[string]string{
	"wrong_term":  "a term is wrong",
	"meaning":     "the meaning is wrong",
	"tone":        "the tone does not fit",
	"placeholder": "a placeholder is wrong or missing",
	"grammar":     "the grammar is wrong",
}

// RejectionReasonPhrase renders a reason code as the words a person reads.
func RejectionReasonPhrase(code string) string {
	if p, ok := rejectionPhrases[code]; ok {
		return p
	}
	return "another reason"
}

// FeedbackFingerprint folds a unit's review feedback into the governing
// fingerprint a producer stamps on its draft, and the staleness read
// recomputes the same way, per unit. A unit with no feedback keeps contextFP
// as it is, so only a unit whose feedback changed (a note edited, a new
// rejection) reads as stale. The wording of a rejected draft is left out:
// whether the host still holds it says nothing about the feedback.
func FeedbackFingerprint(contextFP string, fb UnitReviewFeedback) string {
	if len(fb.Rejections) == 0 {
		return contextFP
	}
	parts := []string{contextFP, "review-feedback"}
	for _, r := range fb.Rejections {
		reasons := slices.Clone(r.Reasons)
		slices.Sort(reasons)
		parts = append(parts, r.TargetHash, strings.Join(reasons, ","), r.Note)
	}
	return OverlayConfigFingerprint(parts...)
}
