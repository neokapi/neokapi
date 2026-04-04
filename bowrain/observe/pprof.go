package observe

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/labstack/echo/v4"
)

// RegisterPprof conditionally registers /debug/pprof/* endpoints on the Echo
// instance. Enabled only when BOWRAIN_PPROF_ENABLED=true.
//
// Security: these endpoints expose runtime internals. In production, ensure
// they are not reachable from external traffic (e.g. block /debug/* at the
// reverse proxy).
func RegisterPprof(e *echo.Echo) {
	if os.Getenv("BOWRAIN_PPROF_ENABLED") != "true" {
		return
	}

	g := e.Group("/debug/pprof")
	g.GET("/", echo.WrapHandler(http.HandlerFunc(pprof.Index)))
	g.GET("/cmdline", echo.WrapHandler(http.HandlerFunc(pprof.Cmdline)))
	g.GET("/profile", echo.WrapHandler(http.HandlerFunc(pprof.Profile)))
	g.GET("/symbol", echo.WrapHandler(http.HandlerFunc(pprof.Symbol)))
	g.GET("/trace", echo.WrapHandler(http.HandlerFunc(pprof.Trace)))
	g.GET("/:name", func(c echo.Context) error {
		pprof.Handler(c.Param("name")).ServeHTTP(c.Response(), c.Request())
		return nil
	})

	slog.Info("pprof endpoints enabled at /debug/pprof/")
}
