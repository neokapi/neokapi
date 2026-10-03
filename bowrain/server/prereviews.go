package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// A pre-review is a decide with outcome advise: the score an agent gives a
// translation and its reasons, recorded against the translation's revision.
// It moves no status and writes no decision. The review queue shows it beside
// the translation for as long as the translation stands at that revision, as
// the kapi host shows the AI pre-review it keeps on a unit.

// prepareAdvice checks a pre-review before anything is written: it carries a
// score, and the translation it judges exists. A translation of inline codes
// alone, or an empty one, is still a translation to judge.
func (d *streamDecisions) prepareAdvice(ctx context.Context, body *change.Decide, target *change.DecisionTarget) *change.Error {
	if body.Score == nil {
		return &change.Error{Code: change.CodeInvalid, Field: "score",
			Message: "a pre-review carries the score it gives, from 0 to 100"}
	}
	if d.rows.find(ctx, target.Doc.Doc, target.Ref.Block) == nil {
		return &change.Error{Code: change.CodeNotFound, Field: "at/block",
			Message: fmt.Sprintf("%s holds no block keyed %q", target.Doc.Doc, target.Ref.Block)}
	}
	if target.Rev == model.AbsentRevision {
		return &change.Error{Code: change.CodeNotFound, Field: "at",
			Message: fmt.Sprintf("block %s has no %s translation to pre-review", target.Ref.Block, target.Ref.Edition.Locale)}
	}
	return nil
}

// recordAdvice stores a pre-review on the translation it judges, in place of
// any earlier one.
func (d *streamDecisions) recordAdvice(ctx context.Context, actor change.Actor, body *change.Decide, target *change.DecisionTarget) (change.OpStatus, *change.Error) {
	row := d.rows.find(ctx, target.Doc.Doc, target.Ref.Block)
	if row == nil {
		return "", &change.Error{Code: change.CodeNotFound, Field: "at/block",
			Message: fmt.Sprintf("%s holds no block keyed %q", target.Doc.Doc, target.Ref.Block)}
	}
	r := store.PreReview{
		BlockID:  row.Block.ID,
		Locale:   string(target.Ref.Edition.Locale),
		Score:    *body.Score,
		Reviewer: adviser(actor),
		Reasons:  body.Reasons,
		Revision: target.Rev,
		At:       time.Now().UTC(),
	}
	if err := d.s.ContentStore.RecordPreReview(ctx, d.proj.ID, d.stream, r); err != nil {
		return "", &change.Error{Code: change.CodeUnreachable, Message: "the pre-review could not be recorded: " + err.Error()}
	}
	return change.OpApplied, nil
}

// adviser is the name a pre-review is recorded under, which the review queue
// shows beside its score: agent/<client>, as the kapi host names an agent. The
// policy takes a pre-review from an agent only.
func adviser(actor change.Actor) string {
	if actor.Name == "" {
		return "agent"
	}
	return "agent/" + actor.Name
}

// freshPreReview is the pre-review recorded on the translation of sb in loc
// while it judges the translation as it stands, nil when there is none.
func freshPreReview(reviews []store.PreReview, sb *venue.StoredBlock, loc model.LocaleID) *store.PreReview {
	if sb == nil || sb.Block == nil {
		return nil
	}
	rev := store.TargetRevision(sb, loc)
	for i := range reviews {
		r := &reviews[i]
		if r.BlockID == sb.Block.ID && model.NormalizeLocale(model.LocaleID(r.Locale)) == model.NormalizeLocale(loc) &&
			r.Revision == rev {
			return r
		}
	}
	return nil
}

// fillPreReview sets the judgement's pre-review fields from the advice an
// agent recorded on the translation, while it judges the translation as it
// stands. A failed read leaves them unset.
func (s *Server) fillPreReview(ctx context.Context, out *reviewContextResponse, pid, stream string, sb *venue.StoredBlock, loc model.LocaleID) {
	reviews, err := s.ContentStore.PreReviews(ctx, pid, stream, []string{sb.Block.ID})
	if err != nil {
		slog.WarnContext(ctx, "review context: read pre-reviews failed", "project", pid, "block", sb.Block.ID, "error", err)
		return
	}
	r := freshPreReview(reviews, sb, loc)
	if r == nil {
		return
	}
	score := r.Score
	out.Judgement.AIScore = &score
	out.Judgement.AIModel = r.Reviewer
	for _, reason := range r.Reasons {
		out.Judgement.AIFindings = append(out.Judgement.AIFindings, state.AIReviewFinding{Message: reason})
	}
}

// preReviewView is a pre-review as a review queue entry carries it.
type preReviewView struct {
	Score    int      `json:"score"`
	Reviewer string   `json:"reviewer"`
	Reasons  []string `json:"reasons,omitempty"`
}

func preReviewViewOf(r *store.PreReview) *preReviewView {
	if r == nil {
		return nil
	}
	return &preReviewView{Score: r.Score, Reviewer: r.Reviewer, Reasons: r.Reasons}
}
