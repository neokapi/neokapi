// Package cli provides a shared CLI base for neokapi CLI tools.
// CLI tools build on this package, selecting which commands to expose.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/cli/pluginhost"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	neokapiconfig "github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/i18n"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
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
	AssumeYes bool // --yes / -y; auto-confirm prompts (e.g. plugin auto-install)
	CfgFile   string
	PluginDir string
	Lang      string // --lang / KAPI_LANG; feeds i18n.Resolve

	// Processing flags bound by AddProcessingFlags.
	FormatFlag string
	Encoding   string
	SourceLang string
	TargetLang string

	// TMBackend, when non-nil, is returned by openTMSQLite instead of
	// opening a SQLite database. Used by the WASM browser build to inject
	// a pre-seeded InMemoryTM so the tm / extract commands work without cgo.
	TMBackend sievepen.TMStore

	// TBBackend, when non-nil, is returned by openTermbaseSQLite instead
	// of opening a SQLite database. Used by the WASM browser build to
	// inject a pre-seeded InMemoryTermBase so termbase / term-check work
	// without cgo.
	TBBackend termbase.TermBase

	// Credentials is the shared credential store for AI provider keys.
	Credentials *credentials.Store

	// RegistryResolver is an optional hook for resolving plugin registries.
	// When set, it is called before falling back to the config-based registries.
	RegistryResolver func() []config.RegistryEntry

	// FallbackRunE is called by NewRunCmd / NewFlowsCmd when the named
	// flow is not a built-in flow. Plugins (e.g. bowrain) install this
	// via RegisterAppInitializer to support project-defined flows.
	// Read by NewRunCmd / NewFlowsCmd as a default when the per-command
	// CmdOptions struct does not explicitly set it.
	FallbackRunE func(cmd *cobra.Command, flowName string, args []string) error

	// ExtraFlows returns additional flow infos for the `flows` command.
	// Plugins install this via RegisterAppInitializer. Read by
	// NewFlowsCmd as a default when the per-command CmdOptions struct
	// does not explicitly set it.
	ExtraFlows func() []output.FlowInfo

	// projectContext is set temporarily by runFromProject so that downstream
	// methods (reader creation, writer setup) can apply project format defaults.
	projectContext *project.ProjectContext

	// projectFlowTools is set temporarily by runProjectSteps to override
	// buildFlowTools for project-defined flows.
	projectFlowTools []tool.Tool

	// projectBindings carries the standing brand-voice + termbase context
	// resolved from a .kapi project (defaults.brand_voice / defaults.termbase).
	// Set temporarily by runFromProject so project-flow steps can be made
	// brand- and terminology-aware with no flags. nil for ad-hoc runs.
	projectBindings *projectBindings

	// translator localizes tool/format/plugin metadata at API egress.
	// Built during Init from --lang / KAPI_LANG / config / POSIX env.
	// Never nil after Init — unresolved locales get a NoopTranslator
	// so T() calls are always safe.
	translator i18n.Translator

	// daemonPool is the lazily-initialized Mode-C daemon pool. Built on
	// first DaemonPool() call; torn down by Shutdown.
	daemonPoolMu sync.Mutex
	daemonPool   *pluginhost.DaemonPool
}

// T returns the active metadata Translator. Safe to call before Init —
// returns a NoopTranslator that passes source text through unchanged.
func (a *App) T() i18n.Translator {
	if a.translator == nil {
		return i18n.NoopTranslator{}
	}
	return a.translator
}

// AddPersistentFlags registers global flags on the root command.
func (a *App) AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&a.CfgFile, "config", "c", "", "config file path")
	cmd.PersistentFlags().BoolVarP(&a.Verbose, "verbose", "v", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&a.Quiet, "quiet", "q", false, "suppress output")
	cmd.PersistentFlags().BoolVarP(&a.AssumeYes, "yes", "y", false, "assume yes for confirmation prompts (e.g. plugin auto-install)")
	cmd.PersistentFlags().StringVar(&a.PluginDir, "plugin-dir", "", "plugin directory")
	cmd.PersistentFlags().StringVar(&a.Lang, "lang", "", "UI locale for tool/format/plugin metadata (BCP-47, e.g. fr-FR); falls back to KAPI_LANG / LC_ALL / LANG")
	output.AddPersistentFlags(cmd)
}

// AddProcessingFlags adds file-processing flags to a command.
func (a *App) AddProcessingFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&a.FormatFlag, "format", "f", "", "override input format detection")
	cmd.Flags().StringVarP(&a.Encoding, "encoding", "e", "UTF-8", "input file encoding")
	cmd.Flags().StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
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
	libtools.RegisterAll(a.ToolReg)
	aitools.RegisterAll(a.ToolReg)
	mttools.RegisterAll(a.ToolReg)
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
	opts := pluginhost.DiscoverOptions{
		EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR"),
		OnWarn: func(s string) {
			if !a.Quiet {
				fmt.Fprintln(os.Stderr, "Warning: "+s)
			}
		},
	}

	var plugins []*pluginhost.Plugin
	if cache, err := pluginhost.LoadCache(pluginhost.CacheLocation()); err == nil && pluginhost.IsFresh(cache, opts) {
		plugins = pluginhost.PluginsFromCache(cache)
	} else {
		plugins = pluginhost.Discover(opts)
		// Best-effort cache write; ignore errors so an unwritable
		// cache dir doesn't break startup.
		_ = pluginhost.SaveCache(pluginhost.CacheLocation(), pluginhost.BuildCache(opts, plugins))
	}

	a.PluginHost = pluginhost.NewHost(plugins, func(s string) {
		if !a.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+s)
		}
	})

	pluginhost.RegisterSchemaExtensions(a.PluginHost, func(s string) {
		if !a.Quiet {
			fmt.Fprintln(os.Stderr, "Warning: "+s)
		}
	})

	// Auto-register a generic source-connector dispatcher for every
	// Mode-C plugin that declares source_connectors. Plugins that need
	// custom dispatch can override this by re-registering after Init.
	for _, p := range a.PluginHost.Plugins() {
		if !p.Manifest.IsModeC() {
			continue
		}
		if len(p.Manifest.Capabilities.SourceConnectors) == 0 {
			continue
		}
		pluginhost.RegisterSourceConnectorDispatcher(
			pluginhost.NewGenericSourceConnectorDispatcher(p.Name()),
			pluginhost.SourceConnectorOpsClaimed...,
		)
	}

	// Register daemon-backed format readers and writers from every
	// Mode-C plugin that declares formats. This makes plugin-provided
	// formats first-class participants in format detection, reader /
	// writer construction, and `kapi formats list`. The daemon pool is
	// constructed eagerly here so the factories close over a non-nil
	// pool reference; daemon processes themselves stay lazy and only
	// spawn on first format use.
	if a.FormatReg != nil {
		pluginhost.RegisterModeCFormats(a.PluginHost, a.DaemonPool(), a.FormatReg)
	}
}

// Init finishes app initialization after flag parsing: credentials,
// config load, format priority overrides, and metadata translator.
// Call this in PersistentPreRun. InitRegistries runs first (idempotently)
// so Init is safe even when the CLI entry point already called
// InitRegistries at init() time.
func (a *App) Init() {
	a.InitRegistries()

	// Initialize the shared credential store and wire credential resolution
	// into the tool registry so AI tools auto-resolve from saved credentials.
	a.Credentials = credentials.NewStore(credentials.DefaultPath())
	credStore := a.Credentials
	a.ToolReg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		return credentials.ResolveCredentials(credStore, requires, config)
	})

	if a.Config == nil {
		a.Config = config.NewAppConfig()
	}
	_ = a.Config.Load()

	// Apply format priority overrides from configuration.
	a.applyFormatPriorities(a.Config.FormatPriorities())

	// Build the metadata Translator from --lang / KAPI_LANG / config /
	// POSIX env vars. Manifest-driven plugin catalogs (when we add them)
	// can be merged in by InitPluginHost later.
	a.translator = i18n.Resolve(i18n.ResolveOptions{
		Flag:           a.Lang,
		ConfigLanguage: a.Config.Language(),
	})
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
func isGlobPattern(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}

// AddCommandGroups registers Cobra command groups on the root command for
// sectioned --help output. Group IDs match the Category field on ToolCommandDef
// and plugin metadata.
func (a *App) AddCommandGroups(cmd *cobra.Command) {
	cmd.AddGroup(
		&cobra.Group{ID: "processing", Title: "Processing:"},
		&cobra.Group{ID: "translation", Title: "Translation:"},
		&cobra.Group{ID: "quality", Title: "Quality:"},
		&cobra.Group{ID: "analysis", Title: "Analysis:"},
		&cobra.Group{ID: "text-processing", Title: "Text Processing:"},
		&cobra.Group{ID: "content", Title: "Project & Content:"},
		&cobra.Group{ID: "management", Title: "Info & Management:"},
	)
}

// Shutdown cleans up plugin resources (stops Mode-C daemons, etc.). Must
// be called before the process exits — typically from main() after
// Execute() returns, to ensure cleanup runs even when RunE returns an
// error.
func (a *App) Shutdown() {
	a.daemonPoolMu.Lock()
	pool := a.daemonPool
	a.daemonPool = nil
	a.daemonPoolMu.Unlock()
	if pool != nil {
		pool.Shutdown()
	}
}

// DaemonPool returns the lazily-constructed Mode-C daemon pool. The
// first call creates the pool with defaults (KAPI_MAX_DAEMONS, manifest
// timeouts). Subsequent calls return the same instance.
//
// Callers (typically plugin command handlers that route to a daemon)
// hold a *DaemonPool reference for the lifetime of the App; the pool
// is torn down by App.Shutdown.
func (a *App) DaemonPool() *pluginhost.DaemonPool {
	a.daemonPoolMu.Lock()
	defer a.daemonPoolMu.Unlock()
	if a.daemonPool == nil {
		a.daemonPool = pluginhost.NewDaemonPool(pluginhost.DaemonPoolOptions{
			Logger: func(format string, args ...any) {
				if a.Verbose {
					fmt.Fprintf(os.Stderr, "[daemon] "+format+"\n", args...)
				}
			},
		})
	}
	return a.daemonPool
}
