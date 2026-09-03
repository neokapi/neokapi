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
  AIModelResolution,
  AIModelOption,
  AIDetectionResult,
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
  ConvergenceReport,
  ConvergePlan,
  RunError,
  ProjectServer,
  ReviewContext,
  ReviewQueue,
  ReviewUnitDetail,
  ReviewAIActionResult,
  ReviewAIActionKind,
  PreReviewScope,
  PreReviewPolicy,
  AIActivityResult,
  PreReviewResult,
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
  getConvergence: (tabID: string) => call<ConvergenceReport>("GetConvergence", tabID),
  getConvergePlan: (tabID: string) => call<ConvergePlan>("GetConvergePlan", tabID),
  getProjectServer: (tabID: string) => call<ProjectServer>("GetProjectServer", tabID),
  bringUpToDate: (tabID: string) => call<void>("BringUpToDate", tabID),
  approveReviewItem: (tabID: string, locale: string, file: string, key: string) =>
    call<void>("ApproveReviewItem", tabID, locale, file, key),

  // Review surface — queue with findings enrichment, per-unit detail, and the
  // decision verbs (all recorded through cli.ApplyReviewDecision).
  /** The unified review queue: every unit awaiting a person across the
   *  project's languages, the source language among them, plus the pending
   *  count per language. */
  reviewQueue: (tabID: string, filter: ProjectFilter) =>
    call<ReviewQueue>("ReviewQueue", tabID, filter),
  getReviewUnit: (tabID: string, locale: string, file: string, key: string) =>
    call<ReviewUnitDetail>("GetReviewUnit", tabID, locale, file, key),
  rejectReviewItem: (tabID: string, locale: string, file: string, key: string, note: string) =>
    call<void>("RejectReviewItem", tabID, locale, file, key, note),
  signOffReviewItem: (tabID: string, locale: string, file: string, key: string) =>
    call<void>("SignOffReviewItem", tabID, locale, file, key),
  updateReviewTarget: (tabID: string, locale: string, file: string, key: string, text: string) =>
    call<void>("UpdateReviewTarget", tabID, locale, file, key, text),
  /** Per-unit AI action (fix-findings | retranslate | explain). Explicit
   * invocation only — the sole review paths that reach a provider. */
  reviewAIAction: (
    tabID: string,
    locale: string,
    file: string,
    key: string,
    action: ReviewAIActionKind,
    instruction: string,
  ) => call<ReviewAIActionResult>("ReviewAIAction", tabID, locale, file, key, action, instruction),
  /** The point and the neighbourhood for one source unit, so the source lane
   *  judges wording against the same context the target lane does. */
  getSourceUnitContext: (tabID: string, file: string, key: string) =>
    call<ReviewContext>("GetSourceUnitContext", tabID, file, key),
  approveSourceUnit: (tabID: string, file: string, key: string) =>
    call<void>("ApproveSourceUnit", tabID, file, key),
  updateSourceText: (tabID: string, file: string, key: string, text: string) =>
    call<string[]>("UpdateSourceText", tabID, file, key, text),
  getAIActivity: (limit: number) => call<AIActivityResult>("GetAIActivity", limit),
  clearAIActivity: () => call<void>("ClearAIActivity"),
  runAIPreReview: (tabID: string, locale: string, scope: PreReviewScope, policy: PreReviewPolicy) =>
    call<PreReviewResult>("RunAIPreReview", tabID, locale, scope, policy),

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
  // Persist a flow's steps (and inline options) to the recipe on disk.
  saveProjectFlow: (tabID: string, name: string, spec: FlowSpec) =>
    call<void>("SaveProjectFlow", tabID, name, spec),
  // Set (or clear, with "") the project's default flow and write the recipe.
  setDefaultFlow: (tabID: string, name: string) => call<void>("SetDefaultFlow", tabID, name),
  // Rename a flow (moving the default with it) and write the recipe.
  renameProjectFlow: (tabID: string, oldName: string, newName: string) =>
    call<void>("RenameProjectFlow", tabID, oldName, newName),
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
        steps?: string[];
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
  /**
   * The classified failure of the most recent run, or null when the last run
   * succeeded. Coverage is derived from files and cannot express "the run that
   * was supposed to write these files failed", so the home surfaces read this
   * to avoid presenting a failed catch-up as convergence.
   */
  getLastRunError: () => call<RunError | null>("GetLastRunError"),

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
    call<
      Array<{
        name: string;
        label: string;
        local: boolean;
        keyless: boolean;
        subscription?: boolean;
      }>
    >("ListProviderTypes"),
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
  /**
   * The provider/model a run of this project will actually use, and which layer
   * decided it — recipe locale preset, recipe project preset, the shared `ai.*`
   * app config (`kapi models setup` / `kapi models default`), or the built-in
   * fallback. Resolved by the same host.EffectiveAIModel the CLI reads, so the
   * displayed model cannot drift from the one that runs.
   */
  getEffectiveModel: (tabID: string, locale = "") =>
    call<AIModelResolution>("GetEffectiveModel", tabID, locale),
  /** Persist the default model; provider "" infers it from the model name. */
  setDefaultModel: (model: string, provider: string) =>
    call<void>("SetDefaultModel", model, provider),
  listAIModels: () => call<AIModelOption[]>("ListAIModels"),
  aiNeedsModelChoice: (tabID: string, flowName: string) =>
    call<boolean>("AINeedsModelChoice", tabID, flowName),
  /** Detect the machine's AI options (Claude Code binary, Ollama server,
   * env keys, saved credentials) — instant and offline. */
  detectAIProviders: () => call<AIDetectionResult>("DetectAIProviders"),
  /** Persist a provider (with optional model — provider default when "") as
   * the app-level AI default the runner and review AI read. */
  selectAIProvider: (providerID: string, model: string) =>
    call<void>("SelectAIProvider", providerID, model),

  // Files
  matchContent: (tabID: string) =>
    call<
      Array<{
        path: string;
        format: string;
        relative: string;
        /** The effective glob, with a collection `base:` already folded in. */
        pattern: string;
        collection: string;
        /** The recipe entry this file came from — the join a surface uses. */
        collection_index: number;
        item_index: number;
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
    call<{
      theme: string;
      ui_language?: string;
      samples_dismissed?: boolean;
      telemetry_disabled?: boolean;
      telemetry_notice_shown?: boolean;
      machine_id?: string;
    }>("GetSettings"),
  saveSettings: (s: Record<string, unknown>) => call<void>("SaveSettings", s),
  // Telemetry is shared with the kapi CLI: these write the same config store
  // `kapi telemetry on|off` writes, not settings.json.
  setTelemetryEnabled: (enabled: boolean) => call<void>("SetTelemetryEnabled", enabled),
  markTelemetryNoticeShown: () => call<void>("MarkTelemetryNoticeShown"),
  dismissSamples: () => call<void>("DismissSamples"),
  getTheme: () => call<string>("GetTheme"),
  setTheme: (theme: string) => call<void>("SetTheme", theme),
  getUILanguage: () => call<string>("GetUILanguage"),
  setUILanguage: (lang: string) => call<void>("SetUILanguage", lang),

  // Recovery
  recoverResource: (path: string) => call<string>("RecoverResource", path),

  // Project resource handles
  getProjectMemoryHandle: (tabID: string) => call<string>("GetProjectMemoryHandle", tabID),
  getProjectTermsHandle: (tabID: string) => call<string>("GetProjectTermsHandle", tabID),
  getProjectHandles: (tabID: string) => call<ProjectHandles>("GetProjectHandles", tabID),

  // Adopt a user/ad-hoc flow into the active project's recipe.
  adoptUserFlowIntoProject: (tabID: string, flowID: string) =>
    call<AdoptFlowResult>("AdoptUserFlowIntoProject", tabID, flowID),

  // content memory
  listNamedMemories: () =>
    call<Array<{ name: string; path: string; size: number; modified: string }>>(
      "ListNamedMemories",
    ),
  getMemoryStats: (handle: string) =>
    call<{ count: number; path: string }>("GetMemoryStats", handle),
  getMemoryLocaleStats: (handle: string) =>
    call<Array<{ locale: string; count: number }>>("GetMemoryLocaleStats", handle),
  getMemoryActivityStats: (handle: string) =>
    call<Array<{ date: string; count: number }>>("GetMemoryActivityStats", handle),
  openMemory: (path: string) => call<string>("OpenMemory", path),
  openMemoryDialog: () => call<string>("OpenMemoryDialog"),
  createMemory: (path: string) => call<string>("CreateMemory", path),
  createNamedMemory: (name: string) => call<string>("CreateNamedMemory", name),
  closeMemory: (handle: string) => call<void>("CloseMemory", handle),
  searchMemoryEntries: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    offset: number,
    limit: number,
  ) => call<unknown>("SearchMemoryEntries", handle, query, anyLocale, requireLocale, offset, limit),
  getMemoryEntry: (handle: string, id: string) => call<unknown>("GetMemoryEntry", handle, id),
  addMemoryEntry: (handle: string, req: unknown) => call<void>("AddMemoryEntry", handle, req),
  updateMemoryEntry: (handle: string, req: unknown) => call<void>("UpdateMemoryEntry", handle, req),
  deleteMemoryEntry: (handle: string, id: string) => call<void>("DeleteMemoryEntry", handle, id),
  deleteMemoryEntries: (handle: string, ids: string[]) =>
    call<void>("DeleteMemoryEntries", handle, ids),
  lookupMemory: (handle: string, req: unknown) => call<unknown[]>("LookupMemory", handle, req),
  annotateEntities: (handle: string, req: unknown) =>
    call<unknown>("AnnotateEntities", handle, req),
  getMemoryFacets: (handle: string) => call<unknown>("GetMemoryFacets", handle),
  getMemoryFacetsFiltered: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: unknown,
  ) => call<unknown>("GetMemoryFacetsFiltered", handle, query, anyLocale, requireLocale, filter),
  searchMemoryEntriesFiltered: (
    handle: string,
    query: string,
    anyLocale: string,
    requireLocale: string,
    filter: unknown,
    offset: number,
    limit: number,
  ) =>
    call<unknown>(
      "SearchMemoryEntriesFiltered",
      handle,
      query,
      anyLocale,
      requireLocale,
      filter,
      offset,
      limit,
    ),
  listMemoryImportSessions: (handle: string) => call<unknown[]>("ListMemoryImportSessions", handle),
  getMemoryImportSession: (handle: string, sessionID: string) =>
    call<unknown>("GetMemoryImportSession", handle, sessionID),
  deleteMemoryImportSession: (handle: string, sessionID: string) =>
    call<void>("DeleteMemoryImportSession", handle, sessionID),

  // Terms
  listNamedTerms: () =>
    call<Array<{ name: string; path: string; size: number; modified: string }>>("ListNamedTerms"),
  getTermsStats: (handle: string) => call<{ count: number }>("GetTermsStats", handle),
  getTermsLocaleStats: (handle: string) =>
    call<Array<{ locale: string; count: number }>>("GetTermsLocaleStats", handle),
  getTermsActivityStats: (handle: string) =>
    call<Array<{ date: string; count: number }>>("GetTermsActivityStats", handle),
  openTerms: (path: string) => call<string>("OpenTerms", path),
  openTermsDialog: () => call<string>("OpenTermsDialog"),
  createTerms: (path: string) => call<string>("CreateTerms", path),
  createNamedTerms: (name: string) => call<string>("CreateNamedTerms", name),
  closeTerms: (handle: string) => call<void>("CloseTerms", handle),
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

  // Inspect — returns the editor ContentTree (as JSON) for a project content
  // file, the structure the PreviewKit (DocumentViewer) renders. The annotated
  // variant additionally carries source-anchored term / brand / check overlays.
  // See backend/inspect.go.
  inspectFile: (tabID: string, filePath: string) => call<string>("InspectFile", tabID, filePath),
  inspectFileAnnotated: (tabID: string, filePath: string) =>
    call<string>("InspectFileAnnotated", tabID, filePath),
  // The file as the next merge will write it for a locale: the same reader,
  // writer and recipe configuration `kapi merge` materializes with. An empty
  // locale returns the source file.
  writtenBackFile: (tabID: string, filePath: string, locale: string) =>
    call<string>("WrittenBackFile", tabID, filePath, locale),
  // Archive (ZIP/TAR/TAR.GZ) container support — list inner entries and preview
  // one of them (the desktop equivalent of the `archive.zip!entry` locator).
  listArchiveEntries: (filePath: string) =>
    call<Array<{ name: string; format: string; size: number }>>("ListArchiveEntries", filePath),
  inspectArchiveEntry: (tabID: string, archivePath: string, entry: string) =>
    call<string>("InspectArchiveEntry", tabID, archivePath, entry),
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
