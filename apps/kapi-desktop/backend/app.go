// Package backend provides the Wails v3 service for Kapi.
// It exposes format/tool registries, plugin management, credential storage,
// .kapi project file operations, and flow execution to the React frontend.
package backend

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	fmtschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	plugincache "github.com/neokapi/neokapi/core/plugin/cache"
	"github.com/neokapi/neokapi/core/plugin/loader"
	pluginreg "github.com/neokapi/neokapi/core/plugin/registry"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/wailsapp/wails/v3/pkg/application"
	"gopkg.in/yaml.v3"
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
	pluginLoader.SetToolRegistry(toolReg)

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
	watcher *fileWatcher

	// Project-scoped TM and termbase (auto-opened from .kapi/tm.db and .kapi/termbase.db).
	tmHandle string // handle ID in App.tmHandles, empty if none
	tbHandle string // handle ID in App.tbHandles, empty if none
}

// GetProjectTMHandle returns the auto-opened TM handle for a project tab,
// or empty string if the project has no .kapi/tm.db.
func (a *App) GetProjectTMHandle(tabID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if op := a.projects[tabID]; op != nil {
		return op.tmHandle
	}
	return ""
}

// GetProjectTermbaseHandle returns the auto-opened termbase handle for a project tab,
// or empty string if the project has no .kapi/termbase.db.
func (a *App) GetProjectTermbaseHandle(tabID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if op := a.projects[tabID]; op != nil {
		return op.tbHandle
	}
	return ""
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
// formats and schemas into the app's registries.
func (a *App) rescanPlugins() {
	if err := a.pluginLoader.ScanMetadata(a.formatReg); err != nil {
		a.logger.Printf("plugin scan: %v", err)
		return
	}
	// Transfer plugin schemas to the app's schema registry so
	// GetFormatSchema can find them (e.g., okf_html, okf_xliff).
	for id, s := range a.pluginLoader.Schemas().AllSchemas() {
		a.schemaReg.RegisterSchema(id, s)
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
// The name is not stored in the YAML — the display name is derived from the
// folder name. Users can override it later on the project detail page.
func (a *App) NewProject(name, sourceLang string, targetLangs []string, savePath string) (*TabInfo, error) {
	// Default save location: ~/KapiProjects/{name}/project.kapi
	if savePath == "" {
		if name == "" {
			return nil, fmt.Errorf("project name or save path is required")
		}
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
		Version: project.CurrentVersion,
		Defaults: project.Defaults{
			SourceLanguage:  sourceLang,
			TargetLanguages: targetLangs,
		},
		Flows: make(map[string]*flow.StepsSpec),
	}

	// Auto-populate plugins from installed plugins so the project
	// is immediately compatible with the user's environment.
	project.PopulatePlugins(proj, a.installedPluginList())

	if err := project.Save(savePath, proj); err != nil {
		return nil, fmt.Errorf("save project: %w", err)
	}

	displayName := projectDisplayName(proj, savePath)

	tabID := id.New()
	op := &openProject{ID: tabID, Path: savePath, Project: proj}
	a.mu.Lock()
	a.projects[tabID] = op
	a.mu.Unlock()

	a.startWatcher(op)
	a.recent.add(savePath, displayName)
	return &TabInfo{ID: tabID, Name: displayName, Path: savePath}, nil
}

// startWatcher begins file system polling for a project tab.
func (a *App) startWatcher(op *openProject) {
	if op.Path == "" {
		return
	}
	dir := filepath.Dir(op.Path)
	fw := newFileWatcher(a, op.ID, dir)
	op.watcher = fw
	fw.Start()
}

// projectDisplayName returns the project name for display purposes.
// If the project has an explicit name set in the YAML, use it.
// Otherwise derive it from the parent directory of the .kapi file.
func projectDisplayName(proj *project.KapiProject, path string) string {
	if proj.Name != "" {
		return proj.Name
	}
	return filepath.Base(filepath.Dir(path))
}

// OpenProject loads a .kapi file from disk and returns its tab ID.
// If the file is already open in another tab, returns that tab's ID.
func (a *App) OpenProject(path string) (*TabInfo, error) {
	// Check if already open.
	a.mu.RLock()
	for _, op := range a.projects {
		if op.Path == path {
			a.mu.RUnlock()
			return &TabInfo{ID: op.ID, Name: projectDisplayName(op.Project, op.Path), Path: op.Path}, nil
		}
	}
	a.mu.RUnlock()

	proj, err := project.Load(path)
	if err != nil {
		return nil, err
	}
	tabID := id.New()
	op := &openProject{ID: tabID, Path: path, Project: proj}

	// Auto-open project-scoped TM and termbase if present.
	a.autoOpenProjectResources(op)

	a.mu.Lock()
	a.projects[tabID] = op
	a.mu.Unlock()

	a.startWatcher(op)
	displayName := projectDisplayName(proj, path)
	a.recent.add(path, displayName)
	return &TabInfo{ID: tabID, Name: displayName, Path: path}, nil
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

// CloseProject removes a project tab, stops its file watcher,
// and closes any auto-opened TM/termbase handles.
func (a *App) CloseProject(tabID string) {
	a.mu.Lock()
	op := a.projects[tabID]
	delete(a.projects, tabID)
	a.mu.Unlock()
	if op == nil {
		return
	}
	if op.watcher != nil {
		op.watcher.Stop()
	}
	if op.tmHandle != "" {
		a.tmHandles.Close(op.tmHandle)
	}
	if op.tbHandle != "" {
		a.tbHandles.Close(op.tbHandle)
	}
}

// autoOpenProjectResources checks for convention-based .kapi/tm.db and
// .kapi/termbase.db files relative to the project root and opens them as
// project-scoped TM/termbase handles.
func (a *App) autoOpenProjectResources(op *openProject) {
	if op.Path == "" {
		return
	}
	basePath := filepath.Dir(op.Path)

	tmPath := filepath.Join(basePath, ".kapi", "tm.db")
	if _, err := os.Stat(tmPath); err == nil {
		if tm, err := sievepen.NewSQLiteTM(tmPath); err == nil {
			op.tmHandle = a.tmHandles.Open(tm)
		}
	}

	tbPath := filepath.Join(basePath, ".kapi", "termbase.db")
	if _, err := os.Stat(tbPath); err == nil {
		if tb, err := termbase.NewSQLiteTermBase(tbPath); err == nil {
			op.tbHandle = a.tbHandles.Open(tb)
		}
	}
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
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Source      string   `json:"source,omitempty"` // "built-in" or plugin name
	HasSchema   bool     `json:"has_schema"`
	Inputs      []string `json:"inputs,omitempty"`
	Outputs     []string `json:"outputs,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"requires,omitempty"`
}

// ListTools returns all registered tools (built-in + plugin).
func (a *App) ListTools() []ToolInfo {
	all := a.toolReg.ListWithSchemas()
	infos := make([]ToolInfo, len(all))
	for i, info := range all {
		infos[i] = ToolInfo{
			Name:        info.Name,
			DisplayName: info.DisplayName,
			Description: info.Description,
			Category:    info.Category,
			Source:      info.Source,
			HasSchema:   info.HasSchema,
			Inputs:      info.Inputs,
			Outputs:     info.Outputs,
			Tags:        info.Tags,
			Requires:    info.Requires,
		}
	}
	slices.SortFunc(infos, func(a, b ToolInfo) int { return cmp.Compare(a.Name, b.Name) })
	return infos
}

// GetToolSchema returns the configuration schema for a tool.
// When the schema has pre-built RawJSON (e.g. from a plugin schema file),
// it is used directly so that all extension metadata (x-editor, x-enumLabels,
// x-step, $defs, etc.) passes through to the frontend unchanged.
func (a *App) GetToolSchema(name string) map[string]any {
	s := a.toolReg.GetSchema(name)
	if s == nil {
		return nil
	}
	// Prefer raw JSON to preserve all extension fields.
	data := s.RawJSON
	if len(data) == 0 {
		var err error
		data, err = json.Marshal(s)
		if err != nil {
			return nil
		}
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
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
	// Deduplicate versioned entries: if both "okf_html" and "okf_html@1.48.0"
	// exist, keep only the bare name. Same logic as the CLI's formats list.
	allInfos := a.formatReg.FormatInfos()
	bareNames := make(map[string]bool, len(allInfos))
	for _, fi := range allInfos {
		if !strings.Contains(fi.Name, "@") {
			bareNames[fi.Name] = true
		}
	}

	var infos []FormatInfo
	for _, fi := range allInfos {
		if idx := strings.LastIndex(fi.Name, "@"); idx > 0 {
			if bareNames[fi.Name[:idx]] {
				continue
			}
		}
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
// GetFormatSchema returns the configuration schema for a format filter.
// When the schema has pre-built RawJSON (e.g. loaded from a plugin schema file),
// it is used directly so that all extension metadata (x-editor, x-enumLabels,
// x-format, $defs, etc.) passes through to the frontend unchanged.
func (a *App) GetFormatSchema(formatName string) map[string]any {
	s, ok := a.schemaReg.GetSchema(formatName)
	if !ok {
		return nil
	}
	// Prefer raw JSON to preserve all extension fields.
	data := s.RawJSON
	if len(data) == 0 {
		var err error
		data, err = json.Marshal(s)
		if err != nil {
			return nil
		}
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// GetPluginDocs returns a summary of available documentation.
// Returns filter/step ID lists and metadata — individual docs are fetched
// via GetFilterDoc/GetStepDoc to avoid loading the full corpus.
func (a *App) GetPluginDocs() map[string]any {
	filterIDs := a.pluginLoader.ListFilterDocs()
	stepIDs := a.pluginLoader.ListStepDocs()
	if len(filterIDs) == 0 && len(stepIDs) == 0 {
		return nil
	}

	result := map[string]any{
		"filterIDs": filterIDs,
		"stepIDs":   stepIDs,
	}

	// Include metadata (aliases, wiki URL).
	meta := a.pluginLoader.DocsMetadata()
	if meta != nil {
		var m map[string]any
		if err := json.Unmarshal(meta, &m); err == nil {
			for k, v := range m {
				result[k] = v
			}
		}
	}

	return result
}

// GetFilterDoc returns documentation for a single filter by ID (e.g. "okf_json").
// The loader handles alias resolution. Returns nil if not found.
func (a *App) GetFilterDoc(filterID string) map[string]any {
	raw := a.pluginLoader.FilterDoc(filterID)
	if raw == nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	return result
}

// GetStepDoc returns documentation for a single pipeline step by ID
// (e.g. "batch-translation"). Returns nil if not found.
func (a *App) GetStepDoc(stepID string) map[string]any {
	raw := a.pluginLoader.StepDoc(stepID)
	if raw == nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
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
	Source      string         `json:"source,omitempty"` // "built-in", "plugin", "user", "bridge", "schema"
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

	// Apply mappings as content entries (bare entries).
	var entries []project.ContentCollection
	for _, m := range fp.Mappings {
		var fmtSpec *project.FormatSpec
		if m.Format != "" {
			fmtSpec = &project.FormatSpec{Name: m.Format}
		}
		entries = append(entries, project.ContentCollection{
			Path:   m.Local,
			Format: fmtSpec,
			Target: m.TargetPath,
		})
	}
	op.Project.Content = entries
	op.Project.Preset = presetName

	return op.Project, nil
}

// DetectPreset scans the project directory for telltale files defined by
// each framework preset's Detect field and returns the matching preset name,
// or empty string if none match.
func (a *App) DetectPreset(tabID string) string {
	a.mu.RLock()
	op := a.projects[tabID]
	a.mu.RUnlock()
	if op == nil {
		return ""
	}
	basePath := filepath.Dir(op.Path)
	if basePath == "" {
		return ""
	}

	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	return reg.DetectFrameworkPreset(basePath)
}

// ListFormatPresets returns format presets for a specific format.
func (a *App) ListFormatPresets(format string) []FormatPresetInfo {
	reg := a.pluginLoader.Presets()
	preset.RegisterBuiltins(reg)
	// Also extract presets from loaded schemas (bridge configurations).
	a.schemaReg.ExtractPresets(reg)
	var infos []FormatPresetInfo
	for _, p := range reg.ListFormatPresets(format) {
		source := p.Source
		if source == "" {
			source = "built-in"
		}
		infos = append(infos, FormatPresetInfo{
			Name:        p.Name,
			Description: p.Description,
			Format:      p.Format,
			Config:      p.Config,
			Source:      source,
		})
	}

	// Merge user presets from ~/.config/kapi/format-presets/{format}/*.json
	userPresets, _ := a.loadUserPresets(format)
	infos = append(infos, userPresets...)

	return infos
}

// SaveFormatPreset saves a user format preset to ~/.config/kapi/format-presets/{format}/{name}.json.
func (a *App) SaveFormatPreset(formatName, presetName string, config map[string]any) error {
	if formatName == "" || presetName == "" {
		return fmt.Errorf("format name and preset name are required")
	}
	dir := a.userPresetDir(formatName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create preset directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal preset config: %w", err)
	}
	path := filepath.Join(dir, presetName+".json")
	return os.WriteFile(path, data, 0o644)
}

// DeleteFormatPreset deletes a user format preset.
func (a *App) DeleteFormatPreset(formatName, presetName string) error {
	if formatName == "" || presetName == "" {
		return fmt.Errorf("format name and preset name are required")
	}
	path := filepath.Join(a.userPresetDir(formatName), presetName+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete preset: %w", err)
	}
	return nil
}

// ListAllFormatPresets returns both built-in, plugin, and user presets for a format.
func (a *App) ListAllFormatPresets(formatName string) []FormatPresetInfo {
	return a.ListFormatPresets(formatName)
}

// userPresetDir returns the directory for user format presets.
func (a *App) userPresetDir(formatName string) string {
	home, err := os.UserConfigDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, "kapi", "format-presets", formatName)
}

// loadUserPresets reads user presets from disk for a given format.
func (a *App) loadUserPresets(formatName string) ([]FormatPresetInfo, error) {
	dir := a.userPresetDir(formatName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var presets []FormatPresetInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		presets = append(presets, FormatPresetInfo{
			Name:   name,
			Format: formatName,
			Config: config,
			Source: "user",
		})
	}
	slices.SortFunc(presets, func(a, b FormatPresetInfo) int { return cmp.Compare(a.Name, b.Name) })
	return presets, nil
}

// --- Phase 3: Config rendering ---

// RenderFormatConfig converts form values to YAML or JSON format.
func (a *App) RenderFormatConfig(formatName string, config map[string]any, outputFormat string) (string, error) {
	if outputFormat == "" {
		outputFormat = "yaml"
	}

	// For bridge formats with a section map, reconstruct hierarchical config.
	rendered := config
	s, hasSchema := a.schemaReg.GetSchema(formatName)
	if hasSchema && len(s.SectionMap) > 0 {
		hierarchical := make(map[string]any)
		for key, val := range config {
			section, ok := s.SectionMap[key]
			if ok {
				sec, exists := hierarchical[section]
				if !exists {
					sec = make(map[string]any)
					hierarchical[section] = sec
				}
				sec.(map[string]any)[key] = val
			} else {
				hierarchical[key] = val
			}
		}
		rendered = hierarchical
	}

	switch strings.ToLower(outputFormat) {
	case "json":
		data, err := json.MarshalIndent(rendered, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal JSON: %w", err)
		}
		return string(data), nil
	case "yaml", "yml":
		data, err := yaml.Marshal(rendered)
		if err != nil {
			return "", fmt.Errorf("marshal YAML: %w", err)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// --- Phase 4: Ad-hoc format runner ---

// FormatPartInfo describes a single part from a format reader.
type FormatPartInfo struct {
	Type       string            `json:"type"`
	ID         string            `json:"id"`
	Summary    string            `json:"summary"`
	SourceText string            `json:"source_text,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// RunFormatReader runs a format reader on a file and returns the parts.
func (a *App) RunFormatReader(formatName string, filePath string, config map[string]any) ([]FormatPartInfo, error) {
	if formatName == "" {
		return nil, fmt.Errorf("format name is required")
	}
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	reader, err := a.formatReg.NewReader(formatName)
	if err != nil {
		return nil, fmt.Errorf("create reader: %w", err)
	}
	defer reader.Close()

	// Apply configuration if provided.
	if len(config) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(config); err != nil {
				a.logger.Printf("warn: apply config to reader: %v", err)
			}
		}
	}

	// Open the file.
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	doc := &model.RawDocument{
		URI:      filePath,
		FormatID: formatName,
		Reader:   f,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open document: %w", err)
	}

	var parts []FormatPartInfo
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return parts, fmt.Errorf("read error: %w", pr.Error)
		}
		parts = append(parts, partToInfo(pr.Part))
	}

	return parts, nil
}

// RunFormatReaderDialog shows a file dialog then runs the reader.
func (a *App) RunFormatReaderDialog(formatName string, config map[string]any) ([]FormatPartInfo, error) {
	if a.app == nil {
		return nil, fmt.Errorf("no application context")
	}

	path, err := a.app.Dialog.OpenFile().
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // user canceled
	}

	return a.RunFormatReader(formatName, path, config)
}

// partToInfo converts a model.Part to a FormatPartInfo for the frontend.
func partToInfo(p *model.Part) FormatPartInfo {
	info := FormatPartInfo{
		Type: p.Type.String(),
	}

	if p.Resource != nil {
		info.ID = p.Resource.ResourceID()
	}

	switch p.Type {
	case model.PartBlock:
		if b, ok := p.Resource.(*model.Block); ok {
			info.Summary = blockSummary(b)
			info.SourceText = blockSourceText(b)
			info.Properties = blockProperties(b)
		}
	case model.PartLayerStart:
		if l, ok := p.Resource.(*model.Layer); ok {
			info.Summary = fmt.Sprintf("Layer: %s", l.ResourceID())
			info.Properties = map[string]string{
				"name": l.Name,
			}
		}
	case model.PartData:
		info.Summary = "Structural data"
	}

	return info
}

// blockSummary returns a short summary of a Block's source content.
func blockSummary(b *model.Block) string {
	text := b.SourceText()
	if text == "" {
		return "(empty)"
	}
	if len(text) > 80 {
		return text[:80] + "..."
	}
	return text
}

// blockSourceText returns the full source text of a Block.
func blockSourceText(b *model.Block) string {
	return b.SourceText()
}

// blockProperties returns notable properties of a Block.
func blockProperties(b *model.Block) map[string]string {
	props := make(map[string]string)
	for _, seg := range b.Source {
		if seg.Content != nil && seg.Content.HasSpans() {
			count := len(seg.Content.Spans)
			props["inline_codes"] = fmt.Sprintf("%d", count)
			break
		}
	}
	if len(b.Targets) > 0 {
		for loc := range b.Targets {
			props["target_"+string(loc)] = "present"
		}
	}
	return props
}

// --- Plugin operations ---

// PluginCapability describes a format or tool provided by a plugin.
type PluginCapability struct {
	Type        string   `json:"type"` // "format" or "tool"
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
}

// PluginInfo is the frontend-facing plugin summary.
type PluginInfo struct {
	Name             string             `json:"name"`
	ID               string             `json:"id"` // unique identifier for this specific plugin installation
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
