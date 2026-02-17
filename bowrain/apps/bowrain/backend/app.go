package backend

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/platform/config"
	"github.com/gokapi/gokapi/bowrain/connector"
	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/core/locale"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/bowrain/store"
	"github.com/gokapi/gokapi/core/version"
	"github.com/gokapi/gokapi/core/formats"
	sqltm "github.com/gokapi/gokapi/bowrain/sievepen"
	"github.com/gokapi/gokapi/core/termbase"
	libtools "github.com/gokapi/gokapi/core/tools"
	"github.com/gokapi/gokapi/core/plugin/loader"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Bowrain UI backend. It exposes methods that can be
// bound to a Wails frontend or called from tests.
type App struct {
	app          *application.App
	formatReg    *registry.FormatRegistry
	toolReg      *registry.ToolRegistry
	store        store.ContentStore          // persistent SQLite
	tm           *sqltm.SQLiteTM              // lazily initialized
	tb           *termbase.InMemoryTermBase  // lazily initialized
	pluginMu     sync.Mutex
	pluginLoader *loader.PluginLoader
	credentials  *credentials.Store
	connectorReg *connector.Registry
	eventBus     *event.ChannelEventBus

	// Server connection (online mode).
	mu               sync.RWMutex
	remote           *ServerClient           // nil when disconnected
	connState        ConnectionState         // current connection state
	serverURL        string                  // e.g. "http://localhost:8080"
	activeWS         string                  // selected workspace slug
	authInfo         *storedDesktopAuth      // cached auth info
	deviceFlowClient *auth.DeviceFlowClient  // active login flow
	watcher          *ProjectWatcher         // active WatchProject stream
	offlineQueue     *OfflineQueue           // pending mutations when offline
	reconnectCancel  context.CancelFunc      // stops the reconnection goroutine

	// pluginSearchRegistry overrides the registry URL for testing.
	pluginSearchRegistry string
	// tmPath overrides the default TM database path (for testing).
	tmPath string
	// queuePath overrides the default offline queue database path (for testing).
	queuePath string
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
	cs, err := store.NewSQLiteStore(storePath)
	if err != nil {
		log.Printf("bowrain: failed to open store at %s: %v", storePath, err)
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

	connReg := connector.NewRegistry()
	connector.RegisterAll(connReg, reg)

	a := &App{
		formatReg:    reg,
		toolReg:      toolReg,
		store:        cs,
		credentials:  credentials.NewStore(credentials.DefaultPath()),
		connectorReg: connReg,
		eventBus:     event.NewChannelEventBus(),
		connState:    StateDisconnected,
	}

	// Initialize the offline queue for queuing mutations when disconnected.
	queuePath := defaultQueuePath()
	if q, err := NewOfflineQueue(queuePath); err != nil {
		log.Printf("bowrain: failed to open offline queue at %s: %v", queuePath, err)
	} else {
		a.offlineQueue = q
	}

	return a
}

// LoadPlugins discovers and loads plugins from the configured plugin directory.
// This may start JVM subprocesses for Java bridge plugins and can take several
// seconds. Safe to call from a goroutine.
func (a *App) LoadPlugins() {
	pluginDir := os.Getenv("KAPI_PLUGIN_DIR")
	if pluginDir == "" {
		pluginDir = config.NewAppConfig().PluginDirectory()
	}

	pl := loader.NewPluginLoader(pluginDir, nil)
	if err := pl.LoadAll(a.formatReg, nil); err != nil {
		log.Printf("bowrain: failed to load plugins: %v", err)
	}

	a.pluginMu.Lock()
	a.pluginLoader = pl
	a.pluginMu.Unlock()
}

// SetApplication stores the Wails v3 application reference for dialog and event access.
func (a *App) SetApplication(app *application.App) {
	a.app = app
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

// ToolInfo describes an available tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
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
			Name:        ri.Name,
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
	"tag-protect": "enrich",
	"tm-leverage": "enrich",
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
			category := toolCategory[name]
			if category == "" {
				category = "utility"
			}
			result = append(result, ToolInfo{
				Name:        name,
				Description: t.Description(),
				Category:    category,
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

// ListPlugins returns all loaded plugins.
func (a *App) ListPlugins() []PluginInfo {
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl == nil {
		return []PluginInfo{}
	}
	raw := pl.Plugins()
	out := make([]PluginInfo, len(raw))
	for i, p := range raw {
		out[i] = PluginInfo{
			Name:    p.Name,
			Type:    p.Type,
			Source:  p.Source,
			Formats: p.Formats,
		}
	}
	return out
}

// PluginDir returns the configured plugin directory path.
func (a *App) PluginDir() string {
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl == nil {
		return ""
	}
	return pl.Dir()
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
	a.pluginMu.Lock()
	pl := a.pluginLoader
	a.pluginMu.Unlock()
	if pl != nil {
		pl.Shutdown()
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

// defaultStorePath returns the path for the persistent SQLite store.
func defaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	dir := filepath.Join(home, ".config", "bowrain")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "bowrain.db")
}

// DetectFormat detects the format of a file by its extension.
func (a *App) DetectFormat(filePath string) (string, error) {
	ext := filepath.Ext(filePath)
	return a.formatReg.Detector().DetectByExtension(ext)
}

func createProvider(name, apiKey, modelName string) provider.LLMProvider {
	cfg := provider.Config{
		APIKey: apiKey,
		Model:  modelName,
	}
	switch name {
	case "anthropic":
		return provider.NewAnthropicProvider(cfg)
	case "openai":
		return provider.NewOpenAIProvider(cfg)
	case "ollama":
		return provider.NewOllamaProvider(cfg)
	default:
		return provider.NewMockProvider()
	}
}
