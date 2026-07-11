package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
)

// BusyboxRoot returns a standalone root command when prog names a multi-call
// toolbox utility (kgrep / ksed / kcat / kconv / kdiff, with an optional .exe suffix), or nil
// when it does not — signalling the caller to run the normal kapi root. The
// returned command owns the app lifecycle (config load, Init, Shutdown) so the
// utility behaves identically whether launched as `kgrep` or `kapi grep`.
func BusyboxRoot(app *App, prog string) *cobra.Command {
	prog = strings.TrimSuffix(strings.ToLower(filepath.Base(prog)), ".exe")
	var cmd *cobra.Command
	switch prog {
	case "kgrep":
		cmd = newGrepCmd(app)
	case "ksed":
		cmd = newSedCmd(app)
		// Faithful `ksed -i.bak` (attached backup suffix) needs arg rewriting.
		cmd.SetArgs(NormalizeSedInPlaceArgs(os.Args[1:]))
	case "kcat":
		cmd = newCatCmd(app)
	case "kconv":
		cmd = newConvCmd(app)
	case "kdiff":
		cmd = newDiffCmd(app)
	default:
		return nil
	}
	// Rebrand the usage line from "grep …" to "kgrep …" (keep the arg spec).
	if i := strings.IndexByte(cmd.Use, ' '); i > 0 {
		cmd.Use = prog + cmd.Use[i:]
	} else {
		cmd.Use = prog
	}
	cmd.GroupID = ""
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		ApplyAppInitializers(app)
		return nil
	}
	cmd.PersistentPostRun = func(*cobra.Command, []string) { app.Shutdown() }
	if inner := cmd.RunE; inner != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error { return MapToolboxErr(inner(c, args)) }
	}
	return cmd
}

// NewToolboxProxies returns the hidden `kapi grep|sed|cat` subcommands. Each is
// a thin proxy with DisableFlagParsing set, so kapi's persistent flags are NOT
// merged into it — the toolbox utilities keep their full classic option surface
// (including -v / -c, which kapi's globals would otherwise shadow). The proxy
// delegates the raw argument list to the very same standalone command the
// kgrep / ksed / kcat binaries run, so `kapi grep` and `kgrep` behave
// identically. They are Hidden so `kapi --help` steers users to the dedicated
// kgrep / ksed / kcat commands.
func NewToolboxProxies(a *App) []*cobra.Command {
	proxy := func(verb, short string, build func() *cobra.Command, normalize func([]string) []string) *cobra.Command {
		return &cobra.Command{
			Use:                verb,
			Short:              short,
			GroupID:            "",
			Hidden:             true,
			DisableFlagParsing: true, // do not inherit/parse kapi's persistent flags
			RunE: func(cmd *cobra.Command, args []string) error {
				if normalize != nil {
					args = normalize(args)
				}
				std := build()
				std.Use = "kapi " + std.Use
				std.SilenceUsage = true
				std.SilenceErrors = true
				std.SetArgs(args)
				return MapToolboxErr(std.ExecuteContext(cmd.Context()))
			},
		}
	}
	return []*cobra.Command{
		proxy("grep", "Search the text/content inside files (use kgrep)", func() *cobra.Command { return newGrepCmd(a) }, nil),
		proxy("sed", "Stream-edit the text/content inside files (use ksed)", func() *cobra.Command { return newSedCmd(a) }, NormalizeSedInPlaceArgs),
		proxy("cat", "Print the text/content inside files (use kcat)", func() *cobra.Command { return newCatCmd(a) }, nil),
		proxy("convert", "Convert files between formats (use kconv)", func() *cobra.Command { return newConvCmd(a) }, nil),
		// No `diff` proxy: kdiff is the file differ's canonical (and only)
		// kapi-side spelling, which frees `kapi diff` for the bowrain
		// plugin's local-vs-server sync diff.
	}
}
