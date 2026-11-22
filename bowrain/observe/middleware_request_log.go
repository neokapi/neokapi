package observe

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/neokapi/neokapi/bowrain/apierror"
)

// RequestLogConfig configures RequestLogMiddleware.
type RequestLogConfig struct {
	// DefaultLevel is the level of a request answered below 400.
	DefaultLevel slog.Level
	// ClientErrorLevel is the level of a request answered 4xx.
	ClientErrorLevel slog.Level
	// ServerErrorLevel is the level of a request answered 5xx.
	ServerErrorLevel slog.Level
	// SkipPaths are request paths (URL.Path, exact match) that are not logged.
	SkipPaths []string
}

// RequestLogMiddleware logs one structured record per request through logger.
//
// The record carries two groups and the request ID. CloudWatch metric filters
// and the monitoring dashboard read these keys, so they are a contract:
//
//	request:  time, method, host, path, query, params, route, referer, length
//	response: time, latency (a duration; nanoseconds in JSON), status, length
//	id:       the X-Request-ID of the request, or of the response
//
// route is the matched route pattern (c.Path()); length is the number of body
// bytes read from the request and written to the response. The record does not
// carry the client's address.
//
// A handler error that carries no HTTP status is answered here. It is wrapped
// as a 500 *echo.HTTPError and passed to the Echo error handler, so the record
// shows the status the client received. The wrapped error is then returned up
// the chain, where the error handler finds the response already committed.
// An HTTP error is returned unhandled and logged with its own status.
func RequestLogMiddleware(logger *slog.Logger, cfg RequestLogConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			start := time.Now()
			path := req.URL.Path
			query := req.URL.RawQuery

			// The route's parameters; none for a request no route or method
			// matched (a 404 or 405).
			params := map[string]string{}
			if c.RouteInfo().Name != echo.MethodNotAllowedRouteName {
				for _, p := range c.PathValues() {
					if p.Name != "" {
						params[p.Name] = p.Value
					}
				}
			}

			body := &countingReader{ReadCloser: req.Body}
			if req.Body != nil {
				req.Body = body
			}

			err := next(c)

			if err != nil {
				if _, ok := err.(echo.HTTPStatusCoder); !ok {
					err = echo.NewHTTPError(http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError)).Wrap(err)
					c.Echo().HTTPErrorHandler(c, err)
				}
			}

			if slices.Contains(cfg.SkipPaths, c.Request().URL.Path) {
				return err
			}

			end := time.Now()
			status, size := 0, int64(0)
			if r, rerr := echo.UnwrapResponse(c.Response()); rerr == nil {
				status, size = r.Status, r.Size
			}

			errMsg := err
			httpErr, isHTTPErr := apierror.AsHTTPError(err)
			if isHTTPErr {
				status = httpErr.Code
				errMsg = errors.New(httpErr.Message)
			}

			requestAttrs := []slog.Attr{
				slog.Time("time", start.UTC()),
				slog.String("method", req.Method),
				slog.String("host", req.Host),
				slog.String("path", path),
				slog.String("query", query),
				slog.Any("params", params),
				slog.String("route", c.Path()),
				slog.String("referer", req.Referer()),
				slog.Int("length", body.n),
			}
			responseAttrs := []slog.Attr{
				slog.Time("time", end.UTC()),
				slog.Duration("latency", end.Sub(start)),
				slog.Int("status", status),
				slog.Int64("length", size),
			}
			attrs := []slog.Attr{
				{Key: "request", Value: slog.GroupValue(requestAttrs...)},
				{Key: "response", Value: slog.GroupValue(responseAttrs...)},
			}
			if id := requestIDOf(c); id != "" {
				attrs = append(attrs, slog.String("id", id))
			}

			level := cfg.DefaultLevel
			msg := "Incoming request"
			switch {
			case status >= http.StatusInternalServerError:
				level = cfg.ServerErrorLevel
				msg = failureMessage(errMsg, status)
			case status >= http.StatusBadRequest:
				level = cfg.ClientErrorLevel
				msg = failureMessage(errMsg, status)
			}

			if isHTTPErr {
				internal := httpErr.Unwrap()
				attrs = append(attrs, slog.Any("error", map[string]any{
					"code":     httpErr.Code,
					"message":  httpErr.Message,
					"internal": internal,
				}))
				if internal != nil {
					attrs = append(attrs, slog.String("internal", internal.Error()))
				}
			}

			logger.LogAttrs(c.Request().Context(), level, msg, attrs...)
			return err
		}
	}
}

// failureMessage is the record's message for a 4xx or 5xx: the error's text,
// or the status text when the handler wrote the status without an error.
func failureMessage(err error, status int) string {
	if err != nil {
		return err.Error()
	}
	return http.StatusText(status)
}

// requestIDOf is the request's X-Request-ID header, or the response's when the
// request carried none.
func requestIDOf(c *echo.Context) string {
	if id := c.Request().Header.Get(echo.HeaderXRequestID); id != "" {
		return id
	}
	return c.Response().Header().Get(echo.HeaderXRequestID)
}

// countingReader counts the bytes read through it.
type countingReader struct {
	io.ReadCloser
	n int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.n += n
	return n, err
}
