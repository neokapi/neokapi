package cli

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
)

// A review decision is a decide operation sent through the change service,
// as Kapi Desktop's Review page, kapi apply and MCP send one: the sender reads
// the edition it judges and names the revision the read shows.

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

// approveQueued approves the review-queue unit in file keyed key, in locale.
func approveQueued(ctx context.Context, a *App, recipe, locale, file, key string) (bool, error) {
	return decideUnit(ctx, a, recipe, ReviewUnitRef{File: file, Key: key, Locale: locale}, ReviewDecisionApproved, "")
}

// decideUnitAs is decideUnit as actor.
func decideUnitAs(ctx context.Context, a *App, recipe string, ref ReviewUnitRef, decision, note string, actor change.Actor) (bool, error) {
	edition := model.EditionKey{Locale: model.LocaleID(ref.Locale)}
	svc, err := a.ChangeService(ctx, host.ChangeServiceOptions{Project: recipe, Origin: "test", TargetLocale: edition.Locale})
	if err != nil {
		return false, err
	}
	page, err := svc.Read(ctx, change.ReadRequest{Doc: ref.File, Blocks: []string{ref.Key}, Editions: []model.EditionKey{edition}})
	if err != nil {
		return false, err
	}
	if len(page.Blocks) == 0 {
		return false, fmt.Errorf("review unit %q (%s) not found in %s", ref.Key, ref.Locale, ref.File)
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
	return sendDecision(ctx, svc, change.Op{Kind: change.KindDecide, At: at, IfMatch: rev, Body: &change.Decide{Outcome: outcomeOf(decision)}}, note, actor)
}

// outcomeOf is the decide outcome a review decision sends.
func outcomeOf(decision string) change.Outcome {
	switch decision {
	case ReviewDecisionApproved:
		return change.OutcomeEstablish
	case ReviewDecisionRejected:
		return change.OutcomeReject
	}
	return change.Outcome(decision)
}

// sendDecision applies one decide operation.
func sendDecision(ctx context.Context, svc *change.Service, op change.Op, note string, actor change.Actor) (bool, error) {
	res, err := svc.Apply(ctx, change.Set{Note: note, Ops: []change.Op{op}}, actor)
	if err != nil {
		return false, err
	}
	switch r := res.Ops[0]; r.Status {
	case change.OpApplied:
		return true, nil
	case change.OpUnchanged:
		return false, nil
	default:
		if r.Error != nil {
			return false, r.Error
		}
		return false, fmt.Errorf("the decision was %s", r.Status)
	}
}
