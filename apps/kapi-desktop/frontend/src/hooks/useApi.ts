/**
 * API hooks that bridge the React frontend to the Wails Go backend.
 *
 * In Wails dev/production mode, imports the auto-generated bindings directly.
 * In Storybook/vitest (no Wails runtime), methods return null gracefully.
 */

import type {
  KapiProject,
  TabInfo,
  FlowInfo,
  FlowSpec,
  ToolInfo,
  FormatInfo,
  PluginInfo,
  PluginStatus,
  ProviderConfig,
  DefaultModelInfo,
  AIModelOption,
  ProjectFilter,
  ProjectFilters,
  PluginDocsSummary,
  FilterDoc,
  StepDoc,
  BrowsePathRequest,
  CheckRunResult,
  SessionState,
  ProjectStatus,
  ExtractResult,
  AdoptFlowResult,
  ProjectHandles,
} from "../types/api";

type Backend = Record<string, (...args: unknown[]) => Promise<unknown>>;

let backendModule: Backend | null = null;
let backendLoaded = false;

/**
 * Lazily load the Wails-generated backend bindings.
 * Returns null when bindings aren't available (Storybook, vitest).
 */
async function getBackend(): Promise<Backend | null> {
  if (backendModule) return backendModule;
  if (backendLoaded) return null; // Already tried and failed (Storybook/vitest).

  try {
    // Static literal path (no @vite-ignore): this lets Vite code-split the
    // generated bindings into a real chunk that ships in dist/. The previous
    // @vite-ignore + variable form left it as a bare runtime import that was
    // NOT bundled, so in the production (embedded) build it 404'd — every
    // call() silently returned null and all lists/data showed empty. It only
    // worked under `wails3 dev`, where the dev server serves the source
    // bindings. (Storybook/vitest still resolve the chunk; @wailsio/runtime is
    // mocked in tests, and a missing Wails runtime is handled by callers.)
    backendModule = (await import(
      // @ts-expect-error -- generated JS bindings ship without a .d.ts (TS7016);
      // the module is typed via the `as Backend` cast.
      "../../bindings/github.com/neokapi/neokapi/kapi-desktop/backend/app.js"
    )) as Backend;
    backendLoaded = true;
  } catch {
    // Bindings chunk unavailable — mark as permanently failed.
    backendLoaded = true;
    backendModule = null;
  }

  return backendModule;
}

/**
 * Call a Wails backend method. Returns null when not in Wails.
 */
export async function call<T>(method: string, ...args: unknown[]): Promise<T | null> {
  const b = await getBackend();
  if (!b || typeof b[method] !== "function") {
    return null;
  }
  return b[method](...args) as Promise<T>;
}

// --- Typed API functions ---

export const api = {
  // Project (multi-tab)
  newProject: (name: string, sourceLang: string, targetLangs: string[], savePath?: string) =>
    call<TabInfo>("NewProject", name, sourceLang, targetLangs, savePath ?? ""),
  openProject: (path: string) => call<TabInfo>("OpenProject", path),
  openProjectDialog: () => call<TabInfo>("OpenProjectDialog"),
  createSampleProject: (name: string) => call<TabInfo>("CreateSampleProject", name),
  getSampleInfo: (tabID: string) => call<SampleInfo>("GetSampleInfo", tabID),
  resetSampleProject: (tabID: string) => call<TabInfo>("ResetSampleProject", tabID),
  acknowledgeSampleRevision: (tabID: string) => call<void>("AcknowledgeSampleRevision", tabID),
  browseProjectLocation: () => call<string>("BrowseProjectLocation"),
  closeProject: (tabID: string) => call<void>("CloseProject", tabID),
  listTabs: () => call<TabInfo[]>("ListTabs"),
  saveProject: (tabID: string) => call<void>("SaveProject", tabID),
  saveProjectAs: (tabID: string, path: string) => call<void>("SaveProjectAs", tabID, path),
  saveProjectDialog: (tabID: string) => call<TabInfo>("SaveProjectDialog", tabID),
  getProject: (tabID: string) => call<KapiProject>("GetProject", tabID),
  updateProject: (tabID: string, project: KapiProject) =>
    call<void>("UpdateProject", tabID, project),
  getProjectPath: (tabID: string) => call<string>("GetProjectPath", tabID),
  getProjectStatus: (tabID: string) => call<ProjectStatus>("GetProjectStatus", tabID),
  runExtract: (tabID: string) => call<ExtractResult>("RunExtract", tabID),

  // App mode + session (project-first restore)
  getAppMode: () => call<string>("GetAppMode"),
  setAppMode: (mode: string) => call<void>("SetAppMode", mode),
  getSessionState: () => call<SessionState>("GetSessionState"),
  saveSessionState: (state: SessionState) => call<void>("SaveSessionState", state),

  // Flows (scoped to tab)
  listFlows: (tabID: string) => call<FlowInfo[]>("ListFlows", tabID),
  getFlow: (tabID: string, name: string) => call<FlowSpec>("GetFlow", tabID, name),
  saveFlow: (tabID: string, name: string, spec: FlowSpec) =>
    call<void>("SaveFlow", tabID, name, spec),
  deleteFlow: (tabID: string, name: string) => call<void>("DeleteFlow", tabID, name),

  // User flows (ad-hoc, stored in ~/.config/kapi/flows/)
  listUserFlows: () =>
    call<
      Array<{
        id: string;
        name: string;
        description: string;
        source: string;
        step_count: number;
        modified: string;
      }>
    >("ListUserFlows"),
  getUserFlow: (id: string) =>
    call<{ id: string; name: string; description: string; source: string; steps: unknown[] }>(
      "GetUserFlow",
      id,
    ),
  saveUserFlow: (req: { id: string; name: string; description: string; steps: unknown[] }) =>
    call<void>("SaveUserFlow", req),
  deleteUserFlow: (id: string) => call<void>("DeleteUserFlow", id),
  copyBuiltInFlow: (builtInID: string, newName: string) =>
    call<string>("CopyBuiltInFlow", builtInID, newName),
  openFlowFileDialog: () =>
    call<{ id: string; name: string; description: string; source: string; steps: unknown[] }>(
      "OpenFlowFileDialog",
    ),
  saveFlowFileDialog: (name: string, steps: unknown[]) =>
    call<void>("SaveFlowFileDialog", name, steps),

  // Checks (scoped to tab) — runs content checks over the project's files and
  // applies one-click fixes. See backend/checks.go.
  runChecks: (tabID: string, filter: ProjectFilter) =>
    call<CheckRunResult>("RunChecks", tabID, filter),
  applyCheckFix: (
    tabID: string,
    filePath: string,
    blockID: string,
    field: string,
    original: string,
    replacement: string,
  ) => call<void>("ApplyCheckFix", tabID, filePath, blockID, field, original, replacement),

  // Runner (scoped to tab)
  runFlow: (tabID: string, name: string, inputPaths: string[], targetLangs: string[]) =>
    call<void>("RunFlow", tabID, name, inputPaths, targetLangs),
  cancelRun: () => call<void>("CancelRun"),
  getRunState: () => call<string>("GetRunState"),
  getRunEvents: () => call<unknown[]>("GetRunEvents"),

  // Tools
  listTools: () => call<ToolInfo[]>("ListTools"),
  listProjectTools: (tabID: string) => call<ToolInfo[]>("ListProjectTools", tabID),
  validateProjectFlows: (tabID: string) =>
    call<Array<{ flow_name: string; step_tool: string; source: string }>>(
      "ValidateProjectFlows",
      tabID,
    ),
  getToolSchema: (name: string) => call<unknown>("GetToolSchema", name),
  listFormats: () => call<FormatInfo[]>("ListFormats"),
  getFormatSchema: (name: string) => call<unknown>("GetFormatSchema", name),
  detectFormat: (path: string) => call<string>("DetectFormat", path),

  // Locales
  getAllLocales: () => call<Array<{ code: string; display_name: string }>>("GetAllLocales"),
  getKnownLocales: () => call<Array<{ code: string; display_name: string }>>("GetKnownLocales"),

  // Presets
  listPresets: () => call<Array<{ name: string; description: string }>>("ListPresets"),
  detectPreset: (tabID: string) => call<string>("DetectPreset", tabID),
  applyPreset: (tabID: string, presetName: string) =>
    call<KapiProject>("ApplyPreset", tabID, presetName),
  listFormatPresets: (format: string) =>
    call<
      Array<{
        name: string;
        description: string;
        format: string;
        config?: Record<string, unknown>;
        source?: string;
      }>
    >("ListFormatPresets", format),
  saveFormatPreset: (format: string, name: string, config: Record<string, unknown>) =>
    call<void>("SaveFormatPreset", format, name, config),
  deleteFormatPreset: (format: string, name: string) =>
    call<void>("DeleteFormatPreset", format, name),
  listAllFormatPresets: (format: string) =>
    call<
      Array<{
        name: string;
        description: string;
        format: string;
        config?: Record<string, unknown>;
        source?: string;
      }>
    >("ListAllFormatPresets", format),
  renderFormatConfig: (format: string, config: Record<string, unknown>, outputFormat: string) =>
    call<string>("RenderFormatConfig", format, config, outputFormat),
  runFormatReader: (format: string, filePath: string, config: Record<string, unknown>) =>
    call<
      Array<{
        type: string;
        id: string;
        summary: string;
        source_text?: string;
        properties?: Record<string, string>;
      }>
    >("RunFormatReader", format, filePath, config),
  runFormatReaderDialog: (format: string, config: Record<string, unknown>) =>
    call<
      Array<{
        type: string;
        id: string;
        summary: string;
        source_text?: string;
        properties?: Record<string, string>;
      }>
    >("RunFormatReaderDialog", format, config),

  // Plugin docs — summary lists available IDs; individual docs fetched on demand
  getPluginDocsSummary: () => call<PluginDocsSummary>("GetPluginDocs"),
  getFilterDoc: (filterID: string) => call<FilterDoc>("GetFilterDoc", filterID),
  getStepDoc: (stepID: string) => call<StepDoc>("GetStepDoc", stepID),

  // Plugins
  checkProjectPlugins: (tabID: string) => call<PluginStatus>("CheckProjectPlugins", tabID),
  listPlugins: () => call<PluginInfo[]>("ListPlugins"),
  searchPlugins: (query: string) => call<unknown[]>("SearchPlugins", query),
  listAvailablePlugins: () => call<unknown[]>("ListAvailablePlugins"),
  installPlugin: (name: string) => call<void>("InstallPlugin", name),
  updatePlugin: (name: string) => call<void>("UpdatePlugin", name),
  removePlugin: (name: string) => call<void>("RemovePlugin", name),
  checkPluginUpdates: () => call<unknown[]>("CheckPluginUpdates"),

  // Credentials
  listProviders: () => call<ProviderConfig[]>("ListProviders"),
  listProviderTypes: () =>
    call<Array<{ name: string; label: string; local: boolean }>>("ListProviderTypes"),
  saveProvider: (req: unknown) => call<ProviderConfig>("SaveProvider", req),
  deleteProvider: (id: string) => call<void>("DeleteProvider", id),
  testProvider: (id: string) => call<boolean>("TestProvider", id),
  /** Mark a credential as the default for its provider (when several are saved). */
  setProviderDefault: (id: string) => call<void>("SetProviderDefault", id),

  // Active Filter — per-project saved filters (collections + glob + languages).
  getProjectFilters: (tabID: string) => call<ProjectFilters>("GetProjectFilters", tabID),
  saveProjectFilter: (tabID: string, filter: ProjectFilter) =>
    call<ProjectFilter>("SaveProjectFilter", tabID, filter),
  deleteProjectFilter: (tabID: string, id: string) => call<void>("DeleteProjectFilter", tabID, id),
  setActiveFilter: (tabID: string, id: string) => call<void>("SetActiveFilter", tabID, id),

  // AI models — the shared default provider+model (ai.provider/ai.model), the
  // model-first catalog, and the run-time prompt check.
  getDefaultModel: () => call<DefaultModelInfo>("GetDefaultModel"),
  /** Persist the default model; provider "" infers it from the model name. */
  setDefaultModel: (model: string, provider: string) =>
    call<void>("SetDefaultModel", model, provider),
  listAIModels: () => call<AIModelOption[]>("ListAIModels"),
  aiNeedsModelChoice: (tabID: string, flowName: string) =>
    call<boolean>("AINeedsModelChoice", tabID, flowName),

  // Files
  matchContent: (tabID: string) =>
    call<
      Array<{
        path: string;
        format: string;
        relative: string;
        pattern: string;
        collection: string;
      }>
    >("MatchContent", tabID),
  getBasePath: (tabID: string) => call<string>("GetBasePath", tabID),
  isEmptyProject: (tabID: string) => call<boolean>("IsEmptyProject", tabID),
  listProjectFiles: (tabID: string) =>
    call<
      Array<{
        path: string;
        relative: string;
        format: string;
        size: number;
        is_dir: boolean;
      }>
    >("ListProjectFiles", tabID),
  applyTemplate: (tabID: string, template: string) => call<void>("ApplyTemplate", tabID, template),
  copyFileToProject: (tabID: string, srcPath: string, destDir: string) =>
    call<string>("CopyFileToProject", tabID, srcPath, destDir),
  addFilesDialog: (tabID: string, destDir: string) =>
    call<string[]>("AddFilesDialog", tabID, destDir),
  // Generic file/folder browse used by schema-form path widgets.
  // Returns the chosen path, "" when the user cancels, or null outside Wails.
  browsePath: (req: BrowsePathRequest) => call<string>("BrowsePath", req),

  // Recent files
  listRecentFiles: () =>
    call<Array<{ path: string; name: string; opened_at: string }>>("ListRecentFiles"),
  clearRecentFiles: () => call<void>("ClearRecentFiles"),

  // Settings
  getSettings: () =>
    call<{ theme: string; ui_language?: string; samples_dismissed?: boolean }>("GetSettings"),
  saveSettings: (s: Record<string, unknown>) => call<void>("SaveSettings", s),
  dismissSamples: () => call<void>("DismissSamples"),
  getTheme: () => call<string>("GetTheme"),
  setTheme: (theme: string) => call<void>("SetTheme", theme),
  getUILanguage: () => call<string>("GetUILanguage"),
  setUILanguage: (lang: string) => call<void>("SetUILanguage", lang),

  // Recovery
  recoverResource: (path: string) => call<string>("RecoverResource", path),

  // Project resource handles
  getProjectTMHandle: (tabID: string) => call<string>("GetProjectTMHandle", tabID),
  getProjectTermbaseHandle: (tabID: string) => call<string>("GetProjectTermbaseHandle", tabID),
  getProjectHandles: (tabID: string) => call<ProjectHandles>("GetProjectHandles", tabID),

  // Adopt a user/ad-hoc flow into the active project's recipe.
  adoptUserFlowIntoProject: (tabID: string, flowID: string) =>
    call<AdoptFlowResult>("AdoptUserFlowIntoProject", tabID, flowID),

  // TM
  listNamedTMs: () =>
    call<Array<{ name: string; path: string; size: number; modified: string }>>("ListNamedTMs"),
  getTMStats: (handle: string) => call<{ count: number; path: string }>("GetTMStats", handle),
  getTMLocaleStats: (handle: string) =>
    call<Array<{ locale: string; count: number }>>("GetTMLocaleStats", handle),
  getTMActivityStats: (handle: string) =>
    call<Array<{ date: string; count: number }>>("GetTMActivityStats", handle),
  openTM: (path: string) => call<string>("OpenTM", path),
  openTMDialog: () => call<string>("OpenTMDialog"),
  createTM: (path: string) => call<string>("CreateTM", path),
  createNamedTM: (name: string) => call<string>("CreateNamedTM", name),
  closeTM: (handle: string) => call<void>("CloseTM", handle),
  searchTMEntries: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    offset: number,
    limit: number,
  ) => call<unknown>("SearchTMEntries", handle, query, anyLocale, requireLocale, offset, limit),
  getTMEntry: (handle: string, id: string) => call<unknown>("GetTMEntry", handle, id),
  addTMEntry: (handle: string, req: unknown) => call<void>("AddTMEntry", handle, req),
  updateTMEntry: (handle: string, req: unknown) => call<void>("UpdateTMEntry", handle, req),
  deleteTMEntry: (handle: string, id: string) => call<void>("DeleteTMEntry", handle, id),
  deleteTMEntries: (handle: string, ids: string[]) => call<void>("DeleteTMEntries", handle, ids),
  lookupTM: (handle: string, req: unknown) => call<unknown[]>("LookupTM", handle, req),
  annotateEntities: (handle: string, req: unknown) =>
    call<unknown>("AnnotateEntities", handle, req),
  importTMXDialog: (handle: string) =>
    call<{ session_id: string; count: number }>("ImportTMXDialog", handle),
  exportTMXDialog: (handle: string, locales: string[]) =>
    call<void>("ExportTMXDialog", handle, locales),
  getTMFacets: (handle: string) => call<unknown>("GetTMFacets", handle),
  getTMFacetsFiltered: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: unknown,
  ) => call<unknown>("GetTMFacetsFiltered", handle, query, anyLocale, requireLocale, filter),
  searchTMEntriesFiltered: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: unknown,
    offset: number,
    limit: number,
  ) =>
    call<unknown>(
      "SearchTMEntriesFiltered",
      handle,
      query,
      anyLocale,
      requireLocale,
      filter,
      offset,
      limit,
    ),
  listTMImportSessions: (handle: string) => call<unknown[]>("ListTMImportSessions", handle),
  getTMImportSession: (handle: string, sessionID: string) =>
    call<unknown>("GetTMImportSession", handle, sessionID),
  deleteTMImportSession: (handle: string, sessionID: string) =>
    call<void>("DeleteTMImportSession", handle, sessionID),

  // Termbase
  listNamedTermbases: () =>
    call<Array<{ name: string; path: string; size: number; modified: string }>>(
      "ListNamedTermbases",
    ),
  getTermbaseStats: (handle: string) => call<{ count: number }>("GetTermbaseStats", handle),
  getTermbaseLocaleStats: (handle: string) =>
    call<Array<{ locale: string; count: number }>>("GetTermbaseLocaleStats", handle),
  getTermbaseActivityStats: (handle: string) =>
    call<Array<{ date: string; count: number }>>("GetTermbaseActivityStats", handle),
  openTermbase: (path: string) => call<string>("OpenTermbase", path),
  openTermbaseDialog: () => call<string>("OpenTermbaseDialog"),
  createTermbase: (path: string) => call<string>("CreateTermbase", path),
  createNamedTermbase: (name: string) => call<string>("CreateNamedTermbase", name),
  closeTermbase: (handle: string) => call<void>("CloseTermbase", handle),
  searchTerms: (
    handle: string,
    query: string,
    srcLocale: string,
    tgtLocale: string,
    offset: number,
    limit: number,
  ) => call<unknown>("SearchTerms", handle, query, srcLocale, tgtLocale, offset, limit),
  getConcept: (handle: string, id: string) => call<unknown>("GetConcept", handle, id),
  addConcept: (handle: string, req: unknown) => call<void>("AddConcept", handle, req),
  updateConcept: (handle: string, req: unknown) => call<void>("UpdateConcept", handle, req),
  deleteConcept: (handle: string, id: string) => call<void>("DeleteConcept", handle, id),
  deleteConcepts: (handle: string, ids: string[]) => call<void>("DeleteConcepts", handle, ids),
  importTermbaseCSVDialog: (handle: string, srcLocale: string, tgtLocale: string, domain: string) =>
    call<{ count: number }>("ImportTermbaseCSVDialog", handle, srcLocale, tgtLocale, domain),
  importTermbaseJSONDialog: (handle: string) =>
    call<{ count: number }>("ImportTermbaseJSONDialog", handle),
  exportTermbaseJSONDialog: (handle: string, name: string) =>
    call<void>("ExportTermbaseJSONDialog", handle, name),

  // Inspect — returns the editor ContentTree (as JSON) for a project content
  // file, the structure the PreviewKit (DocumentViewer) renders. The annotated
  // variant additionally carries source-anchored term / brand / QA overlays.
  // See backend/inspect.go.
  inspectFile: (tabID: string, filePath: string) => call<string>("InspectFile", tabID, filePath),
  inspectFileAnnotated: (tabID: string, filePath: string) =>
    call<string>("InspectFileAnnotated", tabID, filePath),
  // Reads a media file (image/audio/video) from disk and returns a base64 data:
  // URL the DocumentViewer can render directly. The path is a tree media node's URI.
  mediaDataURL: (path: string) => call<string>("MediaDataURL", path),

  // Preview
  previewFlow: (
    tabID: string,
    flowName: string,
    sampleText: string,
    sourceLang: string,
    targetLang: string,
  ) => call<unknown>("PreviewFlow", tabID, flowName, sampleText, sourceLang, targetLang),

  // Trace
  getLastTrace: () => call<unknown>("GetLastTrace"),

  // Outputs — generated target files per source file in a content collection.
  listOutputs: (tabID: string) => call<Record<string, OutputFileInfo[]>>("ListOutputs", tabID),
  inspectOutput: (tabID: string, relative: string) =>
    call<string>("InspectOutput", tabID, relative),

  // System
  getVersion: () => call<VersionInfo>("GetVersion"),
  getHomeDir: () => call<string>("GetHomeDir"),
} as const;

export interface SampleInfo {
  is_sample: boolean;
  name?: string;
  display_name?: string;
  on_disk_revision: number;
  current_revision: number;
  upgrade_available: boolean;
}

export interface VersionInfo {
  version: string;
  commit: string;
  build_date: string;
}

export interface OutputFileInfo {
  lang: string;
  path: string;
  relative: string;
  format?: string;
  exists: boolean;
  size: number;
  mod_time?: string;
}

export type Api = typeof api;
