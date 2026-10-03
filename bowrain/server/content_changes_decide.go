package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// streamDecisions is the server's change.Assets: decide, applied to the
// translation it binds to with the review semantics every review path shares
// (applyBlockReview): the status transition, the decision ledger and the
// workspace content memory. A stream keeps no terms, content memory or recipe
// of its own through a change set, so those operations are refused as
// unsupported.
//
//   - establish moves the translation to established, under the workspace's
//     separation-of-duties policy.
//   - reject moves it to draft, so it re-enters the work queue; withdraw moves
//     it to translated. Moving an established translation takes the review
//     permission for its language.
//   - advise is refused: the server keeps no pre-reviews.
type streamDecisions struct {
	s      *Server
	c      echo.Context
	proj   *store.Project
	stream string
	rows   *rowLookup

	// sod, when set, is the separation-of-duties gate a bulk action opened
	// for every block and language it decides, so a decision asks no query of
	// its own.
	sod *reviewSoD
	// ledger is the decision ledger the change set's decisions write
	// through, opened on the first.
	ledger     *reviewLedger
	ledgerOpen bool
	// quiet files no review.decided record per decision: a bulk action over
	// a corpus files one record for the pass.
	quiet bool

	// approved are the languages a decision established a translation in,
	// which the review loop continues from once the change set has landed.
	approved map[model.LocaleID]bool
	// applied counts the decisions that moved a translation.
	applied int
}

var _ change.Assets = (*streamDecisions)(nil)

func (d *streamDecisions) Prepare(ctx context.Context, _ change.Actor, op change.Op, target *change.DecisionTarget) *change.Error {
	if op.Kind != change.KindDecide {
		return &change.Error{Code: change.CodeUnsupported, Capability: string(op.Kind),
			Message: fmt.Sprintf("a stream takes no %s operation: terms, content memory and the recipe are kept outside the stream", op.Kind)}
	}
	body := op.Body.(*change.Decide)
	switch {
	case d.c == nil:
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide",
			Message: "a review decision on a stream is a person's, sent to the server's changes route"}
	case target == nil:
		return &change.Error{Code: change.CodeNotFound, Field: "at", Message: "the decision names no edition"}
	case target.Role == change.RoleAuthoritative:
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide",
			Message: "a stream records review decisions on translations; the source is reviewed where it is written"}
	case target.Ref.Edition.Tone != "" || target.Ref.Edition.Channel != "":
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide",
			Message: "a stream records review decisions on a language's translation, not on a tone or channel edition"}
	case body.Outcome == change.OutcomeAdvise:
		return &change.Error{Code: change.CodeUnsupported, Capability: "decide.advise",
			Message: "the server keeps no pre-reviews; a person decides"}
	}
	row := d.rows.find(ctx, target.Doc.Doc, target.Ref.Block)
	if row == nil {
		return &change.Error{Code: change.CodeNotFound, Field: "at/block",
			Message: fmt.Sprintf("%s holds no block keyed %q", target.Doc.Doc, target.Ref.Block)}
	}
	locale := string(target.Ref.Edition.Locale)
	current := row.Block.Target(target.Ref.Edition.Locale)
	switch body.Outcome {
	case change.OutcomeEstablish:
		if current == nil || strings.TrimSpace(target.Text) == "" {
			return &change.Error{Code: change.CodeUnsupported, Capability: "decide.establish",
				Message: fmt.Sprintf("block %s has no %s translation to establish: translate it first", target.Ref.Block, locale)}
		}
		if current.Status.Rank() >= model.TargetStatusEstablished.Rank() {
			return nil
		}
		sod := d.sod
		if sod == nil {
			var err error
			sod, err = d.s.newReviewSoD(ctx, d.c, d.proj.ID, d.stream, []string{row.Block.ID}, []string{locale})
			if err != nil {
				return &change.Error{Code: change.CodeUnreachable, Message: "the separation-of-duties policy could not be read: " + err.Error()}
			}
		}
		if err := sod.vet(row.Block.ID, locale); err != nil {
			return decisionRefusal(err)
		}
	default:
		if current != nil && current.Status == model.TargetStatusEstablished && !allowsLanguage(d.c, platauth.PermReview, locale) {
			return &change.Error{Code: change.CodeNotPermitted,
				Message: "moving an established translation takes the review permission for " + locale}
		}
	}
	return nil
}

func (d *streamDecisions) Apply(ctx context.Context, _ change.Actor, set *change.Set, op change.Op, target *change.DecisionTarget) (change.OpStatus, *change.Error) {
	body := op.Body.(*change.Decide)
	row := d.rows.find(ctx, target.Doc.Doc, target.Ref.Block)
	if row == nil {
		return "", &change.Error{Code: change.CodeNotFound, Field: "at/block",
			Message: fmt.Sprintf("%s holds no block keyed %q", target.Doc.Doc, target.Ref.Block)}
	}
	locale := string(target.Ref.Edition.Locale)
	req := ReviewBlockRequest{TargetLocale: locale, ItemName: target.Doc.Doc, BaseRevision: target.Rev}
	demoteTo := model.TargetStatusTranslated
	switch body.Outcome {
	case change.OutcomeEstablish:
		req.Reviewed = true
	case change.OutcomeReject:
		req.Status = string(model.TargetStatusDraft)
		demoteTo = model.TargetStatusDraft
	}
	if !d.ledgerOpen {
		d.ledger, d.ledgerOpen = d.s.newReviewLedger(ctx, d.c, d.proj.ID, d.stream), true
	}
	out, err := d.s.applyBlockReview(ctx, d.c, blockReviewInput{
		ProjectID: d.proj.ID, Stream: d.stream, BlockID: row.Block.ID, Request: req,
		DemoteTo: demoteTo, PromoteTo: model.TargetStatusEstablished,
		// Both were asked when the decision was prepared.
		Elevate: func() error { return nil },
		Ledger:  d.ledger,
	})
	if err != nil {
		return "", decisionRefusal(err)
	}
	if !out.Changed {
		return change.OpUnchanged, nil
	}
	note := ""
	if set != nil {
		note = set.Note
	}
	if !d.quiet {
		d.s.emitReviewDecisionAudit(d.c, d.proj.ID, d.stream, row.Block.ID, locale, out.From, out.Status, req.Reviewed, note)
	}
	d.applied++
	if out.Approval {
		if d.approved == nil {
			d.approved = map[model.LocaleID]bool{}
		}
		d.approved[target.Ref.Edition.Locale] = true
	}
	return change.OpApplied, nil
}

// decisionRefusal is a review refusal as the change contract reports it.
func decisionRefusal(err error) *change.Error {
	if _, ok := asBlockChanged(err); ok {
		return &change.Error{Code: change.CodeStale, Field: "if_match",
			Message: "the translation moved since the decision was made; read it again and decide on its wording"}
	}
	if fault, ok := errors.AsType[reviewFault](err); ok {
		code := change.CodeInvalid
		switch fault.code {
		case http.StatusForbidden:
			code = change.CodeNotPermitted
		case http.StatusNotFound:
			code = change.CodeNotFound
		case http.StatusUnprocessableEntity:
			code = change.CodeUnsupported
		}
		return &change.Error{Code: code, Message: fault.msg}
	}
	return &change.Error{Code: change.CodeUnreachable, Message: "the decision could not be recorded: " + err.Error()}
}
