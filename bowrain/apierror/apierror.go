// Package apierror is the single definition of the Bowrain API error envelope.
//
// Every API error response is a JSON object of the form
//
//	{
//	  "error":     "<stable code or existing error string>",  // unchanged wire contract
//	  "message":   "<plain factual English sentence>",        // additive, for humans
//	  "reference": "<request id>",                            // additive, correlation
//	  ...structured detail fields (e.g. "current", "limit")   // unchanged wire contract
//	}
//
// The "error" field keeps its exact historical value for every existing error —
// clients and tests parse it — while "message" and "reference" are additive.
// "reference" reuses the per-request correlation ID minted by
// observe.RequestIDMiddleware (the X-Request-ID header value that is stamped on
// every log line and Sentry event for the request), so one quoted ID drills
// from a user report down to logs.
//
// Handlers in bowrain/server write the envelope through the package-level
// helper (server's apiErr); bowrain/billing's guards call Write directly. The
// server's central Echo HTTPErrorHandler is the backstop that stamps the same
// envelope onto echo.NewHTTPError returns and panics.
package apierror

import (
	"fmt"
	"maps"

	"github.com/labstack/echo/v4"
)

// Stable codes for a dependency the platform declined to call because its
// circuit breaker is open. They are distinct from a generic 5xx on purpose: the
// request did not fail, it was not attempted, and the work will succeed once
// the upstream returns — so the UI shows a wait, not a failure.
const (
	// CodeAIUnavailable is the AI/LLM provider being circuit-broken. Detail
	// fields: "dependency", "retry_after_seconds".
	CodeAIUnavailable = "ai_unavailable"
	// CodeDependencyUnavailable is any other circuit-broken upstream (a CMS
	// connector, the forge API, the mail relay). Same detail fields.
	CodeDependencyUnavailable = "dependency_unavailable"
)

// RequestID returns the per-request correlation ID set by
// observe.RequestIDMiddleware, or "" when the middleware did not run (tests,
// non-HTTP contexts).
func RequestID(c echo.Context) string {
	if v, ok := c.Get("request_id").(string); ok {
		return v
	}
	return ""
}

// Write writes the standard error envelope. code goes into "error" verbatim
// (the stable wire contract); the human sentence for "message" comes from
// MessageFor; "reference" is the request's correlation ID. Optional detail
// maps are merged in at the top level, so historical payloads such as
// {"error":"project_limit_reached","current":2,"limit":1} keep their fields
// byte-for-byte while gaining "message" and "reference".
func Write(c echo.Context, status int, code string, details ...map[string]any) error {
	var detail map[string]any
	if len(details) > 0 {
		detail = details[0]
	}
	body := make(map[string]any, len(detail)+3)
	maps.Copy(body, detail)
	body["error"] = code
	body["message"] = MessageFor(code, detail)
	if ref := RequestID(c); ref != "" {
		body["reference"] = ref
	}
	return c.JSON(status, body)
}

// messages maps a stable error code — or, for handlers whose historical
// "error" value is itself a sentence, that exact string — to the human
// sentence served in "message". Each code's message is defined once, here.
var messages = map[string]string{
	// Auth and onboarding.
	"not authenticated":                     "You are not signed in. Sign in and try again.",
	"signups are currently closed":          "New signups are currently closed on this server.",
	"invalid_grant":                         "The device code is invalid or has expired. Start the sign-in again.",
	"authorization_pending":                 "The sign-in has not been approved in the browser yet.",
	"slug is required":                      "A workspace URL (slug) is required.",
	"name is required":                      "A name is required.",
	"slug is already in use":                "That workspace URL is already taken. Choose a different one.",
	"slug is reserved from a recent rename": "That workspace URL was recently in use and is temporarily reserved. Choose a different one.",
	"that email is already in use":          "That email address is already in use by another account.",
	"user not found":                        "The user account could not be found.",

	// Step-up / passkeys.
	"elevation_required":                   "This action requires a recent security verification. Verify again to continue.",
	"account_console_required":             "Passkeys for this account are managed in the identity provider's account console.",
	"passkey management is not configured": "Passkey management is not configured on this server.",

	// Claim.
	"claim_token is required":                       "A claim token is required.",
	"name and default_source_language are required": "A project name and a default source language are required.",

	// GitHub App setup.
	"the server has no GitHub App configured":         "This server has no GitHub App configured, so GitHub setup is unavailable.",
	"installation id must be numeric":                 "The GitHub installation id in the URL must be numeric.",
	"the installation does not cover that repository": "The GitHub App installation does not include that repository. Grant the app access to it on GitHub and try again.",
	"repository and project_id are required":          "A repository and a project are required.",
	"installation not found":                          "That GitHub installation is not connected to this workspace. Install the app from this workspace to connect it.",
	"the GitHub setup link is no longer valid":        "This GitHub setup link has expired. Start the install again from this workspace.",

	// Server components not configured.
	"auth not configured":          "Authentication is not configured on this server.",
	"store not configured":         "The server's data store is not configured.",
	"content store not configured": "The server's content store is not configured.",
	"brand voice not configured":   "Brand voice features are not configured on this server.",
	"brand profile not found":      "The brand profile could not be found.",
}

// MessageFor returns the human sentence for a stable error code. Codes whose
// sentence embeds detail values (limits, plans, reset times) are formatted
// here; everything else resolves through the messages table. Unknown codes
// fall back to the code string itself, so "message" is always present.
func MessageFor(code string, details map[string]any) string {
	switch code {
	case "project_limit_reached":
		if limit, ok := details["limit"]; ok {
			return fmt.Sprintf("This workspace's plan allows %v project(s) and it already has %v.",
				limit, detailOr(details, "current", limit))
		}
		return "This workspace has reached its plan's project limit."
	case "seat_limit_reached":
		if limit, ok := details["limit"]; ok {
			return fmt.Sprintf("This workspace's plan allows %v seat(s) and it already has %v.",
				limit, detailOr(details, "current", limit))
		}
		return "This workspace has reached its plan's seat limit."
	case "upgrade_required":
		if p, ok := details["minimum_plan"]; ok {
			return fmt.Sprintf("This workspace's plan does not include this feature; it requires the %v plan or higher.", p)
		}
		return "This workspace's plan does not include this feature."
	case "credits_exhausted":
		if at, ok := details["resets_at"]; ok {
			return fmt.Sprintf("This workspace has used its weekly credits; they reset at %v.", at)
		}
		return "This workspace has used its weekly credits for this week."
	case CodeAIUnavailable:
		if s, ok := details["retry_after_seconds"]; ok {
			return fmt.Sprintf("The AI service is temporarily unavailable. This work is queued and will be retried automatically in about %v seconds.", s)
		}
		return "The AI service is temporarily unavailable. This work is queued and will be retried automatically."
	case CodeDependencyUnavailable:
		if d, ok := details["dependency"]; ok {
			return fmt.Sprintf("The %v service is temporarily unavailable. This work is queued and will be retried automatically.", d)
		}
		return "An external service is temporarily unavailable. This work is queued and will be retried automatically."
	}
	if m, ok := messages[code]; ok {
		return m
	}
	return code
}

// detailOr returns details[key] when present, else the fallback.
func detailOr(details map[string]any, key string, fallback any) any {
	if v, ok := details[key]; ok {
		return v
	}
	return fallback
}
