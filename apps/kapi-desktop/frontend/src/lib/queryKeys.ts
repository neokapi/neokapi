/**
 * Query-key factory for Kapi Desktop react-query usage.
 *
 * Centralising the keys keeps invalidation (`hooks/useInvalidateOnEvent`) and
 * the query sites in agreement — a Wails event names the domain to invalidate,
 * and the components read the same key here.
 *
 * Keys are grouped by domain. Tab-scoped data includes the tab id so switching
 * tabs reads the correct cache entry.
 */
export const qk = {
  // Formats / filter docs
  formats: () => ["formats"] as const,
  formatSchema: (name: string) => ["formats", "schema", name] as const,
  formatPresets: (name: string) => ["formats", "presets", name] as const,
  formatPresetsAll: (name: string) => ["formats", "presets-all", name] as const,
  pluginDocsSummary: () => ["plugin-docs", "summary"] as const,
  filterDoc: (id: string) => ["plugin-docs", "filter", id] as const,
  stepDoc: (id: string) => ["plugin-docs", "step", id] as const,

  // Tools
  tools: () => ["tools"] as const,
  projectTools: (tabID: string) => ["tools", "project", tabID] as const,
  toolSchema: (name: string) => ["tools", "schema", name] as const,

  // Flows (project-scoped + user/ad-hoc)
  flows: (tabID: string) => ["flows", tabID] as const,
  flow: (tabID: string, name: string) => ["flows", tabID, name] as const,
  userFlows: () => ["user-flows"] as const,
  userFlow: (id: string) => ["user-flows", id] as const,

  // Plugins
  plugins: () => ["plugins"] as const,
  availablePlugins: () => ["plugins", "available"] as const,
  pluginUpdates: () => ["plugins", "updates"] as const,

  // Credentials / AI providers + models
  providers: () => ["providers"] as const,
  providerTypes: () => ["providers", "types"] as const,
  /** Combined AI-model bundle (providers + types + catalog + default + detection). */
  aiModelData: () => ["ai", "model-data"] as const,
  /** The provider/model a run will actually use, per (project, locale) scope. */
  effectiveModel: (tabID: string, locale: string) =>
    ["ai", "effective-model", tabID, locale] as const,
  defaultModel: () => ["ai", "default-model"] as const,
  aiModels: () => ["ai", "models"] as const,
  aiDetection: () => ["ai", "detection"] as const,

  // Locales
  allLocales: () => ["locales", "all"] as const,
  knownLocales: () => ["locales", "known"] as const,

  // Presets
  presets: () => ["presets"] as const,

  // Project content / outputs
  projectFiles: (tabID: string) => ["project-files", tabID] as const,
  matchContent: (tabID: string) => ["project-content", tabID] as const,
  outputs: (tabID: string) => ["outputs", tabID] as const,

  // content memory
  namedMemories: () => ["tm", "named"] as const,
  memoryStats: (handle: string) => ["tm", "stats", handle] as const,
  memoryLocaleStats: (handle: string) => ["tm", "locale-stats", handle] as const,
  memoryActivityStats: (handle: string) => ["tm", "activity-stats", handle] as const,

  // Terms
  namedTerms: () => ["terms", "named"] as const,
  termsStats: (handle: string) => ["terms", "stats", handle] as const,
  termsLocaleStats: (handle: string) => ["terms", "locale-stats", handle] as const,
  termsActivityStats: (handle: string) => ["terms", "activity-stats", handle] as const,

  // Settings / system
  settings: () => ["settings"] as const,
  recentFiles: () => ["recent-files"] as const,
  version: () => ["version"] as const,
  homeDir: () => ["home-dir"] as const,

  // Voice — the profile resolved at every point the recipe declares
  projectVoice: (tabID: string) => ["project-voice", tabID] as const,
  /** The values each constrained profile field accepts. */
  voiceFieldValues: () => ["voice", "field-values"] as const,
  voiceStarterPacks: () => ["voice", "starter-packs"] as const,
  /** The axes, channels and profiles the open recipe can name. */
  recipeGovernance: (tabID: string) => ["recipe-governance", tabID] as const,
  /** Every coordinate point the recipe declares, and what governs there. */
  projectPoints: (tabID: string) => ["project-points", tabID] as const,
  /** The channel map: where content sits, and what governs and how much. */
  channelMap: (tabID: string) => ["channel-map", tabID] as const,

  // Project model / handles
  project: (tabID: string) => ["project", tabID] as const,
  projectHandles: (tabID: string) => ["project-handles", tabID] as const,

  // Project state / convergence / review
  projectStatus: (tabID: string) => ["project-status", tabID] as const,
  convergence: (tabID: string) => ["convergence", tabID] as const,
  convergePlan: (tabID: string) => ["converge-plan", tabID] as const,
  projectServer: (tabID: string) => ["project-server", tabID] as const,
  projectFilters: (tabID: string) => ["project-filters", tabID] as const,
  extractResult: (tabID: string) => ["extract-result", tabID] as const,
  checks: (tabID: string, filterID: string) => ["checks", tabID, filterID] as const,
  reviewQueue: (tabID: string) => ["review-queue", tabID] as const,
  reviewUnit: (tabID: string, locale: string, file: string, key: string) =>
    ["review-unit", tabID, locale, file, key] as const,

  // Inspect / preview / archives
  inspectFile: (tabID: string, filePath: string, annotated: boolean) =>
    ["inspect", tabID, filePath, annotated] as const,
  inspectArchiveEntry: (tabID: string, archivePath: string, entry: string) =>
    ["inspect-archive", tabID, archivePath, entry] as const,
  archiveEntries: (filePath: string) => ["archive-entries", filePath] as const,
  writtenBackFile: (tabID: string, filePath: string, locale: string) =>
    ["written-back", tabID, filePath, locale] as const,
} as const;
