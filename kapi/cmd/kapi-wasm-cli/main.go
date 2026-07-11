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
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/config"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/spf13/cobra"
	"syscall/js"
)

var app = &cli.App{}

// ── Engine ABI (the @neokapi/engine contract) ───────────────────────────────
//
// Everything this binary installs on globalThis is the browser engine's JS
// ABI, consumed by the @neokapi/engine npm package (packages/engine). The
// contract:
//
//   - engineExports below is the single registration point. Each entry becomes
//     a global function with exactly that name; nothing else in this binary may
//     call js.Global().Set for an engine entry point.
//   - kapiEngineABI() is the feature-detection endpoint. It returns
//     {abi, version, functions}: `abi` is engineABIVersion, `version` is the
//     kapi build version (ldflags; "dev" for source builds), and `functions`
//     lists the registered global names so clients can probe capabilities
//     without calling them. A build where kapiEngineABI is absent predates the
//     ABI and should be treated as abi 0.
//   - Within one `abi` value, changes are strictly additive: new functions may
//     appear in `functions`, but existing names, signatures, and payload shapes
//     never change or disappear. Renaming or removing a function, or changing a
//     signature/payload incompatibly, bumps engineABIVersion.
//   - Boot handshake: after registration, the binary invokes the host-provided
//     __kapiCliReady() callback (if defined) and then blocks forever so the
//     globals stay callable. The host must install its fs/process shims (the
//     wasm_exec.js environment) before instantiation.
//   - Reverse bridges: the engine optionally calls back into host-provided
//     globals — __kapiPdfium (PDF text+geometry), kapiLocalGenerate (on-device
//     LLM), kapiLocalNER (on-device NER), kapiBrowserTranslate (+ the
//     platform's Translator API). These are capabilities the page MAY provide;
//     each engine feature degrades with an actionable error when its bridge is
//     absent. The typed capability interfaces live in
//     packages/engine/src/capabilities.ts.
//
// The TypeScript half of this contract (ambient global typings + the
// KapiRuntime facade) lives in packages/engine; the docs note is
// web/docs/contribute/notes-internal/wasm-engine-abi.md. Keep all three in
// sync when adding an entry point.

// engineABIVersion is the `abi` field reported by kapiEngineABI(). Bump only
// on a breaking change to an existing entry point (see the contract above).
const engineABIVersion = 1

// engineExports is the ordered, single registration point for the engine's
// global entry points. The names double as the ABI's `functions` list.
var engineExports = []struct {
	name string
	fn   func(js.Value, []js.Value) any
}{
	{"kapiRun", kapiRun},
	{"kapiPreview", kapiPreview},
	{"labInspect", labInspect},
	{"labInspectAnnotated", labInspectAnnotated},
	{"labSegment", labSegment},
	{"labSegmentEngines", labSegmentEngines},
	{"klf", klfDispatch},
}

// registerEngineABI installs every engine entry point plus the additive
// kapiEngineABI() descriptor used for feature detection.
func registerEngineABI() {
	functions := make([]any, len(engineExports))
	for i, e := range engineExports {
		js.Global().Set(e.name, js.FuncOf(e.fn))
		functions[i] = e.name
	}
	js.Global().Set("kapiEngineABI", js.FuncOf(func(js.Value, []js.Value) any {
		return map[string]any{
			"abi":       engineABIVersion,
			"version":   version.Version,
			"functions": functions,
		}
	}))
}

func main() {
	// Populate format + tool registries once so command construction (which
	// enumerates tools/formats) sees them. InitRegistries is idempotent.
	app.InitRegistries()

	// Register the mt-translate tool so it is enumerated by NewToolCommands(). Its
	// engine is resolved per run (browser Translator API where available, else the
	// keyless demo provider — see registerMT). The AI tools (translate, qa,
	// brand-voice-check, …) are already registered by InitRegistries; the demo
	// provider is forced for them per command run via forceDemoProviders.
	registerMT(app.ToolReg)
	// On-device NER (entity-extract engine "ner"): bridge to a JS-loaded
	// model (GLiNER via onnxruntime-web); errors actionably until loaded.
	registerLocalNER()

	// Route the one-time "demo mode" honesty notice to stderr so it surfaces in
	// the browser terminal exactly like a real provider's diagnostics.
	aiprovider.SetDemoNoticeWriter(os.Stderr)
	mtprovider.SetDemoNoticeWriter(os.Stderr)

	// Seed in-memory TM and termbase from embedded fixture data so the tm,
	// termbase, term-check, and extract commands work in the browser build.
	seedBackends()

	registerEngineABI()

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
		Short:         "A format-aware content toolkit (browser build) — parse, edit, check, and localize any format",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			app.Config = config.NewAppConfig()
			if err := app.Init(); err != nil {
				return err
			}
			// App.Init installs a credential-resolution preprocessor; in the
			// browser there are no credentials or network, so override it to
			// coerce AI provider selection to the deterministic demo provider.
			forceDemoProviders(app.ToolReg)
			return nil
		},
	}

	cli.AddPersistentFlags(app, root)
	cli.AddCommandGroups(app, root)

	runCmd := cli.NewRunCmd(app, cli.RunCmdOptions{})
	runCmd.GroupID = "advanced"
	root.AddCommand(runCmd)
	root.AddCommand(cli.NewExtractCmd(app, cli.ExtractCmdOptions{}))
	root.AddCommand(cli.NewMergeCmd(app, cli.MergeCmdOptions{}))
	// .klz workspace verbs (AD-025 §5): pack/unpack the working cache and info
	// to show its dirty state. The cache is the wasm session-persistent
	// in-memory store (core/blockstore/cache_wasm.go).
	root.AddCommand(cli.NewPackCmd(app))
	root.AddCommand(cli.NewUnpackCmd(app))
	root.AddCommand(cli.NewInfoCmd(app))
	// init scaffolds a .kapi project (recipe + state dir) with pure local file
	// writes, so it runs in the browser against the in-memory filesystem.
	root.AddCommand(cli.NewInitCmd(app))
	root.AddCommand(cli.NewAddCmd(app))
	root.AddCommand(cli.NewRmCmd(app))
	root.AddCommand(cli.NewLsCmd(app))
	// status derives project coverage from the files plus the committed state
	// store, and apply writes a review decision into that state store
	// (.kapi-state.json) — both pure local-filesystem + JSON, no SQLite/cgo, so
	// the review→approve loop runs against the in-memory filesystem in the
	// browser. (apply's tm/term kinds, which compile a SQLite cache, are not
	// exercised in the browser; the review kind is.)
	root.AddCommand(cli.NewStatusCmd(app))
	root.AddCommand(cli.NewApplyCmd(app))
	root.AddCommand(cli.NewFlowsCmd(app, cli.FlowCmdOptions{}))
	root.AddCommand(cli.NewToolsCmd(app))
	root.AddCommand(cli.NewFormatsCmd(app))
	root.AddCommand(cli.NewPresetsCmd(app))
	root.AddCommand(cli.NewTranslateCmd(app))
	root.AddCommand(cli.NewPseudoTranslateCmd(app))
	root.AddCommand(cli.NewVersionCmd(app, "kapi"))

	// TM and termbase commands backed by the in-memory fixture data
	// seeded in main() — no SQLite / cgo required in the browser build.
	root.AddCommand(cli.NewTMCmd(app))
	root.AddCommand(cli.NewTermbaseCmd(app))

	// Top-level tool commands (pseudo-translate, word-count, term-check, …).
	for _, c := range cli.NewToolCommands(app) {
		root.AddCommand(c)
	}

	// Format-aware toolbox utilities (grep/sed/cat/convert). `convert` (kconv)
	// powers the in-browser Conversion Lab — read any format, re-express it as a
	// generative target (markdown/html/doclang/xliff/…) via the content model.
	for _, c := range cli.NewToolboxProxies(app) {
		root.AddCommand(c)
	}

	return root
}
