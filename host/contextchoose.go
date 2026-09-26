package host

import (
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/contextop"
)

// ContextChooseRequest settles a conflict by choosing one side of it.
type ContextChooseRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// ID is the rule chosen.
	ID string
	// Replacement changes the chosen rule as it is kept, as a keep's does.
	Replacement string
	Note        string
}

// ContextChooseResult is what choosing did: the rules set aside, then the keep.
type ContextChooseResult struct {
	// SetAside are the drops and reverts that took the other side out.
	SetAside []ContextOperation `json:"set_aside"`
	// Kept is the keep that established the chosen rule.
	Kept ContextOperation `json:"kept"`
}

// ChooseContextSide settles a conflict for the rule a person chose: every
// rival rule is set aside (a suggestion dropped, an established rule
// reverted), and the chosen one is kept. A rule contested by evidence alone
// has no rival, and choosing it is keeping it.
//
// Each step is its own operation, so the log reads the choice the way a person
// would have made it by hand, and each can be taken back on its own.
func (a *App) ChooseContextSide(ctx context.Context, req ContextChooseRequest) (ContextChooseResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextChooseResult{}, err
	}
	chosen, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextChooseResult{}, err
	}
	if !chosen.Status.Answers() {
		return ContextChooseResult{}, fmt.Errorf("operation %s is %s: record it again to choose it", contextop.ShortID(chosen.ID), chosen.Status)
	}
	out := ContextChooseResult{SetAside: []ContextOperation{}}
	for _, id := range chosen.ContestedBy {
		rival, rerr := s.ledger.Get(ctx, id)
		if rerr != nil {
			return out, rerr
		}
		if !rival.Kind.Bears() || !rival.Status.Answers() {
			// A signal or a correction against the rule rather than a rival
			// rule: keeping the rule is the answer to it.
			continue
		}
		if rival.Established {
			res, verr := a.RevertContextOperations(ctx, ContextRevertRequest{Actor: req.Actor, Project: req.Project, ID: rival.ID, Note: req.Note})
			if verr != nil {
				return out, verr
			}
			out.SetAside = append(out.SetAside, res.Reverted...)
			continue
		}
		op, derr := a.DropContextOperation(ctx, ContextDropRequest{Actor: req.Actor, Project: req.Project, ID: rival.ID, Note: req.Note})
		if derr != nil {
			return out, derr
		}
		out.SetAside = append(out.SetAside, op)
	}
	kept, err := a.KeepContextOperation(ctx, ContextKeepRequest{
		Actor: req.Actor, Project: req.Project, ID: chosen.ID, Replacement: req.Replacement, Note: req.Note,
	})
	if err != nil {
		return out, err
	}
	out.Kept = kept
	return out, nil
}

// FormatText renders a choice: what was set aside, then what was kept.
func (r ContextChooseResult) FormatText(w io.Writer) error {
	for _, op := range r.SetAside {
		if _, err := fmt.Fprintln(w, "Set aside "+op.line()); err != nil {
			return err
		}
	}
	return r.Kept.FormatText(w)
}
