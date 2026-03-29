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

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/id"
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

	// Open projects (multiple tabs)
	mu       sync.RWMutex
	projects map[string]*openProject // keyed by tab ID

	// Flow runner
	runState *runner

	// Plugin registry (lazily initialized)
	registryMu sync.Mutex
	registry   *pluginreg.RemoteRegistry

	// Persistence
	credentials *CredentialStore
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
		projects:     make(map[string]*openProject),
		credentials:  NewCredentialStore(DefaultCredentialPath()),
		recent:       newRecentStore(),
		settings:     newSettingsStore(),
		logger:       logger,
	}
}

// openProject holds the state for a single open project tab.
type openProject struct {
	ID      string
	Path    string
	Project *project.KapiProject
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

// --- Project operations (multi-tab) ---

// TabInfo is returned to the frontend to describe an open tab.
type TabInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// NewProject creates a new empty project and returns its tab ID.
func (a *App) NewProject(name, sourceLang string, targetLangs []string) (*TabInfo, error) {
	proj := &project.KapiProject{
		Version:         project.CurrentVersion,
		Name:            name,
		SourceLanguage:  sourceLang,
		TargetLanguages: targetLangs,
		Flows:           make(map[string]*flow.StepsSpec),
	}
	tabID := id.New()

	a.mu.Lock()
	a.projects[tabID] = &openProject{ID: tabID, Project: proj}
	a.mu.Unlock()

	return &TabInfo{ID: tabID, Name: name}, nil
}

// OpenProject loads a .kapi file from disk and returns its tab ID.
// If the file is already open in another tab, returns that tab's ID.
func (a *App) OpenProject(path string) (*TabInfo, error) {
	// Check if already open.
	a.mu.RLock()
	for _, op := range a.projects {
		if op.Path == path {
			a.mu.RUnlock()
			return &TabInfo{ID: op.ID, Name: op.Project.Name, Path: op.Path}, nil
		}
	}
	a.mu.RUnlock()

	proj, err := project.Load(path)
	if err != nil {
		return nil, err
	}
	tabID := id.New()

	a.mu.Lock()
	a.projects[tabID] = &openProject{ID: tabID, Path: path, Project: proj}
	a.mu.Unlock()

	a.recent.add(path, proj.Name)
	return &TabInfo{ID: tabID, Name: proj.Name, Path: path}, nil
}

// UpdateProject replaces the in-memory project state for a tab.
// Called by the frontend to sync edits (content patterns, languages, etc.)
// back to the backend before operations like MatchContent.
func (a *App) UpdateProject(tabID string, proj *project.KapiProject) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	op.Project = proj
	return nil
}

// CloseProject removes a project tab.
func (a *App) CloseProject(tabID string) {
	a.mu.Lock()
	delete(a.projects, tabID)
	a.mu.Unlock()
}

// ListTabs returns all open project tabs.
func (a *App) ListTabs() []TabInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var tabs []TabInfo
	for _, op := range a.projects {
		tabs = append(tabs, TabInfo{ID: op.ID, Name: op.Project.Name, Path: op.Path})
	}
	return tabs
}

// SaveProject writes a project to its file path.
func (a *App) SaveProject(tabID string) error {
	a.mu.RLock()
	op := a.projects[tabID]
	a.mu.RUnlock()

	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if op.Path == "" {
		return fmt.Errorf("project has no file path (use SaveProjectAs)")
	}
	return project.Save(op.Path, op.Project)
}

// SaveProjectAs writes a project to a new file path.
func (a *App) SaveProjectAs(tabID, path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if err := project.Save(path, op.Project); err != nil {
		return err
	}
	op.Path = path
	return nil
}

// GetProject returns the project for a tab.
func (a *App) GetProject(tabID string) *project.KapiProject {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if op := a.projects[tabID]; op != nil {
		return op.Project
	}
	return nil
}

// GetProjectPath returns the file path for a tab.
func (a *App) GetProjectPath(tabID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if op := a.projects[tabID]; op != nil {
		return op.Path
	}
	return ""
}

// getOpenProject is a helper to get an open project by tab ID (caller must not hold mu).
func (a *App) getOpenProject(tabID string) *openProject {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.projects[tabID]
}

// --- Flow operations ---

// FlowInfo is the frontend-facing flow summary.
type FlowInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	StepCount   int    `json:"step_count"`
}

// ListFlows returns all flows in a project tab.
func (a *App) ListFlows(tabID string) []FlowInfo {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil
	}

	var infos []FlowInfo
	for name, spec := range op.Project.Flows {
		infos = append(infos, FlowInfo{
			Name:      name,
			StepCount: len(spec.Steps),
		})
	}
	return infos
}

// GetFlow returns a flow's StepsSpec by name.
func (a *App) GetFlow(tabID, name string) *flow.StepsSpec {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil
	}
	return op.Project.GetFlow(name)
}

// SaveFlow saves or updates a flow in a project tab.
func (a *App) SaveFlow(tabID, name string, spec *flow.StepsSpec) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project.Flows == nil {
		op.Project.Flows = make(map[string]*flow.StepsSpec)
	}
	op.Project.Flows[name] = spec
	return nil
}

// DeleteFlow removes a flow from a project tab.
func (a *App) DeleteFlow(tabID, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	op := a.projects[tabID]
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	delete(op.Project.Flows, name)
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
