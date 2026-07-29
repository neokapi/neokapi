package server

import (
	"github.com/labstack/echo/v4"
)

// Response headers this server sets on everything it serves.
const (
	headerCSP         = "Content-Security-Policy"
	headerNosniff     = "X-Content-Type-Options"
	headerFrameOpts   = "X-Frame-Options"
	headerReferrer    = "Referrer-Policy"
	headerHSTS        = "Strict-Transport-Security"
	headerPermissions = "Permissions-Policy"
)

// defaultCSP governs what this server itself returns: JSON for the API, and the
// handful of static pages the OIDC callbacks render. None of it should ever
// load a script, an image, a frame or a font, so the policy starts from nothing
// and adds back only inline styles — the callback pages carry a style attribute,
// and a style cannot execute.
//
// It deliberately does NOT govern the web application. The SPAs are built in CI
// and served by whatever fronts them — S3 behind CloudFront in the hosted
// deployment, a static file server in a self-hosted one — so a header set here
// is not on the response that delivers the document that runs the app. That
// policy has to be set where the SPA is actually served. The Go server serves
// the SPA only in development and E2E (Config.WebUIDir), and serveSPAFile
// clears the policy there so every environment agrees — see the comment on
// securityHeaders for why applying it there would be worse than leaving it off.
const defaultCSP = "default-src 'none'; " +
	"style-src 'unsafe-inline'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'; " +
	"form-action 'none'"

// sandboxCSP is served with document-derived HTML — a reader-generated preview,
// a stored block's markup. That content is not trusted: it comes from an
// uploaded document, or from a translator, or through a connector from an
// external CMS, and the preview pipeline writes it verbatim by design, because
// reproducing an HTML document faithfully is the whole point of the feature.
//
// Escaping it would defeat that feature, so the fix is to take away what the
// markup could do rather than change the markup. A sandbox directive with no
// tokens puts the response in a unique opaque origin with scripts, forms and
// same-origin access all withdrawn, which is what a direct navigation to one of
// these URLs deserves.
//
// The intended consumer is unaffected: the editor fetches the HTML and injects
// it into an iframe via srcDoc with sandbox="allow-scripts" and no
// allow-same-origin, so it is already isolated from the app origin, and
// response headers play no part in that path.
const sandboxCSP = "sandbox"

// securityHeaders sets the response headers that hold for everything this
// server returns.
//
// The stack had none of these. The policy here is deliberately conservative
// about the one header that can break an application silently: the CSP applies
// to the server's own responses and is cleared for the SPA, because a policy
// that stops the editor's preview from rendering — it inherits the embedder's
// CSP through srcDoc, and fails without an error — would cost more than it
// buys. Tightening the application's own policy is a change to the CDN
// configuration, where the document is actually served.
func securityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()

			// Never let a response be reinterpreted as a type it did not
			// declare — the JSON API's main exposure to content sniffing.
			h.Set(headerNosniff, "nosniff")

			// Nothing here is meant to be embedded. frame-ancestors in the CSP
			// says the same thing to modern browsers; this covers the rest.
			h.Set(headerFrameOpts, "DENY")

			// Send the full URL only to ourselves; an off-origin destination
			// sees the origin alone. Workspace and project identifiers live in
			// our paths.
			h.Set(headerReferrer, "strict-origin-when-cross-origin")

			// No handler asks for these, so decline them rather than let an
			// embedded document inherit the ability to ask.
			h.Set(headerPermissions, "camera=(), microphone=(), geolocation=(), payment=()")

			// Only meaningful over TLS, and asserting it on a plain-HTTP
			// development request would be noise. c.Scheme honours
			// X-Forwarded-Proto, so this fires behind a terminating proxy.
			// includeSubDomains is left off deliberately: this server cannot
			// know that every sibling host is HTTPS-only, and the header would
			// bind them all.
			if c.Scheme() == "https" {
				h.Set(headerHSTS, "max-age=31536000")
			}

			h.Set(headerCSP, defaultCSP)

			return next(c)
		}
	}
}

// clearCSPForSPA removes the server's own content policy from a response that
// carries the web application.
//
// The application needs a real policy, but not this one, and not from here: the
// document is served by a CDN or static file server that this process does not
// configure. Leaving defaultCSP on it would break the app in development and
// E2E only — the one place the header applies — while protecting nothing where
// the app actually runs. A policy authored where the app is served is worth
// more than one enforced only where it can do damage.
func clearCSPForSPA(c echo.Context) {
	c.Response().Header().Del(headerCSP)
}

// applySandboxCSP marks a response as untrusted document-derived HTML.
func applySandboxCSP(c echo.Context) {
	c.Response().Header().Set(headerCSP, sandboxCSP)
}
