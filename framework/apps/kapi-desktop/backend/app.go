// Package backend provides the Wails v3 service for Kapi Desktop.
// It exposes format/tool registries, plugin management, credential storage,
// .kapi project file operations, and flow execution to the React frontend.
package backend

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/core/flow"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	fmtschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/plugin/loader"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the Wails service that bridges Go backend to the React frontend.
type App struct {
	app          *application.App
	formatReg    *registry.FormatRegistry
	toolReg      *registry.ToolRegistry
	schemaReg    *fmtschema.SchemaRegistry
	pluginLoader *loader.PluginLoader

	// Current open project
	mu          sync.RWMutex
	project     *project.KapiProject
	projectPath string

	// Flow runner
	runState *runner

	// Plugin registry (lazily initialized)
	registryMu sync.Mutex
	registry   *pluginreg.RemoteRegistry

	// Persistence
	credentials *credentials.Store
	recent      *recentStore
	settings    *settingsStore

	logger *log.Logger
}

// NewApp creates a new Kapi Desktop backend service.
// Plugins are loaded later via LoadPlugins().
func NewApp() *App {
	formatReg := registry.NewFormatRegistry()
	schemaReg := fmtschema.NewSchemaRegistry()
	formats.RegisterAll(formatReg, formats.RegisterOptions{
		SchemaReg: schemaReg,
	})

	toolReg := registry.NewToolRegistry()
	libtools.RegisterAll(toolReg)

	logger := log.New(os.Stderr, "[kapi-desktop] ", log.LstdFlags)
	pluginDir := defaultPluginDir()
	pluginLoader := loader.NewPluginLoader(pluginDir, logger)

	return &App{
		formatReg:    formatReg,
		toolReg:      toolReg,
		schemaReg:    schemaReg,
		pluginLoader: pluginLoader,
		credentials:  credentials.NewStore(credentials.DefaultPath()),
		recent:       newRecentStore(),
		settings:     newSettingsStore(),
		logger:       logger,
	}
}

// SetApplication stores the Wails app reference for dialogs and events.
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// LoadPlugins scans and loads plugins in the background.
func (a *App) LoadPlugins() {
	if err := a.pluginLoader.ScanMetadata(); err != nil {
		a.logger.Printf("plugin scan: %v", err)
	}
	a.emitEvent("plugins-loaded", nil)
}

// --- Project operations ---

// NewProject creates a new empty .kapi project in memory.
func (a *App) NewProject(name, sourceLang string, targetLangs []string) (*project.KapiProject, error) {
	proj := &project.KapiProject{
		Version:         project.CurrentVersion,
		Name:            name,
		SourceLanguage:  sourceLang,
		TargetLanguages: targetLangs,
		Flows:           make(map[string]*flow.StepsSpec),
	}
	a.mu.Lock()
	a.project = proj
	a.projectPath = ""
	a.mu.Unlock()
	return proj, nil
}

// OpenProject loads a .kapi file from disk.
func (a *App) OpenProject(path string) (*project.KapiProject, error) {
	proj, err := project.Load(path)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.project = proj
	a.projectPath = path
	a.mu.Unlock()

	a.recent.add(path, proj.Name)
	return proj, nil
}

// SaveProject writes the current project to its file path.
func (a *App) SaveProject() error {
	a.mu.RLock()
	proj := a.project
	path := a.projectPath
	a.mu.RUnlock()

	if proj == nil {
		return fmt.Errorf("no project open")
	}
	if path == "" {
		return fmt.Errorf("project has no file path (use SaveProjectAs)")
	}
	return project.Save(path, proj)
}

// SaveProjectAs writes the current project to a new file path.
func (a *App) SaveProjectAs(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.project == nil {
		return fmt.Errorf("no project open")
	}
	if err := project.Save(path, a.project); err != nil {
		return err
	}
	a.projectPath = path
	return nil
}

// GetProject returns the currently open project, or nil.
func (a *App) GetProject() *project.KapiProject {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.project
}

// GetProjectPath returns the file path of the open project.
func (a *App) GetProjectPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.projectPath
}

// --- Flow operations ---

// FlowInfo is the frontend-facing flow summary.
type FlowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	StepCount   int    `json:"step_count"`
}

// ListFlows returns all flows in the current project.
func (a *App) ListFlows() []FlowInfo {
	a.mu.RLock()
	proj := a.project
	a.mu.RUnlock()

	if proj == nil {
		return nil
	}

	var infos []FlowInfo
	for name, spec := range proj.Flows {
		infos = append(infos, FlowInfo{
			Name:      name,
			StepCount: len(spec.Steps),
		})
	}
	return infos
}

// GetFlow returns a flow's StepsSpec by name.
func (a *App) GetFlow(name string) *flow.StepsSpec {
	a.mu.RLock()
	proj := a.project
	a.mu.RUnlock()

	if proj == nil {
		return nil
	}
	return proj.GetFlow(name)
}

// SaveFlow saves or updates a flow in the current project.
func (a *App) SaveFlow(name string, spec *flow.StepsSpec) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.project == nil {
		return fmt.Errorf("no project open")
	}
	if a.project.Flows == nil {
		a.project.Flows = make(map[string]*flow.StepsSpec)
	}
	a.project.Flows[name] = spec
	return nil
}

// DeleteFlow removes a flow from the current project.
func (a *App) DeleteFlow(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.project == nil {
		return fmt.Errorf("no project open")
	}
	delete(a.project.Flows, name)
	return nil
}

// --- Tool & format operations ---

// ToolInfo is the frontend-facing tool summary.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	HasSchema   bool   `json:"has_schema"`
}

// ListTools returns all registered tools.
func (a *App) ListTools() []ToolInfo {
	var infos []ToolInfo
	for _, info := range a.toolReg.ListWithSchemas() {
		infos = append(infos, ToolInfo{
			Name:        info.Name,
			Description: info.Description,
			Category:    info.Category,
			HasSchema:   info.HasSchema,
		})
	}
	return infos
}

// GetToolSchema returns the component schema for a tool's parameters.
func (a *App) GetToolSchema(name string) *schema.ComponentSchema {
	return a.toolReg.GetSchema(name)
}

// FormatInfo is the frontend-facing format summary.
type FormatInfo struct {
	Name string `json:"name"`
}

// ListFormats returns all registered format reader names.
func (a *App) ListFormats() []FormatInfo {
	var infos []FormatInfo
	for _, name := range a.formatReg.ReaderNames() {
		infos = append(infos, FormatInfo{Name: name})
	}
	return infos
}

// --- Plugin operations ---

// PluginInfo is the frontend-facing plugin summary.
type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

// ListPlugins returns installed plugins.
func (a *App) ListPlugins() []PluginInfo {
	var infos []PluginInfo
	for _, p := range a.pluginLoader.Plugins() {
		infos = append(infos, PluginInfo{
			Name:    p.Name,
			Version: p.Version,
			Type:    p.Type,
		})
	}
	return infos
}

// --- Utility ---

// GetVersion returns the kapi version string.
func (a *App) GetVersion() string {
	return version.Version
}

func (a *App) emitEvent(name string, data any) {
	if a.app != nil {
		a.app.Event.Emit(name, data)
	}
}

func defaultPluginDir() string {
	dir := os.Getenv("KAPI_PLUGIN_DIR")
	if dir != "" {
		return dir
	}
	home, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "kapi", "plugins")
}
