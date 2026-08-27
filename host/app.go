// Package cli provides a shared CLI base for neokapi CLI tools.
// CLI tools build on this package, selecting which commands to expose.
package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore"
	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
	clii18n "github.com/neokapi/neokapi/host/i18n"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// App holds shared CLI state that is initialized during PersistentPreRun.
// CLI tools create an App instance and attach shared commands.
type App struct {
	FormatReg  *registry.FormatRegistry
	ToolReg    *registry.ToolRegistry
	SchemaReg  *schema.SchemaRegistry
	PluginHost *pluginhost.Host
	Config     *config.AppConfig

	// Flags bound by AddPersistentFlags.
	Verbose   bool
	Quiet     bool
	AssumeYes bool // --yes / -y; auto-Confirm prompts (e.g. plugin auto-install)
	CfgFile   string
	PluginDir string
	Lang      string // --lang / KAPI_LANG; feeds i18n.Resolve
	// Explain is --explain: show every prompt this run sends to an LLM.
	// ExplainStderr ("-") renders a transcript to stderr; any other value is a
	// path to write the exchanges to as JSON. Empty disables it.
	Explain string

	// explain collects LLM exchanges while Explain is set.
	explain *explainCollector

	// isTTY overrides terminal detection for the prompts App itself raises
	// (today: adopting a package-carried recipe on unpack). nil means check
	// os.Stdin. Tests swap it; nothing else should.
	isTTY func() bool

	// Processing flags bound by AddProcessingFlags.
	FormatFlag string
	Encoding   string
	SourceLang string
	TargetLang string

	// ConvTiming makes kconv report each file's conversion time on stderr —
	// the in-process cost, so the figure is comparable with what conversion
	// libraries publish, unlike wall-clocking the process from outside, which
	// mostly measures binary start-up.
	ConvTiming bool

	// MemoryBackend, when non-nil, is returned by OpenMemorySQLite instead of
	// opening a SQLite database. Used by the WASM browser build to inject
	// a pre-seeded InMemoryStore so the tm / extract commands work without cgo.
	MemoryBackend memory.Store

	// TermsBackend, when non-nil, is returned by OpenTermsSQLite instead
	// of opening a SQLite database. Used by the WASM browser build to
	// inject a pre-seeded InMemoryStore so terms / term-check work
	// without cgo.
	TermsBackend terms.Terminology

	// BlocksBackend, when non-nil, is the project's block cache, in place of
	// the one inside `.kapi/work/store.db`. The browser build injects a
	// process-lifetime in-memory store, since it has no file-backed SQLite:
	// what the lab extracts in one command is what the next one searches.
	BlocksBackend blockstore.Store

	// Credentials is the shared credential store for AI provider keys.
	Credentials *credentials.Store

	// AISetupIOOverride, when non-nil, replaces the default wizard IO used by
	// EnsureAIProviderInteractive and `kapi models setup` — tests inject
	// scripted stdin, fake detection, and stub live checks through it.
	AISetupIOOverride *AISetupIO

	// AISetupPrompter, when non-nil, renders the setup wizard's questions. The
	// terminal experience is a presentation concern, so the front-end registers
	// it (the CLI installs a form-based one) and host keeps its plain numbered
	// reader as the default for a dumb TTY, a pipe, and the tests.
	AISetupPrompter AISetupPrompter

	// RegistryResolver is an optional hook for resolving plugin registries.
	// When set, it is called before falling back to the config-based registries.
	RegistryResolver func() []config.RegistryEntry

	// FallbackRunE is called by NewRunCmd / NewFlowsCmd when the named
	// flow is not a built-in flow. Plugins (e.g. bowrain) install this
	// via RegisterAppInitializer to support project-defined flows.
	// Read by NewRunCmd / NewFlowsCmd as a default when the per-command
	// CmdOptions struct does not explicitly set it.
	FallbackRunE func(cmd Command, flowName string, args []string) error

	// ExtraFlows returns additional flow infos for the `flows` command.
	// Plugins install this via RegisterAppInitializer. Read by
	// NewFlowsCmd as a default when the per-command CmdOptions struct
	// does not explicitly set it.
	ExtraFlows func() []output.FlowInfo

	// ProjectContext is set temporarily by RunFromProject so that downstream
	// methods (reader creation, writer setup) can apply project format defaults.
	ProjectContext *project.ProjectContext

	// MCPSurface widens the MCP tool surface beyond the curated default. Set
	// from `kapi mcp` flags; the zero value is the curated set.
	MCPSurface MCPSurface

	// freshness remembers the governance identities this process last read, so
	// a retrieval answer can say what moved under it. Process-lived on purpose:
	// it is the difference between two reads by the same reader, which is what
	// an assistant holding a stale answer needs told. See host/freshness.go.
	freshness     *governanceWatch
	freshnessOnce sync.Once

	// execTrustGranted records that this process has established the right to
	// run the active project's exec-class steps — the user answered the
	// prompt, a stored decision matched, or the environment granted it. Set by
	// ensureExecTrust and read by checkExecToolAllowed; see host/exectrust.go.
	execTrustGranted bool

	// convergeWriteFiles forces the file-writing path even for a single input in
	// a project, overriding the process-only default (AD-026). Convergence
	// (`kapi run` with no flow) sets it so it materializes the localized target
	// files its file-derived coverage then reads — uniformly, whether the project
	// has one content file or many.
	convergeWriteFiles bool

	// convergeDraftDir is where a convergence pass writes those files: a
	// run-local tree under `.kapi/work/`, never the collection's own `target:`
	// path.
	//
	// A pass's output is a draft. Coverage is derived from files, so the pass
	// has to write them somewhere a reader can measure — but writing them at the
	// destination puts an unreviewed draft where every static-site build,
	// bundler and publishing connector reads, before any gate has been
	// consulted. Materializing from the project block store (finishConverge, and
	// `kapi merge`) is then the only write that reaches the destination, and it
	// is the write the ship gate governs.
	//
	// Empty outside a convergence run, which leaves every other flow run writing
	// where the recipe says.
	convergeDraftDir string

	// convergeDraftRoot is the project root the draft tree mirrors paths
	// against, so a target outside the project — which no recipe gate reaches —
	// is written where it was resolved rather than folded into the tree.
	convergeDraftRoot string

	// docCache is the project's streaming document cache, opened by a project-level
	// command (withParseCache) so repeated reads of unchanged files — across
	// `status` re-runs, `verify`, every `run --until-gate` pass, and the flow
	// runner — replay a prior parse one part at a time instead of re-parsing. It is
	// a rebuildable optimization over the files (the source of truth): blow it away
	// and a re-read reconstructs identical results. nil for ad-hoc reads (no
	// project), which parse directly.
	docCache *docCache

	// projectFlowTools is set temporarily by runProjectSteps to override
	// buildFlowTools for project-defined flows.
	projectFlowTools []tool.Tool

	// flowFindings is armed for the span of one reported flow run so the check
	// steps of the flow have somewhere to report what they found. Non-nil only
	// between beginFlowFindings and the run's own output.
	flowFindings *flowFindings

	// convergeProgressTap, when non-nil, is appended by runProjectStepsOver as
	// a trailing read-only step so a convergence run can count units live.
	// Set only on per-locale converge worker Apps (convergeWorker); nil
	// everywhere else.
	convergeProgressTap tool.Tool

	// ProjectBindings carries the standing voice + terms context
	// resolved from a .kapi project (defaults.voice / defaults.terms_source).
	// Set temporarily by RunFromProject so project-flow steps can be made
	// brand- and terminology-aware with no flags. nil for ad-hoc runs.
	ProjectBindings *ProjectBindings

	// translator localizes tool/format/plugin metadata at API egress.
	// Built during Init from --lang / KAPI_LANG / config / POSIX env.
	// Never nil after Init — unresolved locales get a NoopTranslator
	// so T() calls are always safe.
	translator i18n.Translator

	// pluginRuntime owns the live plugin host + Mode-C daemon pool and the
	// shared discover→wire sequence. Built lazily (InitPluginHost or the first
	// DaemonPool call); the daemon pool it holds is torn down by Shutdown.
	pluginRuntimeOnce sync.Once
	pluginRuntime     *pluginhost.Runtime

	// projectStores holds the open project stores (ProjectDB): one per project
	// root, shared by every command and every converge worker under this App,
	// closed by Shutdown. Built lazily and pre-seeded into worker clones, the
	// same arrangement pluginRuntime uses and for the same reason — a second
	// holder means a second connection pool on one SQLite file.
	projectStoresOnce sync.Once
	projectStores     *projectStores

	// governance holds the run's governance instant and the validity
	// transitions already reported. Built lazily and pre-seeded into worker
	// clones: the locales of one pass must resolve profile validity at one
	// instant, and must report an expired profile once between them.
	governanceOnce sync.Once
	governance     *governanceRun
}

// ensurePluginRuntime lazily builds the shared plugin Runtime from the current
// flags/env. It performs no discovery itself — callers Rescan when they need
// the host. Safe to call from InitPluginHost or DaemonPool.
func (a *App) ensurePluginRuntime() *pluginhost.Runtime {
	a.pluginRuntimeOnce.Do(func() {
		if a.pluginRuntime != nil {
			// Pre-seeded from a parent App (converge worker clones share the
			// parent's runtime so no second daemon pool is ever built).
			return
		}
		warn := func(s string) {
			if !a.Quiet {
				fmt.Fprintln(os.Stderr, "Warning: "+s)
			}
		}
		// Honor --plugin-dir: when set it takes precedence over KAPI_PLUGINS_DIR
		// so a developer can point at a custom directory without touching env.
		envPluginsDir := os.Getenv("KAPI_PLUGINS_DIR")
		if a.PluginDir != "" {
			envPluginsDir = a.PluginDir
		}
		a.pluginRuntime = pluginhost.NewRuntime(pluginhost.RuntimeOptions{
			Discover:           pluginhost.DiscoverOptions{EnvPluginsDir: envPluginsDir, OnWarn: warn},
			FormatReg:          a.FormatReg,
			OnWarn:             warn,
			RegisterConnectors: true,
			UseCache:           true,
			PoolLogger: func(format string, args ...any) {
				if a.Verbose {
					fmt.Fprintf(os.Stderr, "[daemon] "+format+"\n", args...)
				}
			},
			// When a plugin contributes a segmentation engine, recompose the
			// segmentation tool schema so the new engine appears in the selector
			// (the schema is built once, before plugin discovery).
			OnSegmentersChanged: func() {
				if a.ToolReg != nil {
					libtools.RegisterSegmentation(a.ToolReg)
				}
			},
		})
	})
	return a.pluginRuntime
}

// T returns the active metadata Translator. Safe to call before Init —
// returns a NoopTranslator that passes source text through unchanged.
func (a *App) T() i18n.Translator {
	if a.translator == nil {
		return i18n.NoopTranslator{}
	}
	return a.translator
}

// AddProcessingFlags adds file-processing flags to a command.
func (a *App) AddProcessingFlags(cmd Command) {
	cmd.Flags().StringVarP(&a.FormatFlag, "format", "f", "", "override input format detection")
	a.AddEncodingFlag(cmd.Flags(), "e", "input file encoding")
	a.AddSourceLangFlag(cmd.Flags())
	cmd.Flags().StringVar(&a.TargetLang, "target-lang", "", "target language (e.g. fr, de-DE)")
}

// InitRegistries populates FormatReg, SchemaReg, and ToolReg with every
// built-in format, schema, and tool. It has no flag or config dependency
// and is safe to call at cobra `init()` time — specifically, before
// NewToolCommands() walks the tool registry to build subcommand trees.
// Idempotent: repeat calls are a no-op once the registries exist.
func (a *App) InitRegistries() {
	if a.ToolReg != nil {
		return
	}
	a.FormatReg = registry.NewFormatRegistry()
	a.SchemaReg = schema.NewSchemaRegistry()

	// Single-pass registration: formats, schemas, and config decoders.
	formats.RegisterAll(a.FormatReg, formats.RegisterOptions{
		SchemaReg: a.SchemaReg,
		ConfigReg: neokapiconfig.DefaultRegistry,
	})

	a.ToolReg = registry.NewToolRegistry()
	// libtools first: it registers the deterministic `qa`; aitools then overlays
	// the unified `translate`/`qa` (which dispatch to MT and LLM backends).
	libtools.RegisterAll(a.ToolReg)
	aitools.RegisterAll(a.ToolReg)
}

// InitPluginHost discovers plugins (manifest.json sidecar model) from
// $KAPI_PLUGINS_DIR + $XDG_DATA_HOME/kapi/plugins + system roots and
// builds the host-side dispatch tables. Schema extensions surfaced from
// discovered plugins are registered with core/project so recipe
// validation sees them.
//
// Idempotent: repeat calls are a no-op once the host exists. Safe to
// call from cobra init() — the host attaches its commands before
// rootCmd.Execute() runs.
//
// Cache: when a startup-time cache exists at $XDG_CACHE_HOME/kapi/plugins-cache.json
// and every discovery root's mtime is older than the cache, the cache
// is consumed without rescanning manifests on disk.
func (a *App) InitPluginHost() {
	if a.PluginHost != nil {
		return
	}
	// The Runtime owns the discover→build→wire sequence (schema extensions,
	// source-connector dispatchers, and daemon-backed Mode-C formats) plus the
	// lazy daemon pool — the same path the desktop app uses, so the logic lives
	// in one place (pluginhost.Runtime).
	a.PluginHost = a.ensurePluginRuntime().Rescan()
}

// Init finishes app initialization after flag parsing: credentials,
// config load, format priority overrides, and metadata translator.
// Call this in PersistentPreRunE. InitRegistries runs first (idempotently)
// so Init is safe even when the CLI entry point already called
// InitRegistries at init() time.
//
// Init returns an error when an explicitly-specified --config file cannot be
// read, or when the credential store fails to initialize. Config-file-not-
// found is not an error (the default search paths are optional).
func (a *App) Init() error {
	a.InitRegistries()

	// A recipe key that is nearly a field is preserved like any other unknown
	// one, so nothing in the loader notices `source:` where the field is
	// `source_language:` and the project loads as a valid monolingual one.
	// core/project reports the near misses through this hook rather than
	// printing on its own; the CLI is where they become stderr. See #2223.
	// Once each. A single command loads the recipe several times over — `kapi
	// check` reads it twice before it reads a file — and the same three
	// warnings three times over reads as a fault in the warning.
	var warned sync.Map
	project.OnKeyWarnings = func(recipePath string, warnings []string) {
		for _, w := range warnings {
			if _, seen := warned.LoadOrStore(recipePath+"\x00"+w, true); seen {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", DisplayName(recipePath), w)
		}
	}

	// Install the LLM recorder before any provider is constructed, so --explain
	// sees every call the run makes.
	a.StartExplain()

	// Initialize the shared credential store and wire credential resolution
	// into the tool registry so AI tools auto-resolve from saved credentials.
	a.Credentials = credentials.NewStore(credentials.DefaultPath())
	credStore := a.Credentials
	a.ToolReg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		// Apply the configured default AI provider/model before credential
		// resolution, so `kapi ai-translate` (no --provider) uses the user's
		// chosen default (e.g. local "ollama") instead of the built-in anthropic.
		// Runs only when nothing more specific is set, so precedence stays:
		// flag/inline → recipe defaults → app config → built-in. a.Config is
		// loaded by the time tools run, so reading it here is safe.
		config = ApplyAIDefaults(a.Config, toolName, requires, config)
		config = ApplySourceLocale(a.SourceLocale(), config)
		return credentials.ResolveCredentials(credStore, toolName, requires, config)
	})

	if a.Config == nil {
		a.Config = config.NewAppConfig()
	}
	// Honor an explicit --config / -c file path: point the loader at that
	// exact file instead of the fixed search paths. Without this the flag
	// is bound but silently ignored. An explicit file always wins over the
	// search-path locations.
	if a.CfgFile != "" {
		a.Config.Viper().SetConfigFile(a.CfgFile)
	}
	if err := a.Config.Load(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply format priority overrides from configuration.
	a.applyFormatPriorities(a.Config.FormatPriorities())

	// Build the metadata Translator from --lang / KAPI_LANG / config /
	// POSIX env vars, merging the CLI module's own embedded catalogs
	// (cli/i18n: command help + output chrome) with the framework's
	// builtin catalogs. Manifest-driven plugin catalogs (when we add
	// them) can be merged in by InitPluginHost later.
	a.translator = clii18n.Resolve(i18n.ResolveOptions{
		Flag:           a.Lang,
		ConfigLanguage: a.Config.Language(),
	})
	// Output chrome (table headers, list-status lines) renders through
	// the same Translator.
	output.SetTranslator(a.translator)
	return nil
}

// InstalledPluginList returns the currently loaded manifest-driven plugins
// as project.InstalledPlugin values, suitable for passing to
// project.CheckPlugins or project.PopulatePlugins.
func (a *App) InstalledPluginList() []project.InstalledPlugin {
	if a.PluginHost == nil {
		return nil
	}
	plugins := a.PluginHost.Plugins()
	result := make([]project.InstalledPlugin, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, project.InstalledPlugin{
			Name:    p.Name(),
			Version: p.Manifest.Version,
		})
	}
	return result
}

// applyFormatPriorities applies priority overrides to the format registry.
// Keys can be exact format names or glob patterns (e.g. "okf_*").
func (a *App) applyFormatPriorities(priorities map[string]int) {
	for pattern, priority := range priorities {
		if isGlobPattern(pattern) {
			// Glob pattern — match against all registered format infos.
			for _, info := range a.FormatReg.FormatInfos() {
				if matched, _ := filepath.Match(pattern, string(info.Name)); matched {
					a.FormatReg.SetFormatPriority(info.Name, priority)
				}
			}
		} else {
			a.FormatReg.SetFormatPriority(registry.FormatID(pattern), priority)
		}
	}
}

// isGlobPattern returns true if the string contains glob metacharacters.
// `{` counts because the input resolver expands doublestar alternation
// (`{a,b}`) alongside `*`, `?` and `[…]`.
func isGlobPattern(s string) bool {
	return strings.ContainsAny(s, "*?[{")
}

// AddCommandGroups registers Cobra command groups on the root command for
// sectioned --help output (convergence-first layout, #1078 C1).
//

// Shutdown cleans up plugin resources (stops Mode-C daemons, etc.). Must
// be called before the process exits — typically from main() after
// Execute() returns, to ensure cleanup runs even when RunE returns an
// error.
func (a *App) Shutdown() {
	// Render the --explain transcript before tearing anything down, so the user
	// sees the prompts even when the run itself failed.
	if err := a.FlushExplain(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if a.pluginRuntime != nil {
		a.pluginRuntime.Shutdown()
	}
	// After the plugins: a Mode-C daemon shutting down may still be writing
	// through a store this App handed it.
	a.closeProjectStores()
}

// DaemonPool returns the lazily-constructed Mode-C daemon pool. The
// first call creates the pool with defaults (KAPI_MAX_DAEMONS, manifest
// timeouts). Subsequent calls return the same instance.
//
// Callers (typically plugin command handlers that route to a daemon)
// hold a *DaemonPool reference for the lifetime of the App; the pool
// is torn down by App.Shutdown.
func (a *App) DaemonPool() *pluginhost.DaemonPool {
	return a.ensurePluginRuntime().DaemonPool()
}

// ctxOrBackground returns ctx unless it is nil, in which case it falls back
// to a fresh background context. Host App entry points accept nil from
// embedded/desktop callers (Wails bindings, tests) that have no request
// context; the fallback is a deliberate API convenience, not a detached
// context.
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
