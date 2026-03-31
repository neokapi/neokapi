// Package backend provides the Wails v3 service for Kapi.
// It exposes format/tool registries, plugin management, credential storage,
// .kapi project file operations, and flow execution to the React frontend.
package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	plugincache "github.com/neokapi/neokapi/core/plugin/cache"
	"github.com/neokapi/neokapi/core/id"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	fmtschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/plugin/loader"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
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

	// TM and Termbase handles
	tmHandles *handleStore[*sievepen.SQLiteTM]
	tbHandles *handleStore[*termbase.SQLiteTermBase]

	// Persistence
	credentials *CredentialStore
	recent      *recentStore
	settings    *settingsStore

	logger *log.Logger
}

// NewApp creates a new Kapi backend service.
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
		tmHandles:    newHandleStore[*sievepen.SQLiteTM](),
		tbHandles:    newHandleStore[*termbase.SQLiteTermBase](),
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

// ServiceStartup is called by Wails v3 during application startup.
// All initialization happens here — data is guaranteed ready before
// the frontend renders.
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.logger.Println("service starting")
	return nil
}

// ServiceShutdown is called by Wails v3 during application shutdown.
func (a *App) ServiceShutdown() error {
	a.logger.Println("service shutting down")
	a.tmHandles.CloseAll()
	a.tbHandles.CloseAll()
	return nil
}

// SetApplication stores the Wails app reference for dialogs and events.
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// LoadPlugins scans and loads plugins in the background.
func (a *App) LoadPlugins() {
	a.rescanPlugins()
	a.emitEvent("plugins-loaded", nil)

	// Watch the plugin cache file for external changes (e.g., CLI install/remove).
	go a.watchPluginCache()
}

// rescanPlugins re-reads plugin metadata and registers plugin-provided
// formats into the app's format registry. Pass formatReg so ScanMetadata
// can register format readers/writers from the cache.
func (a *App) rescanPlugins() {
	if err := a.pluginLoader.ScanMetadata(a.formatReg); err != nil {
		a.logger.Printf("plugin scan: %v", err)
	}
}

// watchPluginCache polls the plugin-cache.json file for changes and re-scans
// when it detects an external modification (e.g., from the kapi CLI).
func (a *App) watchPluginCache() {
	cachePath := filepath.Join(a.pluginLoader.Dir(), "plugin-cache.json")
	var lastMod time.Time

	if info, err := os.Stat(cachePath); err == nil {
		lastMod = info.ModTime()
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		info, err := os.Stat(cachePath)
		if err != nil {
			continue
		}
		if info.ModTime().After(lastMod) {
			lastMod = info.ModTime()
			a.logger.Println("plugin cache changed externally, re-scanning")
			a.rescanPlugins()
			a.emitEvent("plugins-changed", nil)
			a.emitEvent("registries-changed", nil)
		}
	}
}

// --- Project operations (multi-tab) ---

// TabInfo is returned to the frontend to describe an open tab.
type TabInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// NewProject creates a new project, saves it to disk, and opens it as a tab.
// If savePath is empty, defaults to ~/KapiProjects/{name}/project.kapi.
func (a *App) NewProject(name, sourceLang string, targetLangs []string, savePath string) (*TabInfo, error) {
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}

	// Default save location: ~/KapiProjects/{name}/project.kapi
	if savePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		savePath = filepath.Join(home, "KapiProjects", name, "project.kapi")
	}

	// Expand ~ to the actual home directory (frontend may send ~/... paths).
	if strings.HasPrefix(savePath, "~/") || savePath == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			savePath = filepath.Join(home, savePath[2:])
		}
	}

	// Ensure .kapi extension.
	if !strings.HasSuffix(strings.ToLower(savePath), ".kapi") {
		savePath += ".kapi"
	}

	// Create parent directory.
	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return nil, fmt.Errorf("create project directory: %w", err)
	}

	proj := &project.KapiProject{
		Version:         project.CurrentVersion,
		Name:            name,
		SourceLanguage:  sourceLang,
		TargetLanguages: targetLangs,
		Flows:           make(map[string]*flow.StepsSpec),
	}

	if err := project.Save(savePath, proj); err != nil {
		return nil, fmt.Errorf("save project: %w", err)
	}

	tabID := id.New()
	a.mu.Lock()
	a.projects[tabID] = &openProject{ID: tabID, Path: savePath, Project: proj}
	a.mu.Unlock()

	a.recent.add(savePath, name)
	return &TabInfo{ID: tabID, Name: name, Path: savePath}, nil
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
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	HasSchema   bool     `json:"has_schema"`
	Inputs      []string `json:"inputs,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"requires,omitempty"`
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
			Inputs:      info.Inputs,
			Outputs:     info.Outputs,
			Tags:        info.Tags,
			Requires:    info.Requires,
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
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	MimeTypes   []string `json:"mime_types,omitempty"`
	HasReader   bool     `json:"has_reader"`
	HasWriter   bool     `json:"has_writer"`
	Source      string   `json:"source,omitempty"`
	HasSchema   bool     `json:"has_schema"`
}

// ListFormats returns all registered formats with full metadata.
func (a *App) ListFormats() []FormatInfo {
	var infos []FormatInfo
	for _, fi := range a.formatReg.FormatInfos() {
		_, hasSchema := a.schemaReg.GetSchema(fi.Name)
		infos = append(infos, FormatInfo{
			Name:        fi.Name,
			DisplayName: fi.DisplayName,
			Extensions:  fi.Extensions,
			MimeTypes:   fi.MimeTypes,
			HasReader:   fi.HasReader,
			HasWriter:   fi.HasWriter,
			Source:      fi.Source,
			HasSchema:   hasSchema,
		})
	}
	return infos
}

// GetFormatSchema returns the configuration schema for a format.
func (a *App) GetFormatSchema(formatName string) map[string]any {
	s, ok := a.schemaReg.GetSchema(formatName)
	if !ok {
		return nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// --- Preset operations ---

// PresetInfo is the frontend-facing framework preset summary.
type PresetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FormatPresetInfo is the frontend-facing format preset summary.
type FormatPresetInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Format      string         `json:"format"`
	Config      map[string]any `json:"config,omitempty"`
}

// ListPresets returns all available framework presets.
func (a *App) ListPresets() []PresetInfo {
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	var infos []PresetInfo
	for _, p := range reg.ListFrameworkPresets() {
		infos = append(infos, PresetInfo{
			Name:        p.Name,
			Description: p.Description,
		})
	}
	return infos
}

// GetPresetDetails returns the full details of a framework preset.
func (a *App) GetPresetDetails(name string) map[string]any {
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	for _, p := range reg.ListFrameworkPresets() {
		if p.Name == name {
			return map[string]any{
				"name":           p.Name,
				"description":    p.Description,
				"mappings":       p.Mappings,
				"exclude":        p.Exclude,
				"format_presets": p.FormatPresets,
				"flows":          p.Flows,
			}
		}
	}
	return nil
}

// ApplyPreset applies a framework preset to a project tab,
// setting content mappings, exclude patterns, and format presets.
func (a *App) ApplyPreset(tabID, presetName string) (*project.KapiProject, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	op := a.projects[tabID]
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}

	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)

	var fp *preset.FrameworkPreset
	for _, p := range reg.ListFrameworkPresets() {
		if p.Name == presetName {
			fp = p
			break
		}
	}
	if fp == nil {
		return nil, fmt.Errorf("preset %q not found", presetName)
	}

	// Apply mappings as content entries.
	var entries []project.ContentEntry
	for _, m := range fp.Mappings {
		entries = append(entries, project.ContentEntry{
			Path:   m.Local,
			Format: m.Format,
			Target: m.TargetPath,
		})
	}
	op.Project.Content = entries
	op.Project.Preset = presetName

	return op.Project, nil
}

// ListFormatPresets returns format presets for a specific format.
func (a *App) ListFormatPresets(format string) []FormatPresetInfo {
	reg := a.pluginLoader.Presets()
	preset.RegisterBuiltins(reg)
	var infos []FormatPresetInfo
	for _, p := range reg.ListFormatPresets(format) {
		infos = append(infos, FormatPresetInfo{
			Name:        p.Name,
			Description: p.Description,
			Format:      p.Format,
			Config:      p.Config,
		})
	}
	return infos
}

// --- Plugin operations ---

// PluginCapability describes a format or tool provided by a plugin.
type PluginCapability struct {
	Type        string   `json:"type"`                   // "format" or "tool"
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
}

// PluginInfo is the frontend-facing plugin summary.
type PluginInfo struct {
	Name             string             `json:"name"`
	ID               string             `json:"id"`                         // unique identifier for this specific plugin installation
	Version          string             `json:"version"`
	FrameworkVersion string             `json:"framework_version,omitempty"`
	Type             string             `json:"type"`
	Description      string             `json:"description,omitempty"`
	Formats          []string           `json:"formats,omitempty"`
	Capabilities     []PluginCapability `json:"capabilities,omitempty"`
}

// ListPlugins returns installed plugins with full metadata.
func (a *App) ListPlugins() []PluginInfo {
	// Read the cache to get rich metadata (capabilities, descriptions).
	// Key by name+frameworkVersion since multiple versions can coexist (e.g., okapi 1.44.0 + 1.48.0).
	cache, _ := plugincache.Read(a.pluginLoader.Dir())
	cacheMap := make(map[string]*plugincache.CachedPlugin)
	if cache != nil {
		for i := range cache.Plugins {
			cp := &cache.Plugins[i]
			key := cp.Name + ":" + cp.FrameworkVersion
			cacheMap[key] = cp
			// Also store by name-only as fallback.
			if _, exists := cacheMap[cp.Name]; !exists {
				cacheMap[cp.Name] = cp
			}
		}
	}

	var infos []PluginInfo
	for _, p := range a.pluginLoader.Plugins() {
		// Build a unique ID from the directory name on disk.
		// Source path is like ".../plugins/okapi-1.44.0/2.20.0" — parent base is the plugin ref name.
		pluginID := p.Name
		if p.Source != "" {
			parent := filepath.Dir(p.Source)
			pluginID = filepath.Base(parent)
		}

		info := PluginInfo{
			Name:             p.Name,
			ID:               pluginID,
			Version:          p.Version,
			FrameworkVersion: p.FrameworkVersion,
			Type:             p.Type,
			Formats:          p.Formats,
		}

		// Enrich from cache — try exact match (name+fw), fall back to name-only.
		key := p.Name + ":" + p.FrameworkVersion
		cp := cacheMap[key]
		if cp == nil {
			cp = cacheMap[p.Name]
		}
		if cp != nil {
			if cp.Manifest != nil {
				info.Description = cp.Manifest.Description
				for _, cap := range cp.Manifest.Capabilities {
					info.Capabilities = append(info.Capabilities, PluginCapability{
						Type:        cap.Type,
						Name:        cap.Name,
						DisplayName: cap.DisplayName,
						Extensions:  cap.Extensions,
					})
				}
			}
			if cp.FrameworkVersion != "" {
				info.FrameworkVersion = cp.FrameworkVersion
			}
		}

		infos = append(infos, info)
	}
	return infos
}

// --- Utility ---

// GetVersion returns the kapi version string.
func (a *App) GetVersion() string {
	return version.Version
}

// GetHomeDir returns the user's home directory path.
func (a *App) GetHomeDir() string {
	home, _ := os.UserHomeDir()
	return home
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
