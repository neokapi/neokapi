//go:build !js

package cli

import (
	"github.com/spf13/cobra"
)

// NewEngineCmd creates the `engine` command group. Its one subcommand,
// `engine serve`, serves the content engine as a local gRPC service
// (EngineService, core/proto/engine/v1) on a Unix socket.
func NewEngineCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Serve the content engine as a local gRPC API",
	}
	cmd.AddCommand(newEngineServeCmd(a))
	return cmd
}

func newEngineServeCmd(a *App) *cobra.Command {
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
			return a.RunEngineServe(cmd, socket)
		},
	}
	cmd.Flags().String("socket", "", "Unix socket path to listen on (default: $XDG_RUNTIME_DIR/kapi/engine-<pid>.sock, falling back to the user cache dir)")
	return cmd
}
