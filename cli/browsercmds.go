package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// The browser (WebAssembly) command surface.
//
// kapi ships a second binary — kapi/cmd/kapi-wasm-cli — that runs the CLI
// inside the browser: the docs playground, the interactive labs, and the
// snippet verifier all drive it through kapiRun(argv). That build used to
// hand-roll its own list of cobra commands, which silently drifted from the
// native binary: a verb renamed or added in KapiCommandSet stayed unchanged in
// the browser list, and every such gap surfaced to a lab user as cobra's
// unhelpful `unknown command "x" for "kapi"`.
//
// BrowserCommandSet is the fix. It mirrors KapiCommandSet exactly — same
// commands, same order, same help groups — but constructs a browser-safe
// command for every verb that can run against the in-memory filesystem, and an
// explicit "not available in the browser" command for the handful that cannot.
// A verb is therefore never missing: it either works, or it says why not.
//
// TestBrowserCommandSurface pins the mirror: it fails when a verb exists in one
// set and not the other, and when a browserGap's help metadata drifts from the
// native command it stands in for. Adding a command to KapiCommandSet is
// consequently a decision — wire it up for the browser, or record why it can't
// run there — never a silent omission.

// browserGap describes a kapi verb that cannot run in the browser build,
// together with the help metadata it must present so `kapi --help` reads the
// same in the lab as it does natively.
//
// short/group/hidden mirror the native command built by KapiCommandSet;
// TestBrowserCommandSurface asserts the mirror, so changing a native command's
// Short shows up here as a failing test rather than as a stale help line in the
// browser.
type browserGap struct {
	short  string // native command's Short (asserted equal by the guard test)
	group  string // native command's GroupID
	hidden bool   // native command's Hidden
	reason string // why the verb cannot run in the browser, in one clause
}

// browserGaps is the complete set of kapi verbs the browser build cannot run.
// Everything else in KapiCommandSet is expected to work against the in-memory
// filesystem, the in-memory content memory/terms, and the deterministic demo AI provider.
//
// Each reason names the specific host facility the browser denies — a
// subprocess, the OS keychain, the network, or a socket — because "not
// supported" alone tells a lab user nothing about whether their own machine
// would behave differently.
var browserGaps = map[string]browserGap{
	"plugin": {
		short:  "Install and manage manifest-driven plugins (#438)",
		group:  "advanced",
		reason: "plugins are separate executables the browser cannot download or run as subprocesses",
	},
	"models": {
		short:  "Manage attached LLMs and ML models",
		group:  "assets",
		reason: "model assets are downloaded to a shared on-disk cache, which the browser has no access to",
	},
	"credentials": {
		short:  "Manage saved AI provider credentials",
		group:  "assets",
		reason: "credentials live in the operating system keychain, which the browser has no access to",
	},
	"telemetry": {
		short:  "Show or change anonymous usage telemetry (status, on, off)",
		reason: "the browser build never collects or reports telemetry, so there is nothing to configure",
	},
	"update": {
		short:  "Update kapi to the latest release",
		reason: "the browser build is served as a WebAssembly module and cannot replace itself",
	},
	"mcp": {
		short:  "Start MCP server (stdio) exposing the project's context and the loop around it",
		group:  "advanced",
		reason: "an MCP server serves a long-lived stdio session, which the browser build has no peer for",
	},
	"engine": {
		short:  "Serve the content engine as a local gRPC API",
		hidden: true,
		reason: "the engine gRPC service listens on a Unix socket, which the browser has no access to",
	},
}

// BrowserUnavailableReason reports why the browser build cannot run the
// top-level verb name, and false when the verb does run there. It is the
// single answer to "does this command work in the lab?" — the docs command
// reference (scripts/gen-refs) derives its runnable-in-browser badge from it
// rather than from a second, hand-maintained allowlist.
func BrowserUnavailableReason(name string) (string, bool) {
	gap, ok := browserGaps[name]
	if !ok {
		return "", false
	}
	return gap.reason, true
}

// BrowserUnavailableError is the error a browser-unavailable verb returns. It
// names the verb and the specific facility the browser denies, so the lab
// terminal explains the limitation instead of implying the verb does not exist.
func BrowserUnavailableError(name, reason string) error {
	return fmt.Errorf("kapi %s is not available in the browser: %s", name, reason)
}

// newBrowserGapCmd builds the stand-in command for a verb the browser cannot
// run. It parses nothing and accepts anything, so every spelling a user might
// reach for — `kapi plugin`, `kapi plugin list`, `kapi models download x --y` —
// lands on the same honest error rather than on a second, more confusing
// "unknown command" from a subcommand that was never registered.
//
// `--help` is the one exception: it still prints help, because a reader asking
// what a verb does deserves an answer even where they cannot run it. The Long
// text carries the limitation, so the help itself explains the gap.
func newBrowserGapCmd(name string) *cobra.Command {
	gap, ok := browserGaps[name]
	if !ok {
		// Unreachable via BrowserCommandSet (the guard test pins the table),
		// but a wrong name must not produce a silently empty command.
		panic("cli: no browser gap recorded for command " + name)
	}
	cmd := &cobra.Command{
		Use:     name,
		Short:   gap.short,
		Long:    gap.short + ".\n\nNot available in the browser: " + gap.reason + ".\nRun it with the kapi CLI on your own machine.",
		GroupID: gap.group,
		Hidden:  gap.hidden,
		Args:    cobra.ArbitraryArgs,
		// The real command's flags and subcommands are not linked into the
		// browser build, so nothing here can be parsed or routed. Taking argv
		// verbatim keeps every spelling on the one honest error.
		DisableFlagParsing: true,
		SilenceUsage:       true,
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		for _, a := range args {
			if a == "-h" || a == "--help" {
				return c.Help()
			}
		}
		return BrowserUnavailableError(name, gap.reason)
	}
	return cmd
}

// BrowserCommandSet constructs the kapi command set for the WebAssembly build,
// in the same registration order as KapiCommandSet. Verbs that run in the
// browser are built by their real factories; the rest are the explicit
// browser-gap commands above.
//
// Keep this function structurally parallel to KapiCommandSet — the two are
// meant to be read side by side, and the guard test compares them verb for
// verb.
func BrowserCommandSet(a *App) []*cobra.Command {
	var cmds []*cobra.Command

	// Porcelain. `up` converges the project by running its flows over local
	// files; in the browser that is the in-memory filesystem, and a recipe with
	// a server: block simply has no plugin to dispatch to, so it stays local.
	cmds = append(cmds, NewUpCmd(a))
	cmds = append(cmds, NewTranslateCmd(a), NewPseudoTranslateCmd(a))

	// Primary commands.
	runCmd := NewRunCmd(a, RunCmdOptions{})
	runCmd.GroupID = "advanced"
	cmds = append(cmds, runCmd)
	cmds = append(cmds, NewExtractCmd(a, ExtractCmdOptions{}))
	cmds = append(cmds, NewMergeCmd(a, MergeCmdOptions{}))

	// .kpz workspace verbs (AD-025 §5): the working cache is the wasm
	// session-persistent in-memory store (core/blockstore/cache_wasm.go).
	cmds = append(cmds, NewPackCmd(a), NewUnpackCmd(a), NewInfoCmd(a))

	// Format-aware toolbox utilities. `kconv` powers the in-browser Conversion
	// Lab — read any format, re-express it as a generative target through the
	// content model.
	cmds = append(cmds, NewToolboxProxies(a)...)
	cmds = append(cmds, NewInspectCmd(a))
	cmds = append(cmds, NewApplyCmd(a))
	cmds = append(cmds, NewStatsCmd(a))
	cmds = append(cmds,
		NewStatusCmd(a),
		// `commit` runs in the browser: the working store falls back to a JSON
		// sidecar when SQLite is absent (core/state/workstore.go), and the
		// committed shards write through the sandbox filesystem.
		NewCommitCmd(a),
		NewCheckCmd(a),
		NewHookCmd(a),
		NewInitCmd(a),
		NewAddCmd(a),
		NewRmCmd(a),
		NewLsCmd(a),
	)

	// Management commands. Content memory and terms run against the in-memory backends
	// the wasm entrypoint seeds from embedded fixtures — no SQLite, no cgo.
	cmds = append(cmds,
		NewFlowsCmd(a, FlowCmdOptions{}),
		NewToolsCmd(a),
		NewFormatsCmd(a),
		newBrowserGapCmd("plugin"),
		newBrowserGapCmd("models"),
		NewContextCmd(a),
		NewTermsCmd(a),
		NewMemoryCmd(a),
		NewVoiceCmd(a),
		newBrowserGapCmd("credentials"),
		NewConfigCmd(a),
		newBrowserGapCmd("telemetry"),
		NewVersionCmd(a, "kapi"),
		newBrowserGapCmd("update"),
		NewCompletionCmd(a),
	)

	// Top-level tool commands (the `exec` group over the tool registry).
	cmds = append(cmds, NewToolCommands(a)...)

	cmds = append(cmds, newBrowserGapCmd("mcp"))
	cmds = append(cmds, newBrowserGapCmd("engine"))

	return cmds
}
