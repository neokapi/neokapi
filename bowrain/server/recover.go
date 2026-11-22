package server

import (
	"errors"
	"log/slog"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

// recoverPanics recovers a panic in the rest of the chain, logs its stack at
// WARN, and answers it through the Echo error handler with the panic value
// as the error, so the handler reports a 500 to Sentry under the panic's own
// message. It then returns nil: the middleware before it (tracing, metrics,
// the request log) see a committed 500 and no error.
func recoverPanics() echo.MiddlewareFunc {
	recoverMW := middleware.Recover()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		h := recoverMW(next)
		return func(c *echo.Context) error {
			err := h(c)
			pse, ok := errors.AsType[*middleware.PanicStackError](err)
			if !ok {
				return err
			}
			// WARN: httpErrorHandler logs the same failure at ERROR and reports
			// it to Sentry, and the server_error_count alarm counts ERROR lines.
			// This line adds the stack without counting the panic twice.
			slog.WarnContext(c.Request().Context(), "panic recovered",
				"method", c.Request().Method,
				"path", c.Path(),
				"error", pse.Err,
				"stack", string(pse.Stack),
			)
			c.Echo().HTTPErrorHandler(c, pse.Err)
			return nil
		}
	}
}
