package host

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A review decision is a decide operation sent through the change service,
// as Kapi Desktop's Review page, kapi apply and MCP send one: the sender reads
// the edition it judges and names the revision the read shows. These helpers
// send one the way a review surface does, for a unit a review queue lists.

// reviewPerson is who decides in the tests: the person at the keyboard.
var reviewPerson = change.Actor{Kind: change.ActorPerson}

// decideUnit records decision (ReviewDecisionApproved or
// ReviewDecisionRejected; any other string is sent as the outcome) on the
// review-queue unit ref names, as a person, with note as the change set's
// note, which a rejection keeps as the reviewer's reason. It reports whether
// the decision changed the unit's record.
func decideUnit(ctx context.Context, a *App, recipe string, ref ReviewUnitRef, decision, note string) (bool, error) {
	return decideUnitAs(ctx, a, recipe, ref, decision, note, reviewPerson)
}

// decideUnitAs is decideUnit as actor.
func decideUnitAs(ctx context.Context, a *App, recipe string, ref ReviewUnitRef, decision, note string, actor change.Actor) (bool, error) {
	svc, at, rev, err := readReviewUnit(ctx, a, recipe, ref)
	if err != nil {
		return false, err
	}
	return sendDecision(ctx, svc, at, rev, decision, note, actor)
}

// rejectUnitWithReasons records a person's rejection of the review-queue unit
// ref names, with note and the reason codes a review surface sends beside it.
func rejectUnitWithReasons(ctx context.Context, a *App, recipe string, ref ReviewUnitRef, note string, reasons ...string) (bool, error) {
	svc, at, rev, err := readReviewUnit(ctx, a, recipe, ref)
	if err != nil {
		return false, err
	}
	return sendDecide(ctx, svc, at, rev, note, &change.Decide{Outcome: change.OutcomeReject, Reasons: reasons}, reviewPerson)
}

// readReviewUnit reads the unit ref names on its target edition, the way a
// review surface reads what it judges, and returns where to send a decision
// and the revision the read shows.
func readReviewUnit(ctx context.Context, a *App, recipe string, ref ReviewUnitRef) (*change.Service, change.Ref, string, error) {
	edition := model.EditionKey{Locale: model.LocaleID(ref.Locale)}
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "test", TargetLocale: edition.Locale})
	if err != nil {
		return nil, change.Ref{}, "", err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: ref.File, Blocks: []string{ref.Key}, Editions: []model.EditionKey{edition}})
	if err != nil {
		return nil, change.Ref{}, "", err
	}
	if len(page.Blocks) == 0 {
		return nil, change.Ref{}, "", fmt.Errorf("block %q (%s) under review not found in %s", ref.Key, ref.Locale, ref.File)
	}
	b := page.Blocks[0]
	at, rev := b.Ref, b.Rev
	if at.Edition.Canonical() != edition.Canonical() {
		at.Edition = edition
		rev = model.AbsentRevision
		if ed, ok := b.Editions[at.EditionText()]; ok {
			rev = ed.Rev
		}
	}
	return svc, at, rev, nil
}

// approveSource records a person's approval of the source wording of the
// block key in file: decide establish on the document's own edition.
func approveSource(ctx context.Context, a *App, recipe string, ref SourceUnitRef) (bool, error) {
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, Origin: "test"})
	if err != nil {
		return false, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: ref.File, Blocks: []string{ref.Key}})
	if err != nil {
		return false, err
	}
	if len(page.Blocks) == 0 {
		return false, fmt.Errorf("source block %q not found in %s", ref.Key, ref.File)
	}
	b := page.Blocks[0]
	return sendDecision(ctx, svc, b.Ref, b.Rev, ReviewDecisionApproved, "", reviewPerson)
}

// sendDecision sends one decide operation on at, read at rev.
func sendDecision(ctx context.Context, svc *change.Service, at change.Ref, rev, decision, note string, actor change.Actor) (bool, error) {
	outcome := change.Outcome(decision)
	switch decision {
	case ReviewDecisionApproved:
		outcome = change.OutcomeEstablish
	case ReviewDecisionRejected:
		outcome = change.OutcomeReject
	}
	return sendDecide(ctx, svc, at, rev, note, &change.Decide{Outcome: outcome}, actor)
}

// sendDecide sends body as one decide operation on at, read at rev.
func sendDecide(ctx context.Context, svc *change.Service, at change.Ref, rev, note string, body *change.Decide, actor change.Actor) (bool, error) {
	res, err := svc.Apply(ctx, change.Set{Note: note, Ops: []change.Op{
		{Kind: change.KindDecide, At: at, IfMatch: rev, Body: body},
	}}, actor)
	if err != nil {
		return false, err
	}
	switch op := res.Ops[0]; op.Status {
	case change.OpApplied:
		return true, nil
	case change.OpUnchanged:
		return false, nil
	default:
		if op.Error != nil {
			return false, op.Error
		}
		return false, fmt.Errorf("the decision was %s", op.Status)
	}
}
