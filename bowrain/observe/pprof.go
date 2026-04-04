package observe

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"
)

// StartPprofServer conditionally starts a separate HTTP server for /debug/pprof/*
// endpoints. Enabled only when BOWRAIN_PPROF_ENABLED=true.
//
// The server binds to localhost only (127.0.0.1:6060 by default) so it is never
// reachable from external networks. Override the port with BOWRAIN_PPROF_PORT.
//
// Returns a shutdown function that should be called during graceful shutdown.
// Returns nil if pprof is disabled.
func StartPprofServer(ctx context.Context) (shutdown func(context.Context) error) {
	if os.Getenv("BOWRAIN_PPROF_ENABLED") != "true" {
		return nil
	}

	port := os.Getenv("BOWRAIN_PPROF_PORT")
	if port == "" {
		port = "6060"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	addr := net.JoinHostPort("127.0.0.1", port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info(fmt.Sprintf("pprof server listening on %s (localhost only)", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("pprof server error", "error", err)
		}
	}()

	return srv.Shutdown
}
