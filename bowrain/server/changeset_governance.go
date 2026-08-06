package server

import (
	"github.com/labstack/echo/v4"
)

// This file holds the rules that decide who authors and who may review a
// governed change-set.
//
// authorIdentity separates the author of a change from the account that carried
// it: a push mediated by the loop — CI through kapi-action, an agent-driven
// kapi — is authored by the machine. That is what makes the person whose
// workspace it is an eligible reviewer of what the loop proposes, which is the
// common one-person story.

// authorIdentity is the identity that work created on this request is recorded
// against.
//
// For an ordinary session or a personal API token this is the user ID, exactly
// as before. For a machine API token (one minted with an agent name) it is
// "agent/<name>" — the machine, not the person whose token it is.
//
// Authorization is deliberately not routed through here: every permission check
// still reads user_id. A machine can do no more than its token's owner can. The
// only thing that moves is the name on the work, and the consequence that
// matters is that the owner is not the author, so the owner can review it.
func authorIdentity(c echo.Context) string {
	if machine, _ := c.Get("author_identity").(string); machine != "" {
		return machine
	}
	actor, _ := c.Get("user_id").(string)
	return actor
}

// onBehalfOf names the accountable person when the request's author is a
// machine, and is empty otherwise. It is what keeps a machine identity from
// being an anonymous one: every machine-authored record can be traced back to
// the human who minted the token.
func onBehalfOf(c echo.Context) string {
	if machine, _ := c.Get("author_identity").(string); machine == "" {
		return ""
	}
	actor, _ := c.Get("user_id").(string)
	return actor
}
