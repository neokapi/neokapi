package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// enforceCSRFForCookie requires the CSRF header on a state-changing request that
// authenticated via the session cookie. On a missing header it writes a 403 and
// returns errAccessDenied (a NON-nil error) so the caller aborts before
// dispatching — echo's c.JSON returns nil on success, so returning that would
// fail open (the middleware would proceed to next). It returns nil for safe
// methods and when the header is present. Call it from the cookie branch of the
// auth middlewares, before dispatching to the handler.

// csrfHeaderName is a custom request header the browser SPA sends on every
// cookie-authenticated API call. Requiring it on state-changing requests
// defeats cross-site request forgery: a cross-origin page cannot attach a
// custom header to a "simple" request, and adding one forces a CORS preflight
// that the server's allowlist rejects (the header is not in AllowHeaders and a
// foreign Origin is not allowed). Same-origin SPA calls send it freely.
//
// This is defense-in-depth on top of the SameSite=Lax/Strict session cookies:
// it additionally covers same-site (sibling-subdomain) contexts and any legacy
// browser that ignores SameSite, and it closes any accidental state-changing
// GET. It applies ONLY to the cookie auth path — Bearer JWT and bwt_ API-token
// requests are not ambient-credential CSRF vectors (an attacker's page cannot
// read the token to forge the Authorization header), so they are exempt.
const csrfHeaderName = "X-Bowrain-Csrf"

// csrfSafeMethod reports whether the HTTP method is side-effect-free per
// RFC 7231 and therefore exempt from the CSRF header requirement.
func csrfSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func enforceCSRFForCookie(c echo.Context) error {
	if csrfSafeMethod(c.Request().Method) {
		return nil
	}
	if c.Request().Header.Get(csrfHeaderName) == "" {
		return deny(c, "missing CSRF header")
	}
	return nil
}
