//go:build !js

package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/engineserve"
	"github.com/neokapi/neokapi/host/pluginhost"
	"google.golang.org/grpc"
)

// RunEngineServe serves the EngineService: by default it binds a Unix
// socket, prints the handshake, and serves until the command context is
// cancelled (SIGINT/SIGTERM); with stdio it serves exactly one gRPC
// connection over the process's stdin/stdout instead.
func (a *App) RunEngineServe(cmd Command, socket string, stdio bool) error {
	if stdio && socket != "" {
		return errors.New("--stdio and --socket are mutually exclusive")
	}
	a.InitRegistries()
	// Plugin-provided formats route through their Mode-C daemons
	// transparently once the plugin host has wired the registry.
	a.InitPluginHost()

	gs := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(gs, engineserve.New(a.FormatReg, a.ToolReg))

	if stdio {
		return serveEngineStdio(cmd, gs)
	}

	if socket == "" {
		dir, err := defaultEngineSocketDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create socket dir: %w", err)
		}
		// MkdirAll does not tighten an existing directory; the 0700 dir is
		// the trust boundary for the socket, so enforce it explicitly.
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("restrict socket dir: %w", err)
		}
		socket = filepath.Join(dir, fmt.Sprintf("engine-%d.sock", os.Getpid()))
	}
	// Unix socket paths are limited to ~104 bytes (sockaddr_un); a longer
	// path fails at bind(2) with a cryptic "invalid argument".
	if len(socket) > 100 {
		return fmt.Errorf("socket path too long (%d bytes, unix limit ~104): %s. Pass a shorter --socket", len(socket), socket)
	}
	ctx := cmd.Context()
	// A stale socket file from a dead server would fail the bind; if nothing
	// answers on it, clear it.
	if _, err := os.Stat(socket); err == nil {
		var probe net.Dialer
		if conn, derr := probe.DialContext(ctx, "unix", socket); derr == nil {
			conn.Close()
			return fmt.Errorf("socket %s is already in use", socket)
		}
		_ = os.Remove(socket)
	}

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socket, err)
	}
	defer os.Remove(socket)

	// One-line JSON handshake on stdout — the same envelope a Mode-C plugin
	// daemon prints (cli/pluginhost.Handshake), so callers can share the
	// spawn-and-parse logic.
	hs, err := json.Marshal(pluginhost.Handshake{Socket: socket, Version: version.Version, PID: os.Getpid()})
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(hs))

	done := make(chan error, 1)
	go func() { done <- gs.Serve(lis) }()

	select {
	case <-ctx.Done():
		gs.GracefulStop()
		<-done
		return nil
	case err := <-done:
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

// serveEngineStdio serves exactly one gRPC connection over the process's
// stdin/stdout — the spawn-per-session transport for callers that exec kapi
// and speak gRPC over the pipes. stdout is exclusively the gRPC byte
// stream, so the handshake goes to stderr (as must any logging); when stdin
// reaches EOF the connection closes, the serve loop ends, and the command
// returns cleanly.
func serveEngineStdio(cmd Command, gs *grpc.Server) error {
	ctx := cmd.Context()
	lis := newStdioListener(os.Stdin, os.Stdout)

	// The socket-mode handshake exists to hand the caller the socket path;
	// over stdio the caller already holds both pipe ends, so this stderr
	// line is informational only (version + pid for diagnostics).
	hs, err := json.Marshal(struct {
		Transport string `json:"transport"`
		Version   string `json:"version"`
		PID       int    `json:"pid"`
	}{Transport: "stdio", Version: version.Version, PID: os.Getpid()})
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), string(hs))

	done := make(chan error, 1)
	go func() { done <- gs.Serve(lis) }()

	select {
	case <-ctx.Done():
		gs.GracefulStop()
		<-done
		return nil
	case err := <-done:
		// The single connection closing (stdin EOF) surfaces as
		// net.ErrClosed from the one-shot listener's Accept — that is the
		// clean stdio shutdown, not a failure.
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		gs.Stop()
		return nil
	}
}

// defaultEngineSocketDir picks the per-user runtime directory for engine
// sockets: $XDG_RUNTIME_DIR/kapi when available (tmpfs, 0700 by contract),
// else <user cache dir>/kapi/run.
func defaultEngineSocketDir() (string, error) {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, "kapi"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve socket dir: %w", err)
	}
	return filepath.Join(cache, "kapi", "run"), nil
}
