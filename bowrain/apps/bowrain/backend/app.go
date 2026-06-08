package backend

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/bowrain/connector"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Bowrain UI backend. It exposes methods that can be
// bound to a Wails frontend or called from tests.
type App struct {
	app          *application.App
	formatReg    *registry.FormatRegistry
	toolReg      *registry.ToolRegistry
	store        store.ContentStore       // persistent SQLite
	tm           *sievepen.SQLiteTM       // lazily initialized
	tb           *termbase.SQLiteTermBase // lazily initialized
	pluginMu     sync.Mutex
	pluginDir    string             // resolved plugin directory
	plugins      []manifestedPlugin // discovered manifest plugins (lazy)
	connectorReg *platconn.Registry
	eventBus     *event.ChannelEventBus

	// Server connection (online mode).
	mu              sync.RWMutex
	remote          *ServerClient      // nil when disconnected
	connState       ConnectionState    // current connection state
	serverURL       string             // e.g. "http://localhost:8080"
	activeWS        string             // selected workspace slug
	authInfo        *storedDesktopAuth // cached auth info
	pkceVerifier    string             // PKCE code_verifier
	pkceResultCh    chan *pkceResult   // result from URL protocol callback
	watcher         *ProjectWatcher    // active WatchProject stream
	offlineQueue    *OfflineQueue      // pending mutations when offline
	reconnectCancel context.CancelFunc // stops the reconnection goroutine
	autoConnectDone bool               // true after BOWRAIN_TOKEN auto-connect attempted

	// tmPath overrides the default TM database path (for testing).
	tmPath string
	// queuePath overrides the default offline queue database path (for testing).
	queuePath string

	// eventSink, when set, receives every backend event in addition to the
	// Wails runtime. The recording wbridge (cmd/wbridge) uses it to stream
	// events to a browser over SSE, since there is no Wails runtime there.
	eventSink func(name string, data any)
}

// NewApp creates a new Bowrain backend with all formats and plugins registered.
// This blocks until plugin loading completes (which may start a JVM subprocess).
// For GUI use, prefer NewAppWithoutPlugins followed by LoadPlugins in a goroutine.
func NewApp() *App {
	a := NewAppWithoutPlugins()
	a.LoadPlugins()
	return a
}

// NewAppWithoutPlugins creates a Bowrain backend with built-in formats and tools
// registered but without loading plugins. Call LoadPlugins separately (possibly
// in a background goroutine) to discover and register plugins.
func NewAppWithoutPlugins() *App {
	// Initialize persistent store.
	storePath := defaultStorePath()
	cs, err := bstore.NewSQLiteStore(storePath)
	if err != nil {
		slog.Info("bowrain: failed to open store at", "id", storePath, "error", err)
	}
	return newAppWithStore(cs)
}

// newAppWithStore creates a Bowrain backend with the given ContentStore.
// This is used by NewAppWithoutPlugins (production) and tests (in-memory store).
func newAppWithStore(cs store.ContentStore) *App {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	// The desktop app only offers remote/CMS connectors. The local-filesystem
	// connectors (file, git) are server-side only: under the product boundary
	// kapi owns local files + project configuration, and the desktop's local
	// footprint is a working copy / cache of the server, never a source of
	// truth. (The Bowrain server registers every connector via RegisterAll.)
	connReg := platconn.NewRegistry()
	connector.RegisterRemote(connReg)

	a := &App{
		formatReg:    reg,
		toolReg:      toolReg,
		store:        cs,
		connectorReg: connReg,
		eventBus:     event.NewChannelEventBus(),
		connState:    StateDisconnected,
	}

	// Initialize the offline queue for queuing mutations when disconnected.
	queuePath := defaultQueuePath()
	if q, err := NewOfflineQueue(queuePath); err != nil {
		slog.Info("bowrain: failed to open offline queue at", "id", queuePath, "error", err)
	} else {
		a.offlineQueue = q
	}

	return a
}

// manifestedPlugin pairs a discovered manifest with its install dir so
// the desktop UI can list installed plugins without needing the full
// pluginhost machinery (which lives in the cli/ module that bowrain
// must not import).
type manifestedPlugin struct {
	Dir      string
	Manifest *manifest.Manifest
}

// LoadPlugins discovers manifest-driven plugins from the configured
// plugin directory and caches the result. Safe to call from a goroutine.
// Errors are logged and otherwise ignored — a missing or empty plugin
// dir is a normal state for a fresh install.
func (a *App) LoadPlugins() {
	dir := os.Getenv("KAPI_PLUGIN_DIR")
	if dir == "" {
		dir = defaultPluginDir()
	}

	plugins := discoverManifestPlugins(dir)

	a.pluginMu.Lock()
	a.pluginDir = dir
	a.plugins = plugins
	a.pluginMu.Unlock()
}

// discoverManifestPlugins walks dir/<plugin>/<version>/manifest.json
// entries (the layout `kapi plugin install` writes) and returns a flat
// list of valid manifests. Invalid manifests are skipped silently —
// other tooling surfaces those errors.
func discoverManifestPlugins(dir string) []manifestedPlugin {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []manifestedPlugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginName := e.Name()
		versionsDir := filepath.Join(dir, pluginName)
		versions, err := os.ReadDir(versionsDir)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			pluginDir := filepath.Join(versionsDir, v.Name())
			data, err := os.ReadFile(filepath.Join(pluginDir, "manifest.json"))
			if err != nil {
				continue
			}
			m, err := manifest.Parse(data)
			if err != nil || m == nil {
				continue
			}
			out = append(out, manifestedPlugin{Dir: pluginDir, Manifest: m})
		}
	}
	return out
}

// defaultPluginDir returns the default plugin directory.
func defaultPluginDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "kapi", "plugins")
	}
	return "./plugins"
}

// SetApplication stores the Wails v3 application reference for dialog and event access.
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// SetEventSink registers a function that receives every backend event in
// addition to the Wails runtime. Used by the recording wbridge to forward
// events to a browser over SSE (the Wails runtime is webview-only). Passing nil
// clears it.
func (a *App) SetEventSink(fn func(name string, data any)) {
	a.eventSink = fn
}

// emit delivers a backend event to the Wails runtime (when running as the
// desktop app) and to the event sink (when running under the wbridge). Either
// may be absent; emit is safe to call in both modes.
func (a *App) emit(name string, data any) {
	if a.app != nil {
		a.app.Event.Emit(name, data)
	}
	if a.eventSink != nil {
		a.eventSink(name, data)
	}
}

// VersionInfo describes the application version.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// GetVersion returns the application version information.
func (a *App) GetVersion() VersionInfo {
	return VersionInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		BuildDate: version.BuildDate,
	}
}

// FormatInfo describes a registered data format.
type FormatInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source"`
}

// IOPort is one entry of a tool's IO contract (mirrors core/schema.IOPort).
type IOPort struct {
	Type     string `json:"type"`
	Side     string `json:"side,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Layer    string `json:"layer,omitempty"`
}

// ToolInfo describes an available tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	// Consumes / Produces are the tool's IO contract (AD-006), surfaced
	// to the flow editor so it can type ports and validate connections.
	Consumes []IOPort `json:"consumes,omitempty"`
	Produces []IOPort `json:"produces,omitempty"`
	// IsSourceTransform reports whether this tool may be placed in the
	// source-transform stage of a flow (i.e. it rewrites the source model).
	IsSourceTransform bool `json:"is_source_transform,omitempty"`
}

// ioPorts converts a schema IO contract to the wire form.
func ioPorts(fs []schema.IOPort) []IOPort {
	if len(fs) == 0 {
		return nil
	}
	out := make([]IOPort, len(fs))
	for i, f := range fs {
		out[i] = IOPort{Type: string(f.Type), Side: f.Side.String(), Optional: f.Optional, Layer: f.Layer}
	}
	return out
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Source  string   `json:"source"`
	Formats []string `json:"formats"`
}

// GetKnownLocales returns a curated list of well-known BCP-47 locales with display names.
func (a *App) GetKnownLocales() []locale.LocaleInfo {
	return locale.WellKnownLocales()
}

// GetLocaleDisplayName returns the English display name for a BCP-47 locale code.
func (a *App) GetLocaleDisplayName(code string) string {
	return locale.DisplayName(model.LocaleID(code))
}

// ListFormats returns all registered formats with metadata.
func (a *App) ListFormats() []FormatInfo {
	regInfos := a.formatReg.FormatInfos()
	result := make([]FormatInfo, len(regInfos))
	for i, ri := range regInfos {
		result[i] = FormatInfo{
			Name:        string(ri.Name),
			DisplayName: ri.DisplayName,
			MimeTypes:   ri.MimeTypes,
			Extensions:  ri.Extensions,
			HasReader:   ri.HasReader,
			HasWriter:   ri.HasWriter,
			Source:      ri.Source,
		}
	}
	return result
}

// toolCategory maps tool names to their categories.
var toolCategory = map[string]string{
	// utility tools
	"word-count":      "utility",
	"char-count":      "utility",
	"segment-count":   "utility",
	"encoding-detect": "utility",
	// transform tools
	"pseudo-translate": "transform",
	"search-replace":   "transform",
	"case-transform":   "transform",
	"xslt-transform":   "transform",
	"segmentation":     "transform",
	// validate tools
	"xml-validation": "validate",
	"qa-check":       "validate",
	"term-check":     "validate",
	// enrich tools
	"tag-protect":     "enrich",
	"tm-leverage":     "enrich",
	"layer-processor": "transform",
	// AI tools
	"ai-translate":   "transform",
	"ai-qa":          "validate",
	"ai-terminology": "enrich",
	"ai-review":      "validate",
}

// ListTools returns all available tools.
func (a *App) ListTools() []ToolInfo {
	var result []ToolInfo

	// Add tools from the registry.
	if a.toolReg != nil {
		for _, name := range a.toolReg.Names() {
			t, err := a.toolReg.NewTool(name)
			if err != nil {
				continue
			}
			category := toolCategory[string(name)]
			if category == "" {
				category = "utility"
			}
			// IsSourceTransform comes from the registry metadata (probed at
			// registration from tool.CapTransform capability).
			var isSourceTransform bool
			var consumes, produces []IOPort
			if info := a.toolReg.ToolInfo(name); info != nil {
				isSourceTransform = info.IsSourceTransform
				consumes = ioPorts(info.Consumes)
				produces = ioPorts(info.Produces)
			}
			result = append(result, ToolInfo{
				Name:              string(name),
				Description:       t.Description(),
				Category:          category,
				Consumes:          consumes,
				Produces:          produces,
				IsSourceTransform: isSourceTransform,
			})
		}
	}

	// AI tools (not in tool registry, managed separately).
	aiTools := []ToolInfo{
		{Name: "ai-translate", Description: "Translate content using AI/LLM", Category: "transform"},
		{Name: "ai-qa", Description: "Quality check translations using AI", Category: "validate"},
		{Name: "ai-terminology", Description: "Extract terminology using AI", Category: "enrich"},
		{Name: "ai-review", Description: "Review translations using AI", Category: "validate"},
	}
	result = append(result, aiTools...)

	return result
}

// ListPlugins returns the manifest-driven plugins discovered in the
// configured plugin directory.
func (a *App) ListPlugins() []PluginInfo {
	a.pluginMu.Lock()
	plugins := a.plugins
	a.pluginMu.Unlock()
	out := make([]PluginInfo, len(plugins))
	for i, p := range plugins {
		var formats []string
		for _, f := range p.Manifest.Capabilities.Formats {
			formats = append(formats, f.Name)
		}
		out[i] = PluginInfo{
			Name:    p.Manifest.Plugin,
			Type:    pluginTypeFromManifest(p.Manifest),
			Source:  p.Dir,
			Formats: formats,
		}
	}
	return out
}

// pluginTypeFromManifest classifies a manifest plugin by its dominant
// capability so the desktop UI can group entries (matches the legacy
// "format" / "tool" / "bundle" labels for visual continuity).
func pluginTypeFromManifest(m *manifest.Manifest) string {
	if m == nil {
		return ""
	}
	hasFmt := len(m.Capabilities.Formats) > 0
	hasCmd := len(m.Capabilities.Commands) > 0 || len(m.Capabilities.MCPTools) > 0
	switch {
	case hasFmt && hasCmd:
		return "bundle"
	case hasFmt:
		return "format"
	case hasCmd:
		return "tool"
	}
	return ""
}

// PluginDir returns the configured plugin directory path.
func (a *App) PluginDir() string {
	a.pluginMu.Lock()
	defer a.pluginMu.Unlock()
	return a.pluginDir
}

// ServiceShutdown is called by Wails v3 when the application exits.
func (a *App) ServiceShutdown() error {
	// Stop reconnection goroutine.
	a.stopReconnect()
	// Stop project watcher and close server connection.
	a.StopWatching()
	if a.remote != nil {
		a.remote.Close()
	}
	if a.offlineQueue != nil {
		a.offlineQueue.Close()
	}
	if a.tm != nil {
		a.tm.Close()
	}
	if a.tb != nil {
		a.tb.Close()
	}
	if a.store != nil {
		a.store.Close()
	}
	return nil
}

// stopReconnect cancels the reconnection goroutine if running.
func (a *App) stopReconnect() {
	a.mu.Lock()
	cancel := a.reconnectCancel
	a.reconnectCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// desktopConfigDir returns the bowrain-desktop config directory.
// Respects BOWRAIN_DESKTOP_CONFIG_DIR env var for testing.
func desktopConfigDir() string {
	if dir := os.Getenv("BOWRAIN_DESKTOP_CONFIG_DIR"); dir != "" {
		return dir
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(configDir, "bowrain-desktop")
}

// defaultStorePath returns the path for the persistent SQLite store.
func defaultStorePath() string {
	dir := desktopConfigDir()
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "bowrain.db")
}

// DetectFormat detects the format of a file by its extension.
func (a *App) DetectFormat(filePath string) (string, error) {
	ext := filepath.Ext(filePath)
	return a.formatReg.Detector().DetectByExtension(ext)
}
