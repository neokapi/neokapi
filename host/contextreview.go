package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/neokapi/neokapi/core/contextop"
)

// Reviewing a project's context (AD C-11): the decisions a person makes about
// what was suggested. `kapi context review` walks the digest and asks about
// each item, or takes the decisions as flags; both end here, so a decision
// made at a prompt and one made by a script record the same operations.

// ContextReviewAll is the value ContextReviewRequest.Keep takes, beside a
// session, to keep everything that session suggested that nothing disagrees
// with.
const ContextReviewAll = "all"

// ContextReviewRequest is one round of review decisions.
type ContextReviewRequest struct {
	// Actor is who is deciding. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go). Only a person decides.
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// Keep names the suggestions to keep. A rule already in force named here
	// with WidenTo is applied more widely. ContextReviewAll, with Session,
	// keeps everything the session suggested that nothing disagrees with.
	Keep []string
	// Drop names the suggestions and rules to set aside.
	Drop []string
	// Choose names one side of a contested pair: the other side is dropped and
	// this one kept.
	Choose string
	// Session narrows a ContextReviewAll keep to one agent session.
	Session string
	// WidenTo applies what Keep names more widely: "project", "workspace", or
	// the name of an axis the rule should stop being specific about.
	WidenTo string
	// Replacement changes what the one kept rule says to write instead.
	Replacement string
	// Advisory, when set, replaces whether a kept rule only reports (true) or
	// fails a check (false).
	Advisory *bool
	Note     string
}

// Empty reports a request that decides nothing.
func (r ContextReviewRequest) Empty() bool {
	return len(r.Keep) == 0 && len(r.Drop) == 0 && r.Choose == ""
}

// ContextReviewResult is what one round of review decisions recorded.
type ContextReviewResult struct {
	Kept    []ContextOperation   `json:"kept"`
	Widened []ContextOperation   `json:"widened"`
	Dropped []ContextOperation   `json:"dropped"`
	Chosen  *ContextChooseResult `json:"chosen,omitempty"`
	// Skipped are the suggestions a session keep left alone, with the reason.
	Skipped []ContextKeepSkip `json:"skipped,omitempty"`
}

// FormatText renders the decisions, one line each.
func (r ContextReviewResult) FormatText(w io.Writer) error {
	if r.Chosen != nil {
		if err := r.Chosen.FormatText(w); err != nil {
			return err
		}
	}
	if len(r.Kept) > 0 || len(r.Skipped) > 0 {
		if err := (ContextKeepResult{Kept: r.Kept, Skipped: r.Skipped}).FormatText(w); err != nil {
			return err
		}
	}
	for _, op := range r.Widened {
		if _, err := fmt.Fprintln(w, "Applied more widely "+op.line()+landedSuffix(op)); err != nil {
			return err
		}
	}
	for _, op := range r.Dropped {
		if _, err := fmt.Fprintln(w, "Dropped "+op.line()+landedSuffix(op)); err != nil {
			return err
		}
	}
	return nil
}

func landedSuffix(op ContextOperation) string {
	if op.Landed == "" {
		return ""
	}
	return " (" + op.Landed + ")"
}

// DecideContextReview records a round of review decisions: the choice first,
// then the drops, then the keeps, so a keep never meets a rival the same round
// drops.
func (a *App) DecideContextReview(ctx context.Context, req ContextReviewRequest) (ContextReviewResult, error) {
	out := ContextReviewResult{Kept: []ContextOperation{}, Widened: []ContextOperation{}, Dropped: []ContextOperation{}}
	if req.Empty() {
		return out, errors.New("review names something to decide: --keep, --drop or --choose")
	}
	if req.WidenTo != "" && len(req.Keep) == 0 {
		return out, errors.New("--widen-to applies what --keep names more widely: name it with --keep")
	}
	if req.Replacement != "" && len(req.Keep) != 1 {
		return out, errors.New("--use changes one rule as it is kept: name one --keep")
	}
	if req.Choose != "" {
		chosen, err := a.ChooseContextSide(ctx, ContextChooseRequest{
			Actor: req.Actor, Project: req.Project, ID: req.Choose, Replacement: req.Replacement, Note: req.Note,
		})
		if err != nil {
			return out, err
		}
		out.Chosen = &chosen
	}
	for _, id := range req.Drop {
		op, err := a.DropContextOperation(ctx, ContextDropRequest{Actor: req.Actor, Project: req.Project, ID: id, Note: req.Note})
		if err != nil {
			return out, err
		}
		out.Dropped = append(out.Dropped, op)
	}
	if len(req.Keep) == 1 && req.Keep[0] == ContextReviewAll {
		if req.Session == "" {
			return out, errors.New("--keep all keeps what one session suggested: name it with --session")
		}
		kept, err := a.KeepContextOperations(ctx, ContextKeepRequest{
			Actor: req.Actor, Project: req.Project, Session: req.Session,
			Advisory: req.Advisory, WidenTo: req.WidenTo, Note: req.Note,
		})
		out.Kept, out.Skipped = append(out.Kept, kept.Kept...), kept.Skipped
		return out, err
	}
	for _, id := range req.Keep {
		subject, err := a.ContextSubject(ctx, req.Project, id)
		if err != nil {
			return out, err
		}
		if subject.Established && subject.Status == contextop.StatusEstablished && req.WidenTo != "" {
			widened, werr := a.WidenContextOperation(ctx, ContextWidenRequest{
				Actor: req.Actor, Project: req.Project, ID: id, To: req.WidenTo, Note: req.Note,
			})
			if werr != nil {
				return out, werr
			}
			out.Widened = append(out.Widened, widened)
			continue
		}
		kept, err := a.KeepContextOperations(ctx, ContextKeepRequest{
			Actor: req.Actor, Project: req.Project, IDs: []string{id},
			Replacement: req.Replacement, Advisory: req.Advisory, WidenTo: req.WidenTo, Note: req.Note,
		})
		if err != nil {
			return out, err
		}
		out.Kept = append(out.Kept, kept.Kept...)
	}
	return out, nil
}

// ContextSubject reads the suggestion or rule an id names, as the log folds it
// now. An id naming a decision reaches the rule it decided.
func (a *App) ContextSubject(ctx context.Context, project, id string) (ContextOperation, error) {
	s, err := a.contextOps(ctx, project)
	if err != nil {
		return ContextOperation{}, err
	}
	r, err := s.ledger.Subject(ctx, id)
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: r}, nil
}

// OnlySession narrows a digest to what one agent session noticed: its
// conflicts, suggestions and rules. The numbers stay the project's.
func (d ContextDigest) OnlySession(session string) ContextDigest {
	if session == "" {
		return d
	}
	mine := func(it DigestItem) bool { return it.NoticedBy.Session == session }
	out := d
	out.Conflicts = nil
	for _, c := range d.Conflicts {
		if slices.ContainsFunc(c.Sides, mine) {
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	out.Established = nil
	for _, it := range d.Established {
		if mine(it) {
			out.Established = append(out.Established, it)
		}
	}
	out.Suggested = nil
	for _, t := range d.Suggested {
		theme := DigestTheme{Theme: t.Theme, Title: t.Title}
		for _, g := range t.Groups {
			group := DigestGroup{Collection: g.Collection}
			for _, it := range g.Items {
				if mine(it) {
					group.Items = append(group.Items, it)
				}
			}
			if len(group.Items) > 0 {
				theme.Groups = append(theme.Groups, group)
			}
		}
		if len(theme.Groups) > 0 {
			out.Suggested = append(out.Suggested, theme)
		}
	}
	out.Drift = nil
	for _, dr := range d.Drift {
		if mine(dr.Rule) {
			out.Drift = append(out.Drift, dr)
		}
	}
	return out
}
