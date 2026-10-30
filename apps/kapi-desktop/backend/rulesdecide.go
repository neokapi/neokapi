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
)

// rulesDecideTimeout caps one read of the questions. The read counts each
// option's uses over every block of the project's source, so it takes longer
// than a read of the log alone.
const rulesDecideTimeout = 2 * time.Minute

// RulesDecideViewDTO is every question Decide asks about one project.
type RulesDecideViewDTO struct {
	// Project is the project's workspace key, and ProjectName what a person
	// calls it.
	Project     string            `json:"project"`
	ProjectName string            `json:"project_name"`
	Decisions   []RuleDecisionDTO `json:"decisions"`
	// Rules, Suggested and Narrowed are the counts `kapi rules list` opens
	// with: the rules in force, the suggestions waiting and the rules that
	// hold in only part of the project.
	Rules     int `json:"rules"`
	Suggested int `json:"suggested"`
	Narrowed  int `json:"narrowed"`
}

// RuleDecisionDTO is one question.
type RuleDecisionDTO struct {
	// ID names the suggestion; a disagreement has none, and each option
	// carries its side's.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	// Kind is "name", "word", "note", "memory" or "disagreement".
	Kind     string `json:"kind"`
	Question string `json:"question"`
	// Reason says what a disagreement is about.
	Reason   string `json:"reason,omitempty"`
	Sentence string `json:"sentence"`
	// Label is the wording the question is about, for its row in the list,
	// and Avoided how many wordings a word rule's check reports.
	Label   string `json:"label"`
	Avoided int    `json:"avoided"`
	// Options are the forms of a name or the sides of a disagreement, the
	// most used first. Chosen is the option a keep takes unless the person
	// picks another.
	Options []RuleOptionDTO `json:"options"`
	Chosen  int             `json:"chosen"`
	// Other reports a question that takes a form the person types: a name.
	Other bool `json:"other"`
	// Consequence says what keeping it does, for a question with no options.
	Consequence string `json:"consequence,omitempty"`
	// Seen is the sentence the suggestion was seen in, and its file.
	Seen *ContextEvidenceDTO `json:"seen,omitempty"`
	// Applies is where a kept rule holds.
	Applies   string          `json:"applies"`
	NoticedBy ContextActorDTO `json:"noticed_by"`
}

// RuleOptionDTO is one answer to a question.
type RuleOptionDTO struct {
	// Form is one wording of a name; ID, Short and Sentence name and state one
	// side of a disagreement.
	Form     string `json:"form,omitempty"`
	ID       string `json:"id,omitempty"`
	Short    string `json:"short,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	// Blocks and Files count where the project's source writes it.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
	// Consequence says what keeping this option does.
	Consequence string `json:"consequence"`
	// NoticedBy is who suggested the side, for a disagreement.
	NoticedBy *ContextActorDTO `json:"noticed_by,omitempty"`
}

// RuleDecideResultDTO is what one answer recorded.
type RuleDecideResultDTO struct {
	// Answer is "keep", "drop" or "later".
	Answer string `json:"answer"`
	// Kept and Dropped count the operations the answer recorded; a choice
	// between two sides keeps one and drops the rest.
	Kept    int `json:"kept"`
	Dropped int `json:"dropped"`
	// Later counts the suggestions left waiting, which records nothing.
	Later int `json:"later"`
}

// RulesDecideView reads the questions about the project a tab holds. A folder
// without a kapi.yaml has nothing recorded and answers with no question.
func (a *App) RulesDecideView(tabID string) (*RulesDecideViewDTO, error) {
	op, err := a.decideTab(tabID)
	if err != nil {
		return nil, err
	}
	out := &RulesDecideViewDTO{
		Project:     string(op.workspaceKey),
		ProjectName: projectDisplayName(op.Project, op.Path),
		Decisions:   []RuleDecisionDTO{},
	}
	if op.unsaved.Load() {
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), rulesDecideTimeout)
	defer cancel()
	view, err := a.hostEngine().DecideRules(ctx, host.RulesRequest{Project: op.Path, WithCounts: true})
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
func (a *App) DecideRule(tabID, id, answer, form string) (*RuleDecideResultDTO, error) {
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
	out := &RuleDecideResultDTO{Answer: res.Answer, Later: len(res.Later)}
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
func ruleDecisionDTO(d host.RuleDecision) RuleDecisionDTO {
	out := RuleDecisionDTO{
		ID:          d.ID,
		Short:       d.Short,
		Kind:        d.Kind,
		Question:    d.Question,
		Reason:      d.Reason,
		Sentence:    d.Sentence,
		Label:       d.Label(),
		Avoided:     d.Avoided(),
		Options:     []RuleOptionDTO{},
		Other:       d.Kind == host.RuleDecisionName,
		Consequence: d.Consequence,
		Applies:     d.Applies,
		NoticedBy:   actorDTO(d.NoticedBy()),
	}
	if e := d.Seen; e != nil {
		out.Seen = &ContextEvidenceDTO{Path: e.Path, Unit: e.Unit, Quote: e.Quote}
	}
	order := make([]int, len(d.Options))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int { return d.Options[y].Blocks - d.Options[x].Blocks })
	for at, i := range order {
		o := d.Options[i]
		opt := RuleOptionDTO{
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
