package main

import (
	"strings"

	"github.com/neokapi/neokapi/cli"
	cliconfig "github.com/neokapi/neokapi/cli/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// offlineOverride is a curated map of dot-joined command paths that override
// the wasm-allowlist derivation. Commands that require network, a running
// server, the OS keychain subprocess model, or SQLite cgo are marked false.
// The wasm buildRoot already implies offline=true for the commands it wires;
// this map handles commands that appear in the full kapi tree but should be
// clearly marked as requiring network or system services.
//
// Commands NOT in the wasm allowlist AND NOT listed here default to
// offlineCapable=false (conservative: assume they need something the browser
// cannot provide).
var offlineOverride = map[string]bool{
	// Network / AI / MT — require API keys + outbound TLS. (qa is offline by
	// default — rule-based — so it is left to the conservative default rather
	// than force-marked here; with --provider it needs network at run time.)
	"translate":          false,
	"brand-voice-check":  false,
	"brand-voice-review": false,
	// verify is project-oriented and can invoke an AI-backed QA gate (needs a
	// key); it is not wired into the browser build. (init is pure local
	// scaffolding and IS runnable in the browser — see wasmRunnableTop.)
	"verify": false,
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
	// Registry — requires network (GitHub API / plugin index).
	"registry":         false,
	"registry.add":     false,
	"registry.list":    false,
	"registry.remove":  false,
	"registry.resolve": false,
	// MCP server — requires a running process.
	"mcp": false,
	// Brand voice — the --ai path needs an AI provider/key.
	"brand":             false,
	"brand.voice":       false,
	"brand.voice.apply": false,
	// Skills — requires network (download).
	"skills": false,
}

// wasmAllowlist is the set of dot-joined command paths that are present in
// kapi-wasm-cli's buildRoot(). Commands here are offline-capable by definition.
// Tool commands (pseudo-translate, word-count, …) are added dynamically below
// since they are enumerated from the registry rather than hardcoded.
var wasmAllowlist = map[string]bool{
	"run":     true,
	"flows":   true,
	"tools":   true,
	"formats": true,
	"presets": true,
	"version": true,
	// Subcommands added by NewToolsCmd / NewFormatsCmd / NewPresetsCmd.
	"tools.list":            true,
	"tools.schema":          true,
	"formats.list":          true,
	"formats.info":          true,
	"formats.schema":        true,
	"presets.list":          true,
	"presets.show":          true,
	"presets.apply":         true,
	"completion":            true,
	"completion.bash":       true,
	"completion.zsh":        true,
	"completion.fish":       true,
	"completion.powershell": true,
	// extract/merge are wired into buildRoot; TM-prefill uses the injected
	// in-memory TM. tm/termbase run via in-memory backends seeded from
	// embedded fixtures (#662); no SQLite in the wasm build.
	"extract":            true,
	"merge":              true,
	"tm":                 true,
	"tm.import":          true,
	"tm.import-dir":      true,
	"tm.export":          true,
	"tm.lookup":          true,
	"tm.search":          true,
	"tm.stats":           true,
	"tm.audit":           true,
	"tm.list":            true,
	"tm.sessions":        true,
	"tm.sessions.list":   true,
	"tm.sessions.show":   true,
	"tm.sessions.delete": true,
	"termbase":           true,
	"termbase.import":    true,
	"termbase.export":    true,
	"termbase.lookup":    true,
	"termbase.search":    true,
	"termbase.stats":     true,
	"termbase.list":      true,
	"term-check":         true,
}

// buildKapiRoot constructs the full kapi cobra command tree exactly as the
// real kapi binary does, without wiring plugins or config (which require a
// running process). The resulting tree is used only for metadata extraction;
// none of the RunE functions are called.
//
// This mirrors kapi/cmd/kapi/root.go init() but skips InitPluginHost() and
// config loading since we only need the static command/flag metadata.
// wasmRunnableTop is the set of top-level framework commands wired into the
// kapi-wasm-cli buildRoot (run/extract/merge/init + flows/tools/formats/
// presets/version/completion + tm/termbase). init is pure local scaffolding,
// so it runs against the in-memory filesystem. Tool commands are runnable too
// and are detected separately (enumerated from the registry). Everything else
// (plugin, registry, credentials, mcp, brand, skills, verify) is not in the
// wasm build → not runnable in the browser.
var wasmRunnableTop = map[string]bool{
	"run": true, "extract": true, "merge": true, "init": true, "flows": true,
	"tools": true, "formats": true, "presets": true, "version": true,
	"completion": true, "tm": true, "termbase": true,
}

func buildKapiRoot() (*cobra.Command, map[string]bool) {
	app := &cli.App{}
	// InitRegistries populates FormatReg/ToolReg so NewToolCommands and
	// NewFormatsCmd / NewToolsCmd can enumerate their subcommands correctly.
	app.InitRegistries()

	root := &cobra.Command{
		Use:   "kapi",
		Short: "A localization and translation toolkit",
		Long: `kapi helps you manage multilingual content — convert document formats,
translate with AI, and run quality checks across a wide range of file types.`,
	}

	app.AddPersistentFlags(root)
	app.AddCommandGroups(root)

	// Provide a no-op config so PersistentPreRun-style flag lookups don't
	// panic; not called during metadata extraction.
	app.Config = cliconfig.NewAppConfig()

	// Primary commands.
	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	root.AddCommand(runCmd)
	root.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	root.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))
	root.AddCommand(app.NewVerifyCmd())
	root.AddCommand(app.NewInitCmd())

	// Management commands.
	root.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	root.AddCommand(app.NewToolsCmd())
	root.AddCommand(app.NewFormatsCmd())
	root.AddCommand(app.NewPluginCmd())
	root.AddCommand(app.NewModelsCmd())
	root.AddCommand(app.NewOllamaCmd())
	root.AddCommand(app.NewRegistryCmd())
	root.AddCommand(app.NewPresetsCmd())
	root.AddCommand(app.NewTermbaseCmd())
	root.AddCommand(app.NewTMCmd())
	root.AddCommand(app.NewBrandCmd())
	root.AddCommand(app.NewSkillsCmd())
	root.AddCommand(app.NewCredentialsCmd())
	root.AddCommand(app.NewVersionCmd("kapi"))
	root.AddCommand(app.NewCompletionCmd())

	// Top-level tool commands (pseudo-translate, word-count, …). These are all
	// present in the wasm build; AI/MT ones run there via the demo provider.
	toolNames := map[string]bool{}
	for _, c := range app.NewToolCommands() {
		toolNames[c.Name()] = true
		root.AddCommand(c)
	}

	mcpCmd := app.NewMCPCmd("kapi")
	mcpCmd.GroupID = "processing"
	root.AddCommand(mcpCmd)

	return root, toolNames
}

// collectCommands walks the cobra command tree rooted at root and returns a
// flat slice of CommandEntry values. parentPath is the dot-joined path to the
// parent command (empty for root's direct children). The root itself ("kapi")
// is not emitted — only its descendants are.
func collectCommands(cmd *cobra.Command, parentPath []string, toolNames map[string]bool) []CommandEntry {
	var out []CommandEntry

	for _, sub := range cmd.Commands() {
		// Skip the auto-generated help command; it is not a user-visible command.
		if sub.Name() == "help" {
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

		// Runnable-in-browser = present in the wasm buildRoot: a framework
		// command (or descendant of one) or a tool command. AI/MT tool commands
		// (those with a --credential flag) run there via the demo provider.
		top := path[0]
		isTool := toolNames[top]
		entry.RunnableInBrowser = wasmRunnableTop[top] || isTool
		entry.DemoMode = isTool && sub.Flags().Lookup("credential") != nil

		out = append(out, entry)
		// Recurse into subcommands.
		out = append(out, collectCommands(sub, path, toolNames)...)
	}
	return out
}

// isOfflineCapableCmd combines the static map lookup with a heuristic for
// tool-level commands that are not listed in either map: a command is offline
// if it has no "credential" flag (AI/MT tools that call external APIs register
// this flag). The static maps take precedence; the heuristic only fires for
// genuinely unlisted commands.
func isOfflineCapableCmd(dotPath string, cmd *cobra.Command) bool {
	// Static map + ancestor propagation always wins.
	staticResult, hasStatic := offlineCapableFromMaps(dotPath)
	if hasStatic {
		return staticResult
	}
	// Heuristic for unlisted commands: "credential" flag → needs network.
	return cmd.Flags().Lookup("credential") == nil
}

// offlineCapableFromMaps returns (value, true) when the dot-path (or any
// ancestor prefix) appears in either static map, otherwise (false, false).
func offlineCapableFromMaps(dotPath string) (bool, bool) {
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
	// WASM allowlist.
	if v, ok := wasmAllowlist[dotPath]; ok {
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
	root, toolNames := buildKapiRoot()
	entries := collectCommands(root, nil, toolNames)
	return CommandDataset{
		GeneratedAt: now,
		Commands:    entries,
	}
}
