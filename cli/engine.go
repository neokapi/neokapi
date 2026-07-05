//go:build !js

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/cli/engineserve"
	"github.com/neokapi/neokapi/cli/pluginhost"
	enginev1 "github.com/neokapi/neokapi/core/proto/engine/v1"
	"github.com/neokapi/neokapi/core/version"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// NewEngineCmd creates the `engine` command group. Its one subcommand,
// `engine serve`, serves the content engine as a local gRPC service
// (EngineService, core/proto/engine/v1) on a Unix socket.
func (a *App) NewEngineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Serve the content engine as a local gRPC API",
	}
	cmd.AddCommand(a.newEngineServeCmd())
	return cmd
}

func (a *App) newEngineServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the EngineService gRPC API on a local Unix socket",
		Long: `Serve the neokapi content engine as a gRPC service (EngineService,
core/proto/engine/v1) on a local Unix socket, so any gRPC-capable language
can extract documents into the canonical content model, process the part
stream through tools or flows, and merge it back to document bytes.

On startup the command prints a one-line JSON handshake on stdout —
{"socket": "<path>", "version": "<kapi version>", "pid": <pid>} — mirroring
the plugin daemon convention, then serves until interrupted.

Trust model: the service is for TRUSTED LOCAL PEERS ONLY. There is no
authentication in v1; the socket (created inside a 0700 directory) is the
security boundary, exactly like kapi's plugin daemon sockets. Do not expose
the socket over the network or place it in a shared directory.`,
		Example: `  kapi engine serve
  kapi engine serve --socket /tmp/my-engine.sock`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			socket, _ := cmd.Flags().GetString("socket")
			return a.runEngineServe(cmd, socket)
		},
	}
	cmd.Flags().String("socket", "", "Unix socket path to listen on (default: $XDG_RUNTIME_DIR/kapi/engine-<pid>.sock, falling back to the user cache dir)")
	return cmd
}

// runEngineServe binds the Unix socket, prints the handshake, and serves
// until the command context is cancelled (SIGINT/SIGTERM).
func (a *App) runEngineServe(cmd *cobra.Command, socket string) error {
	a.InitRegistries()
	// Plugin-provided formats route through their Mode-C daemons
	// transparently once the plugin host has wired the registry.
	a.InitPluginHost()

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
		return fmt.Errorf("socket path too long (%d bytes, unix limit ~104): %s — pass a shorter --socket", len(socket), socket)
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

	gs := grpc.NewServer()
	enginev1.RegisterEngineServiceServer(gs, engineserve.New(a.FormatReg, a.ToolReg))

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
