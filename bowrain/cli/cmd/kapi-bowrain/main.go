// Command kapi-bowrain is the bowrain plugin binary for kapi
// (manifest-driven plugin model, #438).
//
// kapi discovers this binary via $XDG_DATA_HOME/kapi/plugins/bowrain/ or
// the equivalent system path, reads the sibling manifest.json, and execs
// this binary in one of three modes:
//
//	kapi-bowrain command <name> [args]   # Mode A — one-shot per command
//	kapi-bowrain mcp-server               # Mode B — long-lived MCP-over-stdio
//	kapi-bowrain version                  # utility — print plugin version
//
// All bowrain commands (push, pull, status, auth, ...) live as
// subcommands under the `command` cobra subtree.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/neokapi/neokapi/cli"
	cliconfig "github.com/neokapi/neokapi/cli/config"
	"github.com/spf13/cobra"

	// The bowrain plugin's commands package init() registers the
	// CommandFactories that build the bowrain command tree on top of the
	// shared cli.App. Blank-importing the anchor pulls in schema +
	// commands + MCP tool registrations in one go.
	_ "github.com/neokapi/neokapi/bowrain/plugin"
)

// pluginVersion is set at link time via:
//
//	-ldflags "-X main.pluginVersion=1.4.0"
var pluginVersion = "0.0.0-dev"

var app = &cli.App{}

var rootCmd = &cobra.Command{
	Use:           "kapi-bowrain",
	Short:         "Bowrain plugin for kapi",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `kapi-bowrain is the bowrain plugin binary that kapi dispatches to
under the manifest-driven plugin model. End users typically do not run
this binary directly; instead they install it via:

  kapi plugin install bowrain
  brew install neokapi/tap/bowrain-cli

and then run bowrain commands through kapi (e.g., kapi push, kapi pull).`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// SilenceErrors keeps cobra from printing (so kapi can format dispatch
		// errors on its side), but when this binary runs as a Mode-A subprocess
		// its stderr is inherited by the user — so we must surface the error
		// ourselves. Without this, a bad flag or failed command exits 1 silently.
		// (ExitError from a nested exec already printed its own output, so don't
		// double-report it.)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

func init() {
	app.InitRegistries()

	rootCmd.AddCommand(buildCommandSubtree())
	rootCmd.AddCommand(buildMCPServerCmd())
	rootCmd.AddCommand(buildDaemonCmd())
	rootCmd.AddCommand(buildVersionCmd())
}

// buildCommandSubtree returns the `command` cobra subcommand. Every
// bowrain CLI command (push, pull, status, auth, …) is attached as a
// child. kapi spawns this binary as
//
//	kapi-bowrain command push --force my/path
//
// and cobra routes that down to the push handler.
func buildCommandSubtree() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "command",
		Short:         "Run a bowrain command",
		Hidden:        true, // not relevant for human users
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.Config = newBowrainAppConfig()
			app.RegistryResolver = func() []cliconfig.RegistryEntry { return nil }
			if err := app.Init(); err != nil {
				return err
			}
			cli.ApplyAppInitializers(app)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			app.Shutdown()
		},
	}

	app.AddPersistentFlags(cmd)
	app.AddCommandGroups(cmd)

	// Built-in framework commands the bowrain CLI used to expose
	// alongside its own commands. We expose the full set so a power user
	// running `kapi-bowrain command run translate` has the same surface
	// as the legacy `bowrain` binary.
	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	cmd.AddCommand(runCmd)
	cmd.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	cmd.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))
	cmd.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	cmd.AddCommand(app.NewToolsCmd())
	cmd.AddCommand(app.NewFormatsCmd())
	cmd.AddCommand(app.NewRegistryCmd())
	cmd.AddCommand(app.NewPresetsCmd())
	cmd.AddCommand(app.NewTermbaseCmd())
	cmd.AddCommand(app.NewTMCmd())
	cmd.AddCommand(app.NewCredentialsCmd())
	cmd.AddCommand(app.NewCompletionCmd())

	for _, tc := range app.NewToolCommands() {
		cmd.AddCommand(tc)
	}

	// Bowrain plugin subcommands (push, pull, status, ls, add, rm, sync,
	// auth, config, diff, stream, ui, init).
	cli.ApplyCommandFactories(cmd, app)
	return cmd
}

// buildMCPServerCmd returns the mcp-server subcommand. kapi spawns
// kapi-bowrain mcp-server once per `kapi mcp` session and proxies tool
// calls over stdio.
func buildMCPServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp-server",
		Short: "Run as an MCP-over-stdio server",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			app.Config = newBowrainAppConfig()
			app.RegistryResolver = func() []cliconfig.RegistryEntry { return nil }
			if err := app.Init(); err != nil {
				return err
			}
			cli.ApplyAppInitializers(app)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			app.Shutdown()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			mcpCmd := app.NewMCPCmd("kapi-bowrain")
			return mcpCmd.RunE(mcpCmd, args)
		},
	}
}

// buildVersionCmd returns the version subcommand. kapi plugin verify
// uses this to confirm the binary matches the manifest.
func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print plugin version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(pluginVersion)
		},
	}
}

func newBowrainAppConfig() *cliconfig.AppConfig {
	return cliconfig.NewOverlayAppConfig("bowrain", func(cfg *cliconfig.AppConfig) {
		cfg.Viper().SetDefault("server.url", "http://localhost:8080")
		_ = cfg.Viper().BindEnv("server.url", "BOWRAIN_SERVER_URL")
	})
}
