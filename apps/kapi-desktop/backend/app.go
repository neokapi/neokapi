// Package backend provides the Wails v3 service for Kapi.
// It exposes format/tool registries, plugin management, credential storage,
// .kapi project file operations, and flow execution to the React frontend.
package backend

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	// Blank-import the cli package so its init() registrations run in the desktop
	// too — command factories, MCP tools, and any cli-registered AI providers —
	// keeping the desktop's tool and provider lists in sync with the CLI.
	_ "github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/cli/pluginhost"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	fmtschema "github.com/neokapi/neokapi/core/format/schema"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/i18n"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	pluginmanifest "github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/wailsapp/wails/v3/pkg/application"
	"gopkg.in/yaml.v3"
)

// App is the Wails service that bridges Go backend to the React frontend.
type App struct {
	app       *application.App
	formatReg *registry.FormatRegistry
	toolReg   *registry.ToolRegistry
	schemaReg *fmtschema.SchemaRegistry
	pluginDir string

	// pluginRuntime owns the live plugin host, the Mode-C daemon pool that backs
	// plugin-provided formats (e.g. okapi-bridge's okf_*), and the discover→wire
	// sequence — the same pluginhost.Runtime the CLI uses, so the lifecycle logic
	// lives in one place. watchCancel stops the directory watcher that picks up
	// plugins installed or removed by another process (e.g. `kapi plugins
	// install` run in a terminal). The daemon pool is torn down in
	// ServiceShutdown.
	pluginRuntime *pluginhost.Runtime
	watchCancel   context.CancelFunc

	// i18n — localizes metadata on the way out of Wails methods so the
	// React frontend's tool palette and schema forms render in the
	// user's chosen locale without the frontend having to know anything
	// about the backend's string table. Built lazily on first SetLocale.
	i18nMu     sync.RWMutex
	translator i18n.Translator

	// Open projects (multiple tabs)
	mu       sync.RWMutex
	projects map[string]*openProject // keyed by tab ID

	// Flow runner
	runState *runner

	// TM and Termbase handles
	tmHandles *handleStore[*sievepen.SQLiteTM]
	tbHandles *handleStore[*termbase.SQLiteTermBase]

	// Persistence
	credentials *credentials.Store
	recent      *recentStore
	settings    *settingsStore

	// eventSink, when non-nil, receives every emitted event in addition to the
	// Wails app. The recording wbridge (cmd/wbridge) sets this to stream events
	// to the browser over SSE, since a plain browser has no Wails event channel.
	// Nil in production.
	eventMu   sync.RWMutex
	eventSink func(name string, data any)

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
	aitools.RegisterAll(toolReg)

	logger := log.New(os.Stderr, "[kapi-desktop] ", log.LstdFlags)
	pluginDir := defaultPluginDir()

	// Resolve the provider-config store under the env-overridable config dir
	// (not credentials.DefaultPath, which hardcodes os.UserConfigDir) so an
	// isolated KAPI_CONFIG_DIR fully isolates credentials from the user's own.
	credStore := credentials.NewStore(filepath.Join(kapiConfigDir(), "providers.json"))

	// Wire credential resolution: tools requiring "credentials" get their
	// provider/apiKey/model injected from the shared credential store.
	toolReg.SetConfigPreprocessor(func(toolName string, requires []string, config map[string]any) (map[string]any, error) {
		return credentials.ResolveCredentials(credStore, toolName, requires, config)
	})

	app := &App{
		formatReg:   formatReg,
		toolReg:     toolReg,
		schemaReg:   schemaReg,
		pluginDir:   pluginDir,
		projects:    make(map[string]*openProject),
		tmHandles:   newHandleStore[*sievepen.SQLiteTM](),
		tbHandles:   newHandleStore[*termbase.SQLiteTermBase](),
		credentials: credStore,
		recent:      newRecentStore(),
		settings:    newSettingsStore(),
		logger:      logger,
	}
	// Emit recent:changed whenever the recent-projects list mutates so the
	// native File → Recent Projects menu can rebuild itself (the menu is built
	// once at startup and otherwise never sees later opens — issue #3).
	app.recent.onChange = func() { app.emitEvent("recent:changed", nil) }

	// The plugin runtime owns discovery, the host, the daemon pool, and the
	// schema/format wiring — the same pluginhost.Runtime the CLI uses. Cache is
	// off so a rescan always reflects on-disk truth (live installs); connectors
	// are off because the desktop doesn't source through plugin connectors.
	app.pluginRuntime = pluginhost.NewRuntime(pluginhost.RuntimeOptions{
		Discover:  pluginhost.DiscoverOptions{EnvPluginsDir: pluginDir, OnWarn: func(msg string) { logger.Printf("plugin: %s", msg) }},
		FormatReg: formatReg,
		OnWarn:    func(msg string) { logger.Printf("plugin: %s", msg) },
		PoolLogger: func(format string, args ...any) {
			logger.Printf("[daemon] "+format, args...)
		},
		// Recompose the segmentation tool schema when a plugin contributes a
		// segmentation engine, so the new engine appears in the selector.
		OnSegmentersChanged: func() {
			libtools.RegisterSegmentation(toolReg)
		},
	})
	return app
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

	// blockStore is the project's .kapi/cache/blocks.db, opened once and reused
	// across calls. Opening it per call created a fresh connection pool (plus a
	// migration write) each time, so two concurrent operations on the same file
	// could trip "database is locked". One shared pool lets SQLite/WAL serialize
	// internally. Guarded by blockStoreMu; closed in CloseProject.
	blockStoreMu sync.Mutex
	blockStore   blockstore.Store
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

// ProjectHandles bundles the project-scoped TM and termbase handle IDs for a
// tab so the frontend can preselect both in a single call. Each id is the
// string handle the TM/termbase Wails methods (and handleStore.Get) accept;
// an empty id means the project has no auto-opened resource of that kind.
type ProjectHandles struct {
	TabID          string `json:"tabID"`
	TMHandle       string `json:"tmHandle"`
	TermbaseHandle string `json:"termbaseHandle"`
}

// GetProjectHandles returns the project-scoped TM and termbase handle IDs for a
// tab in one call. Convenience wrapper over GetProjectTMHandle /
// GetProjectTermbaseHandle for frontends that preselect both at once.
func (a *App) GetProjectHandles(tabID string) ProjectHandles {
	a.mu.RLock()
	defer a.mu.RUnlock()
	h := ProjectHandles{TabID: tabID}
	if op := a.projects[tabID]; op != nil {
		h.TMHandle = op.tmHandle
		h.TermbaseHandle = op.tbHandle
	}
	return h
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
	if a.watchCancel != nil {
		a.watchCancel()
	}
	a.pluginRuntime.Shutdown()
	return nil
}

// SetApplication stores the Wails app reference for dialogs and events.
func (a *App) SetApplication(app *application.App) {
	a.app = app
}

// host returns the live plugin host, or nil before plugins are first loaded.
// All host reads go through the runtime so a background rescan (the directory
// watcher) can swap the host in safely.
func (a *App) host() *pluginhost.Host {
	return a.pluginRuntime.Host()
}

// LoadPlugins discovers manifest-driven plugins and registers their schema
// extensions and formats on the app's registries, then starts watching the
// plugin directories so plugins installed or removed by another process (e.g.
// the kapi CLI) are picked up live. Runs in the foreground — callers needing
// async startup wrap with a goroutine.
func (a *App) LoadPlugins() {
	a.rescanPlugins()
	// Prime the metadata Translator from the persisted UI language so
	// tool/format listings come back localized on the very first
	// ListTools/ListFormats call — before the frontend has a chance to
	// invoke SetLocale.
	a.SetLocale(a.GetUILanguage())
	a.emitEvent("plugins-loaded", nil)
	a.startPluginWatch()
}

// rescanPlugins re-discovers manifest plugins and rebuilds the host via the
// shared runtime, which wires schema extensions and registers the formats
// plugins provide (e.g. okapi-bridge's okf_* filters) as daemon-backed
// readers/writers. A project using plugin formats therefore becomes readable as
// soon as the plugin is installed — no app restart needed (issue #4).
func (a *App) rescanPlugins() {
	a.pluginRuntime.Rescan()
}

// startPluginWatch begins watching the plugin directories. When the installed
// set changes out-of-band (a CLI install/remove, or another desktop window),
// the runtime rebuilds the host + formats and we notify the frontend so open
// projects re-check their plugin requirements and reload. In-app install/remove
// already rescans synchronously, so the watcher only fires for external change.
func (a *App) startPluginWatch() {
	if a.watchCancel != nil {
		return // already watching
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	go a.pluginRuntime.Watch(ctx, 3*time.Second, func(*pluginhost.Host) {
		a.logger.Println("plugin change detected on disk — rescanned")
		a.emitEvent("plugins-changed", nil)
		a.emitEvent("registries-changed", nil)
	})
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
			return nil, errors.New("project name or save path is required")
		}
		home, err := userHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		savePath = filepath.Join(home, "KapiProjects", name, "project.kapi")
	}

	// Expand ~ to the actual home directory (frontend may send ~/... paths).
	if strings.HasPrefix(savePath, "~/") || savePath == "~" {
		home, err := userHomeDir()
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
			SourceLanguage:  model.LocaleID(sourceLang),
			TargetLanguages: toLocaleIDs(targetLangs),
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
	op.blockStoreMu.Lock()
	if op.blockStore != nil {
		_ = op.blockStore.Close()
		op.blockStore = nil
	}
	op.blockStoreMu.Unlock()
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
		return errors.New("project has no file path (use SaveProjectAs)")
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
	Name        string          `json:"name"`
	Description string          `json:"description"`
	StepCount   int             `json:"step_count"`
	Valid       bool            `json:"valid"`
	Issues      []FlowIssueInfo `json:"issues,omitempty"`
}

// FlowIssueInfo is a validation issue for a flow step.
type FlowIssueInfo struct {
	Tool    string `json:"tool"`
	Type    string `json:"type"` // "unknown" or "undeclared_plugin"
	Message string `json:"message"`
}

// ListFlows returns all flows in a project tab with validation status.
func (a *App) ListFlows(tabID string) []FlowInfo {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil
	}

	// Validate all flows against the tool registry.
	pctx := project.NewProjectContext(op.Project, op.Path)
	allIssues := pctx.ValidateFlows(a.toolReg.ListWithSchemas())

	// Index issues by flow name.
	issuesByFlow := make(map[string][]FlowIssueInfo)
	for _, issue := range allIssues {
		issuesByFlow[issue.FlowName] = append(issuesByFlow[issue.FlowName], FlowIssueInfo{
			Tool:    issue.StepTool,
			Type:    issue.Type,
			Message: issue.Message,
		})
	}

	var infos []FlowInfo
	for name, spec := range op.Project.Flows {
		issues := issuesByFlow[name]
		infos = append(infos, FlowInfo{
			Name:      name,
			StepCount: len(spec.Steps),
			Valid:     len(issues) == 0,
			Issues:    issues,
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
	return op.Project.Flow(name)
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
	Tags        []string `json:"tags,omitempty"`
	Requires    []string `json:"requires,omitempty"`

	// IO contract fields (Framework AD-006): port Consumes/Produces (IOPort).
	Cardinality   string   `json:"cardinality,omitempty"`    // "monolingual", "bilingual", "multilingual"
	DefaultLocale string   `json:"default_locale,omitempty"` // e.g., "qps" for pseudo-translate
	Consumes      []IOPort `json:"consumes,omitempty"`       // ports read upstream
	Produces      []IOPort `json:"produces,omitempty"`       // ports written
	SideEffects   []string `json:"side_effects,omitempty"`   // external interactions

	// IsSourceTransform reports whether the tool is a transformer — it may
	// rewrite source (AD-006); the placement pass validates its position.
	IsSourceTransform bool `json:"is_source_transform,omitempty"`

	// Recoverable marks a transformer that vaults originals for later restore
	// (redaction); the placement pass holds it to the remote-egress rule.
	Recoverable bool `json:"recoverable,omitempty"`
}

// IOPort is one entry of a tool's IO contract surfaced to the flow
// editor: the port type, the side it pertains to, and whether // port is optional (graceful degradation) vs required.
type IOPort struct {
	Type     string `json:"type"`
	Side     string `json:"side,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	Layer    string `json:"layer,omitempty"`
}

// SetLocale configures the active locale for metadata Wails methods.
// Called from the React frontend whenever the user changes UI language.
// Empty or "en" disables localization; non-English locales load the
// matching MO catalog (embedded builtins + installed plugin catalogs).
// Returns the resolved locale so the frontend can confirm what took
// effect — useful when the request was "auto" and the backend chose.
func (a *App) SetLocale(locale string) string {
	tr := i18n.Resolve(i18n.ResolveOptions{Flag: locale})
	a.i18nMu.Lock()
	a.translator = tr
	a.i18nMu.Unlock()
	return string(tr.Locale())
}

// T returns the active Translator. Safe to call before SetLocale —
// returns a NoopTranslator that passes source text through unchanged.
// Not exposed to Wails (lowercase methods are private); backend code
// reaches for it directly.
func (a *App) T() i18n.Translator {
	a.i18nMu.RLock()
	defer a.i18nMu.RUnlock()
	if a.translator == nil {
		return i18n.NoopTranslator{}
	}
	return a.translator
}

// ListTools returns all registered tools (built-in + plugin).
func (a *App) ListTools() []ToolInfo {
	return a.toolInfosFrom(a.toolReg.ListWithSchemas())
}

// ListProjectTools returns tools available for a project, filtered by the
// project's declared plugins. Built-in tools are always included; plugin
// tools are only included if the project declares the plugin.
func (a *App) ListProjectTools(tabID string) []ToolInfo {
	op := a.getOpenProject(tabID)
	if op == nil {
		return a.ListTools()
	}
	ctx := project.NewProjectContext(op.Project, op.Path)
	filtered := ctx.AllowedTools(a.toolReg.ListWithSchemas())
	return a.toolInfosFrom(filtered)
}

// ValidateProjectFlows checks all flows in a project for tool references
// that require undeclared plugins. Returns nil if all tools are available.
func (a *App) ValidateProjectFlows(tabID string) []project.FlowValidationIssue {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil
	}
	ctx := project.NewProjectContext(op.Project, op.Path)
	return ctx.ValidateFlows(a.toolReg.ListWithSchemas())
}

func (a *App) toolInfosFrom(all []registry.ToolInfo) []ToolInfo {
	t := a.T()
	infos := make([]ToolInfo, len(all))
	for i, info := range all {
		toIOPort := func(fs []schema.IOPort) []IOPort {
			if len(fs) == 0 {
				return nil
			}
			out := make([]IOPort, len(fs))
			for i, f := range fs {
				out[i] = IOPort{Type: string(f.Type), Side: f.Side.String(), Optional: f.Optional, Layer: f.Layer}
			}
			return out
		}
		consumes := toIOPort(info.Consumes)
		produces := toIOPort(info.Produces)
		var sideEffects []string
		for _, s := range info.SideEffects {
			sideEffects = append(sideEffects, string(s))
		}
		name := string(info.Name)
		scope := "tools." + name
		infos[i] = ToolInfo{
			Name:              name,
			DisplayName:       t.T(i18n.Scope(scope+".displayName"), info.DisplayName),
			Description:       t.T(i18n.Scope(scope+".description"), info.Description),
			Category:          info.Category,
			Source:            info.Source,
			HasSchema:         info.HasSchema,
			Tags:              info.Tags,
			Requires:          info.Requires,
			Cardinality:       string(info.Cardinality),
			DefaultLocale:     string(info.DefaultLocale),
			Consumes:          consumes,
			Produces:          produces,
			SideEffects:       sideEffects,
			IsSourceTransform: info.IsSourceTransform,
			Recoverable:       info.Recoverable,
		}
	}
	slices.SortFunc(infos, func(a, b ToolInfo) int { return cmp.Compare(a.Name, b.Name) })
	return infos
}

// GetToolSchema returns the configuration schema for a tool.
// When the schema has pre-built RawJSON (e.g. from a plugin schema file),
// it is used directly so that all extension metadata (x-editor, x-enumLabels,
// x-step, $defs, etc.) passes through to the frontend unchanged.
//
// For tools that require credentials, a "credential" property is injected
// into the schema with a credential-picker widget, and the manual provider
// fields (provider, apiKey, model) are made conditionally visible.
func (a *App) GetToolSchema(name string) map[string]any {
	s := a.toolReg.Schema(registry.ToolID(name))
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

	// Inject credential picker for tools that require credentials.
	if s.ToolMeta != nil && slices.Contains(s.ToolMeta.Requires, "credentials") {
		a.injectCredentialPicker(result)
	}

	return result
}

// injectCredentialPicker adds a "credential" property with a credential-picker
// widget to the schema and makes the manual provider/apiKey/model fields
// conditionally visible only when no credential is selected.
func (a *App) injectCredentialPicker(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	// Build credential options from the store.
	options := []map[string]any{
		{"value": "", "label": "Custom (manual entry)"},
	}
	for _, c := range a.credentials.List() {
		label := c.Name
		if c.Model != "" {
			label += " (" + c.Model + ")"
		}
		options = append(options, map[string]any{
			"value": c.Name,
			"label": label,
		})
	}

	// Add the credential property.
	props["credential"] = map[string]any{
		"type":        "string",
		"title":       "Credential",
		"description": "Saved credential to use for this tool",
		"default":     "",
		"options":     options,
		"ui:widget":   "credential-picker",
		"ui:order":    float64(-1), // show first in group
	}

	// Make manual provider fields conditionally visible (only when credential is empty).
	manualCondition := map[string]any{
		"field": "credential",
		"eq":    "",
	}
	for _, fieldName := range []string{"provider", "apiKey", "model"} {
		if prop, ok := props[fieldName].(map[string]any); ok {
			prop["ui:visible"] = manualCondition
		}
	}

	// Add the credential field to whichever group holds the provider fields, so
	// it inherits that group's visibility. For a ToolGroup tool the provider
	// fields live in a member group (e.g. "ai:provider" for qa, "llm:provider"
	// for entity-extract) gated by the discriminator, so the credential picker
	// then appears only when that AI backend is selected — not in rules/ner mode.
	// Match by the group that contains "provider", which covers both the plain
	// "provider" group (translate) and the namespaced member groups.
	if groups, ok := schema["ui:groups"].([]any); ok {
		for _, g := range groups {
			group, ok := g.(map[string]any)
			if !ok {
				continue
			}
			fields, ok := group["fields"].([]any)
			if !ok {
				continue
			}
			if slices.Contains(fields, any("provider")) {
				// Prepend credential to the group fields.
				group["fields"] = append([]any{"credential"}, fields...)
				break
			}
		}
	}
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
		name := string(fi.Name)
		if !strings.Contains(name, "@") {
			bareNames[name] = true
		}
	}

	t := a.T()
	var infos []FormatInfo
	for _, fi := range allInfos {
		name := string(fi.Name)
		if idx := strings.LastIndex(name, "@"); idx > 0 {
			if bareNames[name[:idx]] {
				continue
			}
		}
		_, hasSchema := a.schemaReg.GetSchema(name)
		infos = append(infos, FormatInfo{
			Name:        name,
			DisplayName: t.T(i18n.Scope("formats."+name+".displayName"), fi.DisplayName),
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

// ListProjectFormats returns formats available for a project, filtered by
// the project's declared plugins. Built-in formats are always included.
func (a *App) ListProjectFormats(tabID string) []FormatInfo {
	op := a.getOpenProject(tabID)
	if op == nil {
		return a.ListFormats()
	}
	ctx := project.NewProjectContext(op.Project, op.Path)
	allowed := make(map[string]bool, len(ctx.AllowedSources))
	for _, s := range ctx.AllowedSources {
		allowed[s] = true
	}

	all := a.ListFormats()
	var filtered []FormatInfo
	for _, fi := range all {
		source := fi.Source
		if source == "" {
			source = registry.SourceBuiltIn
		}
		if allowed[source] {
			filtered = append(filtered, fi)
		}
	}
	return filtered
}

// DetectProjectFormat detects a format scoped to a project's declared plugins.
func (a *App) DetectProjectFormat(tabID, path string) string {
	op := a.getOpenProject(tabID)
	if op == nil {
		return a.DetectFormat(path)
	}
	ctx := project.NewProjectContext(op.Project, op.Path)
	return ctx.DetectFormat(a.formatReg, path)
}

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

// GetPluginDocs returns a summary of available plugin-contributed
// documentation. The manifest plugin model does not currently surface
// docs through the host, so this always returns nil — the frontend
// degrades to "no docs available" gracefully.
func (a *App) GetPluginDocs() map[string]any {
	return nil
}

// GetFilterDoc returns documentation for a single filter by ID. The
// manifest plugin model does not currently surface docs, so this
// always returns nil.
func (a *App) GetFilterDoc(filterID string) map[string]any {
	return nil
}

// GetStepDoc returns documentation for a single pipeline step. The
// manifest plugin model does not currently surface docs, so this
// always returns nil.
func (a *App) GetStepDoc(stepID string) map[string]any {
	return nil
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
	reg := preset.NewPresetRegistry()
	preset.RegisterBuiltins(reg)
	// Also extract presets from loaded schemas (bridge configurations).
	a.schemaReg.ExtractPresets(reg)
	var infos []FormatPresetInfo
	for _, p := range reg.ListFormatPresets(format) {
		source := p.Source
		if source == "" {
			source = registry.SourceBuiltIn
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
		return errors.New("format name and preset name are required")
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
		return errors.New("format name and preset name are required")
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
	return filepath.Join(kapiConfigDir(), "format-presets", formatName)
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
		return nil, errors.New("format name is required")
	}
	if filePath == "" {
		return nil, errors.New("file path is required")
	}

	// On-demand: if this format is read out-of-core by a plugin we don't have
	// yet (e.g. PDF via kapi-pdfium on a Linux/Windows desktop), fetch it from
	// the registry now so the reader below resolves. No-op once installed.
	a.ensureFormatPlugin(formatName)
	// For media formats the in-core reader exists, but OCR/ASR/demux need the
	// engine plugin (kapi-vision/asr/av) — install it on first open. No-op once
	// the engine is available or for non-media formats.
	a.ensureMediaEngine(formatName)

	reader, err := a.formatReg.NewReader(registry.FormatID(formatName))
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
		return nil, errors.New("no application context")
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
			info.Summary = "Layer: " + l.ResourceID()
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
	if n := countInlineCodeRuns(b.Source); n > 0 {
		props["inline_codes"] = strconv.Itoa(n)
	}
	for _, loc := range b.TargetLocales() {
		props["target_"+string(loc)] = "present"
	}
	return props
}

// countInlineCodeRuns counts non-text runs (Ph / PcOpen / PcClose / Sub /
// Plural / Select) in a Run sequence.
func countInlineCodeRuns(runs []model.Run) int {
	n := 0
	for _, r := range runs {
		if r.Text == nil {
			n++
		}
	}
	return n
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

// ListPlugins returns installed manifest-driven plugins with metadata.
// Plugins are sourced from the pluginhost (populated by LoadPlugins).
func (a *App) ListPlugins() []PluginInfo {
	if a.host() == nil {
		return nil
	}

	var infos []PluginInfo
	for _, p := range a.host().Plugins() {
		m := p.Manifest
		// Derive a unique installation ID from the install dir; this
		// matches the on-disk layout {name}/{version} so the frontend
		// can distinguish multiple versions of the same plugin.
		pluginID := p.Name()
		if p.Dir != "" {
			pluginID = filepath.Base(filepath.Dir(p.Dir)) + "/" + filepath.Base(p.Dir)
		}

		var formats []string
		var caps []PluginCapability
		for _, f := range m.Capabilities.Formats {
			formats = append(formats, f.Name)
			caps = append(caps, PluginCapability{
				Type:        "format",
				Name:        f.Name,
				DisplayName: f.DisplayName,
				Extensions:  f.Extensions,
			})
		}
		for _, c := range m.Capabilities.Commands {
			caps = append(caps, PluginCapability{
				Type: "command",
				Name: c.Name,
			})
		}
		for _, t := range m.Capabilities.MCPTools {
			caps = append(caps, PluginCapability{
				Type: "mcp_tool",
				Name: t.Name,
			})
		}

		infos = append(infos, PluginInfo{
			Name:         m.Plugin,
			ID:           pluginID,
			Version:      m.Version,
			Type:         pluginTypeFromManifest(m),
			Description:  m.Description,
			Formats:      formats,
			Capabilities: caps,
		})
	}
	return infos
}

// pluginTypeFromManifest classifies a manifest plugin by its dominant
// capability so the desktop UI can group entries (matches the legacy
// "format" / "tool" / "bundle" labels for visual continuity).
func pluginTypeFromManifest(m *pluginmanifest.Manifest) string {
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

// --- Utility ---

// VersionInfo describes the application version, mirroring the CLI's
// `kapi version` output (semantic version + git commit + build date).
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

// GetHomeDir returns the user's home directory path.
func (a *App) GetHomeDir() string {
	home, _ := userHomeDir()
	return home
}

func (a *App) emitEvent(name string, data any) {
	if a.app != nil {
		a.app.Event.Emit(name, data)
	}
	a.eventMu.RLock()
	sink := a.eventSink
	a.eventMu.RUnlock()
	if sink != nil {
		sink(name, data)
	}
}

// SetEventSink registers a listener that receives every emitted event, in
// addition to the Wails app. Used by the recording wbridge to stream events
// (plugin install progress, flow:event, …) to the browser over SSE. Passing nil
// clears the sink. The sink is invoked from arbitrary goroutines, so it must be
// safe for concurrent use.
func (a *App) SetEventSink(sink func(name string, data any)) {
	a.eventMu.Lock()
	a.eventSink = sink
	a.eventMu.Unlock()
}

func defaultPluginDir() string {
	dir := os.Getenv("KAPI_PLUGIN_DIR")
	if dir != "" {
		return dir
	}
	return filepath.Join(kapiConfigDir(), "plugins")
}

func toLocaleIDs(ss []string) []model.LocaleID {
	ids := make([]model.LocaleID, len(ss))
	for i, s := range ss {
		ids[i] = model.LocaleID(s)
	}
	return ids
}
