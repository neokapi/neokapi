//go:build js && wasm

// Command kapi-wasm-cli is a browser entrypoint that runs the kapi CLI inside
// WebAssembly. It exposes a single JS function, kapiRun(argv []string), which
// builds a fresh cobra root and executes one command — turning the one-shot
// CLI into a REPL the page can drive from xterm.js. Standard output and
// standard error flow through os.Stdout/os.Stderr (i.e. globalThis.fs) so the
// page can route them to the terminal exactly as a real shell would.
//
// The command surface comes from cli.BrowserCommandSet, which mirrors the
// native binary's cli.KapiCommandSet verb for verb: browser-safe commands are
// built for real, and the ones needing a subprocess (plugins), the OS keychain
// (credentials), the network (models, update) or a socket (engine, mcp) report
// that limitation instead of going missing. tm and terms run against the
// in-memory backends seeded from embedded fixtures (see wasm_backends.go) — no
// cgo or SQLite needed.
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
// web/docs/contribute/implementation/surfaces/wasm-engine-abi.md. Keep all three in
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
	{"kbf", kbfDispatch},
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
	// voice-check, …) are already registered by InitRegistries; the demo
	// provider is forced for them per command run via forceDemoProviders.
	registerMT(app.ToolReg)
	// On-device NER (entity-extract engine "ner"): bridge to a JS-loaded
	// model (GLiNER via onnxruntime-web); errors actionably until loaded.
	registerLocalNER()

	// Route the one-time "demo mode" honesty notice to stderr so it surfaces in
	// the browser terminal exactly like a real provider's diagnostics.
	aiprovider.SetDemoNoticeWriter(os.Stderr)
	mtprovider.SetDemoNoticeWriter(os.Stderr)

	// Seed in-memory content memory and terms from embedded fixture data so the tm,
	// terms, term-check, and extract commands work in the browser build.
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

	// The browser build has no PersistentPostRun, so nothing rendered the
	// --explain-prompts transcript: the flag parsed, the recorder captured every
	// call, and the output was dropped on the floor. Flush it here — including
	// when the command failed, since a failed run's prompts are the ones you most
	// want to see.
	defer func() {
		if err := app.FlushExplain(); err != nil {
			fmt.Fprintln(os.Stderr, err)
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
		Short:         "A format-aware content toolkit (browser build): parse, edit, check, and localize any format",
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
			forceDemoProviders(app)
			return nil
		},
	}

	cli.AddPersistentFlags(app, root)
	cli.AddCommandGroups(app, root)

	// One command surface, declared once. cli.BrowserCommandSet mirrors
	// cli.KapiCommandSet (the native binary's surface) verb for verb: every
	// browser-safe command is constructed by its real factory, and the handful
	// that need a subprocess, the OS keychain, the network, or a socket
	// register a command that says so. cli.TestBrowserCommandSurface fails the
	// build if the two sets ever diverge, so a verb added or renamed natively
	// can no longer go missing here and surface to a lab user as cobra's
	// `unknown command`.
	for _, c := range cli.BrowserCommandSet(app) {
		root.AddCommand(c)
	}

	return root
}
