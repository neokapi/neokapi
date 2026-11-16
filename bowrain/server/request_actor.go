package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
)

// toolNameKey is the echo context key handleAPIToken sets the tool's name
// under when a tool token authenticated the request.
const toolNameKey = "tool_name"

// toolTokenRefusedCode is the error code of a review action a tool token
// asked for.
const toolTokenRefusedCode = "tool_token_refused"

// The kinds of actor a request acts as.
const (
	actorKindPerson = "person"
	actorKindTool   = "tool"
)

// requestActor is who a request acts as, as the auth middleware resolved it.
// A client never names it.
type requestActor struct {
	// Kind is person or tool.
	Kind string
	// UserID is the signed-in member: the person, or the member who issued
	// the tool's token.
	UserID string
	// Tool is the tool's name, and TokenID the token it authenticated with.
	Tool    string
	TokenID string
}

// requestActorOf reads who the request acts as.
func requestActorOf(c echo.Context) requestActor {
	userID, _ := c.Get("user_id").(string)
	a := requestActor{Kind: actorKindPerson, UserID: userID}
	if tool, _ := c.Get(toolNameKey).(string); tool != "" {
		a.Kind, a.Tool = actorKindTool, tool
		a.TokenID, _ = c.Get("api_token_id").(string)
	}
	return a
}

// isTool reports whether the actor is a tool.
func (a requestActor) isTool() bool { return a.Kind == actorKindTool }

// pusher is the stamp a context object the actor pushes carries.
func (a requestActor) pusher() platstore.Pusher {
	return platstore.Pusher{UserID: a.UserID, Tool: a.Tool, TokenID: a.TokenID}
}

// contextActor is the actor a context operation the request records names: a
// person with no name, whom a reader names after the member who pushed the
// segment, or the tool. A reader fills in the member a tool ran for from the
// segment's stamp.
func (a requestActor) contextActor() contextop.Actor {
	if a.isTool() {
		return contextop.Actor{Kind: contextop.ActorTool, Name: a.Tool}
	}
	return contextop.Actor{Kind: contextop.ActorPerson}
}

// eventTool is the tool an event records, the zero value for a person.
func (a requestActor) eventTool() platev.Tool {
	return platev.Tool{Name: a.Tool, TokenID: a.TokenID}
}

// reviewDeniedToTools are the permissions a tool token never holds, whatever
// its scopes and its issuer's membership grant: approving and rejecting
// translations.
const reviewDeniedToTools = platauth.PermReview

// refuseTool wraps a review action so a tool token is refused it. A tool's
// work is reviewed by a person, and the member who issued the token reviews
// it as an agent's (contextop.BasisAgentAuthored), never through the token.
func (s *Server) refuseTool(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if a := requestActorOf(c); a.isTool() {
			s.emitAudit(c, auditEvent{
				Type:      platev.EventAuthzDenied,
				ProjectID: projectParam(c),
				Effect:    "deny",
				Data: map[string]string{
					"reason": "tool_token_review",
					"path":   c.Path(),
				},
			})
			return c.JSON(http.StatusForbidden, ErrorResponse{
				Error: "a tool token cannot review: tool " + a.Tool + " acts for its issuer, who reviews its work with their own sign-in",
				Code:  toolTokenRefusedCode,
			})
		}
		return next(c)
	}
}
