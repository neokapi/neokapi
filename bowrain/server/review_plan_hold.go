package server

import (
	"context"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// The plan decides what a language's review deck holds. A translation
// reviewed before its terms are decided is reviewed twice. So under
// involvement context or interfaces, while the context has open
// decisions, the deck deals only the exceptions (its desk group) and the page
// sample. The other translations are held, and the deck, its collections, the
// plan and the inbox count them with a reason instead of dealing them. In a
// collection under involvement every, by the project's setting or an
// override at its point, nothing is held.

// Exceptions stay reviewable: each names a rule a reviewer can decide
// whatever terms are still open.

// heldContext is the reason translations are held while the project's
// context has open decisions.
const heldContext = "context"

// deckHeld is the translations a deck holds back, and why.
type deckHeld struct {
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

// translationsHeld reports whether the project's language decks hold their
// translations back: some point of the project is not under every and the
// context has open decisions, counted for the whole project as a settle pass
// counts them. It reads the project's context deck and the workspace's
// concepts once. A hold reaches only the collections not under every
// (projectInvolvement.heldIn).
func (s *Server) translationsHeld(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project, pi projectInvolvement) (bool, error) {
	if ws == nil || pi.allEvery() {
		return false, nil
	}
	counts, err := s.planContextCounts(ctx, ws, proj, "", 0, nil)
	if err != nil {
		return false, err
	}
	policy, err := s.planPolicy(ctx, ws, proj, pi, counts.Untranslated)
	if err != nil {
		return false, err
	}
	return translationsHeldBy(policy), nil
}

// translationsHeldBy reports whether a plan's policy holds translations
// back: the context has open decisions and the involvement leaves part
// of the work to the checks.
func translationsHeldBy(p planPolicyDTO) bool {
	return p.Settle == settleFirstPass || p.Settle == settleHeld
}

// held moves the translations a deck holds back out of what it deals:
// everything outside the desk group (a failing check or a stale approval).
// A translation not classified yet is held until a deck request classifies
// it and finds a failing check.
func (w deckWaiting) held() deckWaiting {
	w.Held = w.Waiting - w.Desk
	w.Waiting = w.Desk
	w.Fast, w.Closer, w.Unclassified = 0, 0, 0
	return w
}

// requestWorkspace is the workspace a project route resolved, or nil.
func (s *Server) requestWorkspace(ctx context.Context, wsID string) *platauth.Workspace {
	if s.AuthStore == nil || wsID == "" {
		return nil
	}
	ws, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil {
		return nil
	}
	return ws
}
