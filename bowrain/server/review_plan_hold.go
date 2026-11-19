package server

import (
	"context"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// The plan decides what a language's review deck holds. Under involvement
// context or interfaces, a translation reviewed before its terms are decided
// is reviewed twice, so while the project's key language has open decisions
// the deck deals only what the plan leaves for people at that point: the
// exceptions (the translations a check fails, the deck's desk group) and the
// page sample. The deck holds the other translations back, and the deck, its
// collections, the plan and the review inbox report them as one count with a
// reason instead of as cards. Under involvement every nothing is held.
//
// Exceptions stay reviewable while the key language is open: a failing check
// names a rule, and a reviewer can decide that rule whatever terms are still
// open.

// heldKeyLanguage is the reason translations are held while the project's
// key language has open decisions.
const heldKeyLanguage = "key_language"

// deckHeld is the translations a deck holds back, and why.
type deckHeld struct {
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

// translationsHeld reports whether the project's language decks hold their
// translations back: the involvement is not every and the key language has
// open decisions, counted for the whole project as a settle pass counts them.
// It reads the project's context deck and the workspace's concepts once.
func (s *Server) translationsHeld(ctx context.Context, ws *platauth.Workspace, proj *platstore.Project) (bool, error) {
	if ws == nil || involvementOf(proj) == involvementEvery {
		return false, nil
	}
	key, err := s.planKeyLanguage(ctx, ws, proj, "", 0, nil)
	if err != nil {
		return false, err
	}
	policy, err := s.planPolicy(ctx, ws, proj, involvementOf(proj), key.Untranslated)
	if err != nil {
		return false, err
	}
	return translationsHeldBy(policy), nil
}

// translationsHeldBy reports whether a plan's policy holds translations
// back: the key language has open decisions and the involvement leaves part
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
