package backend

// Decide asks about each suggestion waiting for a person as one question, with
// the evidence as its options.
//
// The host reads the questions `kapi rules` asks (DecideRules), with each
// option's blocks and files and what keeping it does. It records an answer
// through the calls `kapi rules` makes for k, n and l (DecideRule). This file
// shapes the questions for the screen and passes the answers on. It decides
// nothing of its own.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// rulesDecideTimeout caps one read of the questions. The read counts each
// option's uses over every block of the project's source, so it takes longer
// than a read of the log alone.
const rulesDecideTimeout = 2 * time.Minute

// RulesDecideView reads the questions about the project a tab holds. A folder
// without a kapi.yaml has nothing recorded and answers with no question.
func (a *App) RulesDecideView(tabID string) (*screens.RulesDecideViewDTO, error) {
	op, err := a.decideTab(tabID)
	if err != nil {
		return nil, err
	}
	out := &screens.RulesDecideViewDTO{
		Project:     string(op.workspaceKey),
		ProjectName: projectDisplayName(op.Project, op.Path),
		Decisions:   []screens.RuleDecisionDTO{},
	}
	if op.unsaved.Load() {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), rulesDecideTimeout)
	defer cancel()
	// The last read's blocks are counted again while the source is unchanged,
	// so an answer does not cost a read of every file.
	op.decideMu.Lock()
	last := op.decideView
	op.decideMu.Unlock()
	view, err := a.hostEngine().DecideRules(ctx, host.RulesRequest{Project: op.Path, WithCounts: true, Reuse: last})
	if err != nil {
		return nil, err
	}
	op.decideMu.Lock()
	op.decideView = view
	op.decideMu.Unlock()

	out.Rules, out.Suggested, out.Narrowed = view.Counts.Rules, view.Counts.Suggested, view.Counts.Narrowed
	for _, d := range view.Decisions {
		out.Decisions = append(out.Decisions, ruleDecisionDTO(d))
	}
	return out, nil
}

// DecideRule records one answer: "keep" (with the form a kept name writes,
// empty for the one the suggestion names), "drop" for not a rule, or "later",
// which records nothing. id names the suggestion, or for a disagreement the
// side a keep chooses. Keeping is the first act that gives a folder opened
// without a recipe its kapi.yaml.
func (a *App) DecideRule(tabID, id, answer, form string) (*screens.RuleDecideResultDTO, error) {
	op, err := a.decideTab(tabID)
	if err != nil {
		return nil, err
	}
	if answer == host.RuleAnswerKeep {
		if err := a.ensureRecipeForKey(op.workspaceKey); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	res, err := a.hostEngine().DecideRule(ctx, host.RuleDecideRequest{
		Actor:   deskPerson(),
		Project: op.Path,
		ID:      id,
		Answer:  answer,
		Form:    form,
	})
	if res.Review != nil {
		a.emitEvent("workspace:changed", nil)
	}
	if err != nil {
		return nil, err
	}
	out := &screens.RuleDecideResultDTO{Answer: res.Answer, Later: len(res.Later)}
	if r := res.Review; r != nil {
		out.Kept, out.Dropped = len(r.Kept), len(r.Dropped)
		if r.Chosen != nil {
			out.Kept++
			out.Dropped += len(r.Chosen.SetAside)
		}
	}
	return out, nil
}

// RulesDecideConsequence states what keeping a name with a form the person
// typed does, counted over the blocks the last read of the questions counted.
func (a *App) RulesDecideConsequence(tabID, id, form string) (string, error) {
	op, err := a.decideTab(tabID)
	if err != nil {
		return "", err
	}
	op.decideMu.Lock()
	view := op.decideView
	op.decideMu.Unlock()
	if view == nil {
		return "", errors.New("read the questions before asking what an answer does")
	}
	d, _, ok := view.Find(id)
	if !ok {
		return "", fmt.Errorf("no suggestion %s waits for a decision", contextop.ShortID(id))
	}
	if d.Kind != host.RuleDecisionName {
		return "", errors.New("only a name takes a form of its own")
	}
	return view.Consequence(d, form), nil
}

// decideTab is the tab a Decide call is for, which holds a checkout: a tab
// over a project's context alone has no files to count uses in.
func (a *App) decideTab(tabID string) (*openProject, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.contextOnly {
		return nil, errors.New("no checkout of this project is on this machine, and the questions count uses in its files")
	}
	return op, nil
}

// ruleDecisionDTO shapes one question for the screen, its options the most
// used first.
func ruleDecisionDTO(d host.RuleDecision) screens.RuleDecisionDTO {
	out := screens.RuleDecisionDTO{
		ID:          d.ID,
		Short:       d.Short,
		Kind:        d.Kind,
		Question:    d.Question,
		Reason:      d.Reason,
		Sentence:    d.Sentence,
		Label:       d.Label(),
		Avoided:     d.Avoided(),
		Options:     []screens.RuleOptionDTO{},
		Other:       d.Kind == host.RuleDecisionName,
		Consequence: d.Consequence,
		Applies:     d.Applies,
		NoticedBy:   actorDTO(d.NoticedBy()),
	}
	if e := d.Seen; e != nil {
		out.Seen = &screens.ContextEvidenceDTO{Path: e.Path, Unit: e.Unit, Quote: e.Quote}
	}
	order := make([]int, len(d.Options))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int { return d.Options[y].Blocks - d.Options[x].Blocks })
	for at, i := range order {
		o := d.Options[i]
		opt := screens.RuleOptionDTO{
			Form: o.Form, ID: o.ID, Short: o.Short, Sentence: o.Sentence,
			Blocks: o.Blocks, Files: o.Files, Consequence: o.Consequence,
		}
		if d.Kind == host.RuleDecisionDisagreement {
			who := actorDTO(o.NoticedBy())
			opt.NoticedBy = &who
		}
		out.Options = append(out.Options, opt)
		if i == d.Chosen {
			out.Chosen = at
		}
	}
	return out
}
