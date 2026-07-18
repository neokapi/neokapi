package observe

import (
	"log/slog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const RequestIDHeader = "X-Request-ID"

// RequestIDMiddleware generates or propagates a unique request ID per request.
// The ID is set in the response header and stored in the context as a child
// slog logger with "request_id" pre-attached.
func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqID := c.Request().Header.Get(RequestIDHeader)
			if reqID == "" {
				reqID = uuid.New().String()
			}
			c.Response().Header().Set(RequestIDHeader, reqID)
			c.Set("request_id", reqID)

			// Store the ID as a plain context value so ContextHandler stamps it
			// onto every *Context log record, and keep the child logger for
			// callers that use observe.Logger(ctx) directly.
			ctx := WithRequestID(c.Request().Context(), reqID)
			childLogger := slog.Default().With("request_id", reqID)
			ctx = WithLogger(ctx, childLogger)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}
