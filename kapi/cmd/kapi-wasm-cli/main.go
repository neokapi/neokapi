//go:build js && wasm

// Command kapi-wasm-cli is a browser entrypoint that runs the kapi CLI inside
// WebAssembly. It exposes a single JS function, kapiRun(argv []string), which
// builds a fresh cobra root and executes one command — turning the one-shot
// CLI into a REPL the page can drive from xterm.js. Standard output and
// standard error flow through os.Stdout/os.Stderr (i.e. globalThis.fs) so the
// page can route them to the terminal exactly as a real shell would.
//
// Only the browser-safe command subset is wired here. Commands that need a
// subprocess (plugins), the OS keychain (credentials), or SQLite (termbase,
// tm) are intentionally omitted — they'd compile but fail at runtime.
package main

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/config"
	"github.com/spf13/cobra"
	"syscall/js"
)

var app = &cli.App{}

func main() {
	// Populate format + tool registries once so command construction (which
	// enumerates tools/formats) sees them. InitRegistries is idempotent.
	app.InitRegistries()

	js.Global().Set("kapiRun", js.FuncOf(kapiRun))
	js.Global().Set("kapiPreview", js.FuncOf(kapiPreview))

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
		},
	}

	app.AddPersistentFlags(root)
	app.AddCommandGroups(root)

	runCmd := app.NewRunCmd(cli.RunCmdOptions{})
	runCmd.GroupID = "processing"
	root.AddCommand(runCmd)
	root.AddCommand(app.NewExtractCmd(cli.ExtractCmdOptions{}))
	root.AddCommand(app.NewMergeCmd(cli.MergeCmdOptions{}))
	root.AddCommand(app.NewFlowsCmd(cli.FlowCmdOptions{}))
	root.AddCommand(app.NewToolsCmd())
	root.AddCommand(app.NewFormatsCmd())
	root.AddCommand(app.NewPresetsCmd())
	root.AddCommand(app.NewVersionCmd("kapi"))

	// Top-level tool commands (pseudo-translate, word-count, …).
	for _, c := range app.NewToolCommands() {
		root.AddCommand(c)
	}

	return root
}
