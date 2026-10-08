package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// Resetting a project's context (AD C-11).
//
// A reset rewinds the context to how it stood at a point in its history: before
// an agent session started, before a moment, or before one operation. Every
// context operation from that point on is set aside. It stays in the log, and
// the project's stores are rebuilt from what the log holds before the point and
// after the reset. The reset is itself an operation, so a later reset before it
// sets it aside and brings back what it set aside.
//
// There is no undo for one decision. Changing your mind about a rule is a new
// decision in review; a reset is for going back to a known good state.

// ContextResetRequest rewinds a project's context.
type ContextResetRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go). Only a person resets.
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// Before names the point to go back to: an agent session (the context as it
	// stood before the session's first operation), a date (2006-01-02) or an
	// instant (RFC 3339), or an operation id or an unambiguous start of one,
	// such as an earlier reset's.
	Before string
	Note   string
}

// ContextResetResult is what a reset did, or what it would do.
type ContextResetResult struct {
	// Reset is the reset operation recorded, empty for a preview.
	Reset *ContextOperation `json:"reset,omitempty"`
	// Before is the point the context goes back to, as the request named it,
	// and Point the first operation id it sets aside.
	Before string `json:"before"`
	Point  string `json:"point"`
	// SetAside are the subject-bearing operations that stop answering:
	// suggestions and rules recorded after the point.
	SetAside []ContextOperation `json:"set_aside"`
	// Decisions counts the other operations set aside: keeps, drops, widenings
	// and the rest of what acted on a rule after the point.
	Decisions int `json:"decisions"`
	// Restored are the suggestions and rules an earlier reset had set aside
	// that answer again, because this reset goes back to before that one.
	Restored []ContextOperation `json:"restored,omitempty"`
	// Rebuild is the rebuild of the project's stores that followed.
	Rebuild *ContextRebuild `json:"rebuild,omitempty"`
}

// FormatText renders a reset.
func (r ContextResetResult) FormatText(w io.Writer) error {
	verb, restore := "Set aside", "Brought back"
	if r.Reset == nil {
		verb, restore = "Would set aside", "Would bring back"
	}
	what := pluralUnit(len(r.SetAside), "suggestion or rule", "suggestions and rules")
	if r.Decisions > 0 {
		what += " and " + pluralUnit(r.Decisions, "decision", "decisions")
	}
	if _, err := fmt.Fprintf(w, "%s %s recorded since %s.\n", verb, what, r.Before); err != nil {
		return err
	}
	for _, op := range r.SetAside {
		if _, err := fmt.Fprintln(w, "  "+op.line()); err != nil {
			return err
		}
	}
	if len(r.Restored) > 0 {
		if _, err := fmt.Fprintf(w, "%s %s an earlier reset set aside.\n", restore, pluralUnit(len(r.Restored), "suggestion or rule", "suggestions and rules")); err != nil {
			return err
		}
		for _, op := range r.Restored {
			if _, err := fmt.Fprintln(w, "  "+op.line()); err != nil {
				return err
			}
		}
	}
	if r.Reset != nil {
		if _, err := fmt.Fprintf(w, "Recorded as reset #%s; `kapi context reset --before %s` takes it back.\n",
			r.Reset.Short, r.Reset.Short); err != nil {
			return err
		}
	}
	return nil
}

// ResetContext rewinds the project's context to how it stood at the point
// req.Before names, then rebuilds the project's stores from the log.
func (a *App) ResetContext(ctx context.Context, req ContextResetRequest) (ContextResetResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextResetResult{}, err
	}
	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return ContextResetResult{}, err
	}
	out, err := s.resetPlan(ctx, req.Before)
	if err != nil {
		return ContextResetResult{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   actor,
		Kind:    contextop.KindReset,
		Project: s.key,
		Before:  out.Point,
		Note:    note,
	})
	if err != nil {
		return ContextResetResult{}, teachRefusal(err)
	}
	out.Reset = &ContextOperation{Record: written}
	rebuilt, err := a.RebuildProjectContext(ctx, s.recipe, false)
	if err != nil {
		return out, fmt.Errorf("the reset is recorded, and rebuilding the stores from the log failed (`kapi store rebuild` tries again): %w", err)
	}
	out.Rebuild = &rebuilt
	return out, nil
}

// ContextResetScope reports what resetting to req.Before would set aside,
// without recording anything.
func (a *App) ContextResetScope(ctx context.Context, req ContextResetRequest) (ContextResetResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextResetResult{}, err
	}
	return s.resetPlan(ctx, req.Before)
}

// resetPlan resolves the point a reset goes back to and works out what it
// sets aside, by reading the log as it would stand with the reset appended.
func (s *contextOpsSession) resetPlan(ctx context.Context, before string) (ContextResetResult, error) {
	before = strings.TrimSpace(before)
	if before == "" {
		return ContextResetResult{}, errors.New("name the point to go back to: a session id, a date (2006-01-02), an instant, or an operation id")
	}
	held, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key})
	if err != nil {
		return ContextResetResult{}, err
	}
	slices.Reverse(held) // oldest first
	point, err := s.resetPoint(ctx, held, before)
	if err != nil {
		return ContextResetResult{}, err
	}

	ops, err := s.ws.Select(ctx, workspace.OpQuery{Project: s.key, KindPrefix: contextop.OpKindPrefix})
	if err != nil {
		return ContextResetResult{}, err
	}
	last := ""
	for _, op := range ops {
		last = max(last, op.ID)
	}
	if point > last {
		return ContextResetResult{}, fmt.Errorf("nothing was recorded in this project's context since %s", before)
	}
	payload := fmt.Appendf(nil, `{"before":%q}`, point)
	planned := append(slices.Clone(ops), workspace.Op{
		ID: workspace.NewOpID(time.Now(), last), Project: s.key, Kind: workspace.OpContextReset, Payload: payload,
	})
	aside := workspace.SetAside(planned)

	out := ContextResetResult{Before: before, Point: point, SetAside: []ContextOperation{}}
	for _, r := range held {
		switch {
		case aside[r.ID] && r.Status != contextop.StatusReset:
			if r.Kind.Bears() {
				out.SetAside = append(out.SetAside, ContextOperation{Record: r})
			} else {
				out.Decisions++
			}
		case !aside[r.ID] && r.Status == contextop.StatusReset && r.Kind.Bears():
			out.Restored = append(out.Restored, ContextOperation{Record: r})
		}
	}
	return out, nil
}

// resetPoint resolves what a reset names into the first operation id it sets
// aside. A date or an instant is the earliest id at that moment, so every
// operation accepted from then on is set aside, whatever wrote it. A session
// is its first operation. Anything else is an operation id.
func (s *contextOpsSession) resetPoint(ctx context.Context, held []contextop.Record, before string) (string, error) {
	if at, err := parseResetTime(before); err == nil {
		return workspace.OpIDAt(at), nil
	}
	for _, r := range held {
		if r.Actor.Session != "" && r.Actor.Session == before {
			return r.ID, nil
		}
	}
	r, err := s.ledger.Get(ctx, before)
	if err != nil {
		if errors.Is(err, contextop.ErrNotFound) {
			return "", fmt.Errorf("%q names no session, date or operation in this project's context", before)
		}
		return "", err
	}
	if r.Project != s.key {
		return "", fmt.Errorf("operation %s belongs to another project", r.Short)
	}
	return r.ID, nil
}

// parseResetTime reads a reset point as a bare date (midnight UTC) or an
// instant.
func parseResetTime(value string) (time.Time, error) {
	if at, err := time.Parse("2006-01-02", value); err == nil {
		return at, nil
	}
	return time.Parse(time.RFC3339, value)
}
