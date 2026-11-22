package server

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// corsMiddleware is Echo's CORS middleware for cfg, answering two cases the
// way the server's clients have always seen them:
//
//   - With no allowed origin and no origin function (production with
//     AppPublicURL and PublicSiteURL unset), every origin is answered with
//     Access-Control-Allow-Origin "*". A browser does not send credentials to
//     a "*" origin, so no cross-origin caller is credentialed.
//   - Every OPTIONS request is answered as a preflight with 204 No Content,
//     before route middleware such as authentication runs. Without an Origin
//     header the response carries the router's Allow header and no CORS
//     headers; with one, the origin is checked as for any preflight.
func corsMiddleware(cfg middleware.CORSConfig) echo.MiddlewareFunc {
	if cfg.UnsafeAllowOriginFunc == nil && len(cfg.AllowOrigins) == 0 {
		cfg.UnsafeAllowOriginFunc = func(*echo.Context, string) (string, bool, error) {
			return "*", true, nil
		}
	}
	cors := middleware.CORSWithConfig(cfg)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		h := cors(next)
		return func(c *echo.Context) error {
			req := c.Request()
			if req.Method != http.MethodOptions || req.Header.Get(echo.HeaderAccessControlRequestMethod) != "" {
				return h(c)
			}
			if req.Header.Get(echo.HeaderOrigin) == "" {
				res := c.Response()
				res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
				if allow, ok := c.Get(echo.ContextKeyHeaderAllow).(string); ok && allow != "" {
					res.Header().Set(echo.HeaderAllow, allow)
				}
				return c.NoContent(http.StatusNoContent)
			}
			// Echo answers an OPTIONS request as a preflight only when it
			// names the method it asks about. The preflight answer does not
			// depend on that value, so name OPTIONS itself.
			preflight := req.Clone(req.Context())
			preflight.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodOptions)
			c.SetRequest(preflight)
			return h(c)
		}
	}
}
