package main

import (
	"strings"

	"github.com/neokapi/neokapi/cli"
	cliconfig "github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// offlineOverride is a curated map of dot-joined command paths whose
// offline-capability the flag heuristic below gets wrong. A command is offline
// when it can complete with no outbound network: the heuristic reads that off
// the absence of a --credential flag, which is right for the tool commands but
// blind to management verbs that talk to a registry, a release feed, or a
// keychain daemon.
//
// This is a different axis from runnable-in-browser (which comes from
// cli.BrowserUnavailableReason): `kapi models list` is offline yet unavailable
// in the browser, and `kapi translate` is available in the browser — against
// the deterministic demo provider — yet not offline on a real machine.
var offlineOverride = map[string]bool{
	// Network / AI / MT — require API keys + outbound TLS. (qa is offline by
	// default — rule-based — so it is left to the heuristic rather than
	// force-marked here; with --provider it needs network at run time.)
	"translate":          false,
	"brand-voice-check":  false,
	"brand-voice-review": false,
	// Credentials — require the OS keychain (cgo, subprocess).
	"credentials":        false,
	"credentials.add":    false,
	"credentials.list":   false,
	"credentials.remove": false,
	"credentials.test":   false,
	// Plugin management — network-dependent operations.
	// plugin.list / plugin.info / plugin.rebuild-cache / plugin.verify are
	// local-only, so they remain offline (not overridden here).
	"plugin.install":      false,
	"plugin.remove":       false,
	"plugin.update":       false,
	"plugin.search":       false,
	"plugin.update-index": false,
	// Plugin registry — requires network (GitHub API / plugin index).
	"plugin.registry": false,
	// Model assets — downloaded from a release feed into the shared cache.
	"models.download": false,
	"models.pull":     false,
	// Self-update — resolves and downloads a release.
	"update": false,
	// MCP server — requires a running process.
	"mcp": false,
	// Brand voice — check and rewrite reach an AI provider; the rest of the
	// group (guide, show, profiles, validate, new, import, pack) is local.
	"brand.check":   false,
	"brand.rewrite": false,

	// The porcelain flow verbs carry the shared provider flag set
	// (--credential/--provider/--model), which the heuristic below reads as
	// "needs network". They do not: pseudo-translate is deterministic, and run
	// and up need network only when the flow they execute contains an AI or MT
	// tool. translate is the one that always does, and is marked above.
	"pseudo-translate": true,
	"run":              true,
	"up":               true,
}

// buildKapiRoot constructs the full kapi cobra command tree exactly as the
// real kapi binary does, without wiring plugins or config (which require a
// running process). The resulting tree is used only for metadata extraction;
// none of the RunE functions are called.
//
// The command set comes from cli.KapiCommandSet — the same function
// kapi/cmd/kapi registers and the cli/i18n generator walks — so the reference
// dataset can never document a different surface than the binary ships. It
// used to re-declare the tree by hand, and had silently fallen thirteen verbs
// behind (status, apply, inspect, ls, add, rm, config, hook, pack, unpack,
// info, telemetry, update) while still documenting verbs the hard cutover in
// #1180 had retired.
func buildKapiRoot() *cobra.Command {
	app := &cli.App{}
	// InitRegistries populates FormatReg/ToolReg so NewToolCommands and
	// NewFormatsCmd / NewToolsCmd can enumerate their subcommands correctly.
	app.InitRegistries()

	root := &cobra.Command{
		Use:   "kapi",
		Short: cli.KapiRootShort,
		Long:  cli.KapiRootLong,
	}

	cli.AddPersistentFlags(app, root)
	cli.AddCommandGroups(app, root)

	// Provide a no-op config so PersistentPreRun-style flag lookups don't
	// panic; not called during metadata extraction.
	app.Config = cliconfig.NewAppConfig()

	for _, cmd := range cli.KapiCommandSet(app) {
		root.AddCommand(cmd)
	}

	return root
}

// collectCommands walks the cobra command tree rooted at root and returns a
// flat slice of CommandEntry values. parentPath is the dot-joined path to the
// parent command (empty for root's direct children). The root itself ("kapi")
// is not emitted — only its descendants are.
func collectCommands(cmd *cobra.Command, parentPath []string) []CommandEntry {
	var out []CommandEntry

	for _, sub := range cmd.Commands() {
		// Skip the auto-generated help command; it is not a user-visible command.
		if sub.Name() == "help" {
			continue
		}
		// Hidden commands are shortcuts and machine plumbing (the hidden
		// top-level tool dispatch, engine, toolbox proxies): they must not
		// surface in the reference — the docs document the visible surface.
		if sub.Hidden {
			continue
		}

		path := append(append([]string{}, parentPath...), sub.Name())
		dotPath := strings.Join(path, ".")

		entry := CommandEntry{
			ID:             dotPath,
			Path:           path,
			Use:            sub.Use,
			Short:          sub.Short,
			Long:           strings.TrimSpace(sub.Long),
			GroupID:        sub.GroupID,
			Aliases:        nonEmptyStrings(sub.Aliases),
			Flags:          collectFlags(sub),
			Examples:       parseExamples(sub.Example),
			OfflineCapable: isOfflineCapableCmd(dotPath, sub),
		}

		// Runnable-in-browser comes straight from the browser build's own
		// command set (cli.BrowserCommandSet): a verb the browser cannot run is
		// recorded there with the facility it needs, and everything else is
		// wired up for real. Deriving it here — rather than from a second
		// allowlist — is what keeps the badge honest when the surface changes.
		//
		// Demo mode is then exactly the overlap: a command that runs in the
		// browser but would need an AI or MT provider on a real machine, so the
		// browser resolves it to the deterministic demo provider.
		_, browserGap := cli.BrowserUnavailableReason(path[0])
		entry.RunnableInBrowser = !browserGap
		entry.DemoMode = entry.RunnableInBrowser && !entry.OfflineCapable

		out = append(out, entry)
		// Recurse into subcommands.
		out = append(out, collectCommands(sub, path)...)
	}
	return out
}

// isOfflineCapableCmd combines the curated override with a heuristic: a
// command is offline if it has no "credential" flag (AI/MT tools that call
// external APIs register this flag). The override takes precedence; the
// heuristic only fires for commands it does not name.
func isOfflineCapableCmd(dotPath string, cmd *cobra.Command) bool {
	if v, ok := offlineCapableOverride(dotPath); ok {
		return v
	}
	// Heuristic for unlisted commands: "credential" flag → needs network.
	return cmd.Flags().Lookup("credential") == nil
}

// offlineCapableOverride returns (value, true) when the dot-path — or any
// ancestor prefix marked offline=false, which propagates to descendants —
// appears in offlineOverride, otherwise (false, false).
func offlineCapableOverride(dotPath string) (bool, bool) {
	segments := strings.Split(dotPath, ".")
	// Check ancestors for an explicit offline=false override; it propagates.
	for i := 1; i <= len(segments); i++ {
		prefix := strings.Join(segments[:i], ".")
		if v, ok := offlineOverride[prefix]; ok && !v {
			return false, true
		}
	}
	// Exact override (could be true or false).
	if v, ok := offlineOverride[dotPath]; ok {
		return v, true
	}
	return false, false
}

// collectFlags returns the local (non-inherited) flags for a command, sorted
// by name. Persistent flags on sub-commands are excluded; only flags directly
// registered on this command are returned.
func collectFlags(cmd *cobra.Command) []CommandFlag {
	var flags []CommandFlag
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Skip hidden flags and the help flag.
		if f.Hidden || f.Name == "help" {
			return
		}
		flags = append(flags, CommandFlag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Usage:     f.Usage,
			Default:   f.DefValue,
			Type:      f.Value.Type(),
		})
	})
	return flags
}

// parseExamples splits a cobra Example string into individual non-empty lines,
// trimming leading/trailing whitespace from each line. Returns nil when empty.
func parseExamples(example string) []string {
	if example == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// nonEmptyStrings returns nil when the slice is empty, otherwise the slice
// unchanged. Avoids emitting `"aliases": []` in JSON.
func nonEmptyStrings(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	return ss
}

// collectCommandDataset builds the full kapi command tree and returns a
// CommandDataset ready for JSON serialisation.
func collectCommandDataset(now string) CommandDataset {
	entries := collectCommands(buildKapiRoot(), nil)
	return CommandDataset{
		GeneratedAt: now,
		Commands:    entries,
	}
}
