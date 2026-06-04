//go:build js && wasm

// Command kapi-wasm-cli is a browser entrypoint that runs the kapi CLI inside
// WebAssembly. It exposes a single JS function, kapiRun(argv []string), which
// builds a fresh cobra root and executes one command — turning the one-shot
// CLI into a REPL the page can drive from xterm.js. Standard output and
// standard error flow through os.Stdout/os.Stderr (i.e. globalThis.fs) so the
// page can route them to the terminal exactly as a real shell would.
//
// The browser-safe command subset is wired here. Commands that need a
// subprocess (plugins) or the OS keychain (credentials) are omitted.
// tm and termbase are included: they use the in-memory backends seeded
// from embedded fixtures (see wasm_backends.go) — no cgo or SQLite needed.
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/spf13/cobra"
	"syscall/js"
)

var app = &cli.App{}

func main() {
	// Populate format + tool registries once so command construction (which
	// enumerates tools/formats) sees them. InitRegistries is idempotent.
	app.InitRegistries()

	// Register the demo MT translate tool so `mt-translate` is enumerated by
	// NewToolCommands(). The AI tools (ai-translate, ai-qa, brand-voice-check,
	// …) are already registered by InitRegistries; the demo provider is forced
	// for them per command run via forceDemoProviders (see buildRoot).
	registerDemoMT(app.ToolReg)

	// Route the one-time "demo mode" honesty notice to stderr so it surfaces in
	// the browser terminal exactly like a real provider's diagnostics.
	aiprovider.SetDemoNoticeWriter(os.Stderr)
	mtprovider.SetDemoNoticeWriter(os.Stderr)

	// Seed in-memory TM and termbase from embedded fixture data so the tm,
	// termbase, term-check, and extract commands work in the browser build.
	seedBackends()

	js.Global().Set("kapiRun", js.FuncOf(kapiRun))
	js.Global().Set("kapiPreview", js.FuncOf(kapiPreview))
	js.Global().Set("labInspect", js.FuncOf(labInspect))
	js.Global().Set("klf", js.FuncOf(klfDispatch))

	if ready := js.Global().Get("__kapiCliReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}

	select {} // keep the instance alive so kapiRun stays callable
}

// kapiRun executes one CLI invocation. args[0] is a JS array of argv tokens
// (without the leading "kapi"). It returns a Promise that resolves to the
// process-style exit code; output is written to os.Stdout/os.Stderr.
//
// The work runs in a goroutine so the call can return to JS immediately:
// Go's js/wasm filesystem ops (open/read/stat) are asynchronous and block on
// a channel serviced by the JS event loop, so running the command
// synchronously inside this callback would deadlock. Returning a Promise lets
// the event loop run while the goroutine parks on fs I/O.
func kapiRun(_ js.Value, args []js.Value) any {
	argv := []string{}
	if len(args) >= 1 {
		jsArgv := args[0]
		argv = make([]string, jsArgv.Length())
		for i := range argv {
			argv[i] = jsArgv.Index(i).String()
		}
	}

	executor := js.FuncOf(func(_ js.Value, p []js.Value) any {
		resolve := p[0]
		go func() {
			resolve.Invoke(runOnce(argv))
		}()
		return js.Undefined()
	})
	promise := js.Global().Get("Promise").New(executor)
	return promise
}

// runOnce builds a fresh root and executes one command, returning the exit
// code. It recovers panics so a single bad command can't kill the instance.
func runOnce(argv []string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "kapi: internal error: %v\n", r)
			code = 2
		}
	}()

	root := buildRoot()
	root.SetArgs(argv)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

// buildRoot constructs a fresh kapi root command with the browser-safe
// subset. A fresh tree per invocation avoids flag state leaking between
// REPL commands.
func buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "kapi",
		Short:         "A localization and translation toolkit (browser build)",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			app.Config = config.NewAppConfig()
			app.Init()
			// App.Init installs a credential-resolution preprocessor; in the
			// browser there are no credentials or network, so override it to
			// coerce AI provider selection to the deterministic demo provider.
			forceDemoProviders(app.ToolReg)
		},
	}

	app.AddPersistentFlags(root)
	app.AddCommandGroups(root)

	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	root.AddCommand(runCmd)
	root.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	root.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))
	// init scaffolds a .kapi project (recipe + state dir) with pure local file
	// writes, so it runs in the browser against the in-memory filesystem.
	root.AddCommand(app.NewInitCmd())
	root.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	root.AddCommand(app.NewToolsCmd())
	root.AddCommand(app.NewFormatsCmd())
	root.AddCommand(app.NewPresetsCmd())
	root.AddCommand(app.NewVersionCmd("kapi"))

	// TM and termbase commands backed by the in-memory fixture data
	// seeded in main() — no SQLite / cgo required in the browser build.
	root.AddCommand(app.NewTMCmd())
	root.AddCommand(app.NewTermbaseCmd())

	// Top-level tool commands (pseudo-translate, word-count, term-check, …).
	for _, c := range app.NewToolCommands() {
		root.AddCommand(c)
	}

	return root
}
