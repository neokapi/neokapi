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
  ProviderConfig,
} from "../types/api";

type Backend = Record<string, (...args: unknown[]) => Promise<unknown>>;

let backendModule: Backend | null = null;
let backendLoaded = false;

/**
 * Lazily load the Wails-generated backend bindings.
 * Returns null when bindings aren't available (Storybook, vitest).
 */
async function getBackend(): Promise<Backend | null> {
  if (backendLoaded) return backendModule;
  backendLoaded = true;

  try {
    // Wails v3 generates bindings at this path via `wails3 generate bindings`.
    // The variable prevents Vite from statically resolving the import.
    const path = "../../bindings/github.com/neokapi/neokapi/kapi-desktop/backend/app.js";
    backendModule = (await import(/* @vite-ignore */ path)) as Backend;
  } catch {
    // Expected in Storybook/vitest — no Wails runtime available.
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
  newProject: (name: string, sourceLang: string, targetLangs: string[]) =>
    call<TabInfo>("NewProject", name, sourceLang, targetLangs),
  openProject: (path: string) => call<TabInfo>("OpenProject", path),
  openProjectDialog: () => call<TabInfo>("OpenProjectDialog"),
  closeProject: (tabID: string) => call<void>("CloseProject", tabID),
  listTabs: () => call<TabInfo[]>("ListTabs"),
  saveProject: (tabID: string) => call<void>("SaveProject", tabID),
  saveProjectAs: (tabID: string, path: string) => call<void>("SaveProjectAs", tabID, path),
  saveProjectDialog: (tabID: string) => call<void>("SaveProjectDialog", tabID),
  getProject: (tabID: string) => call<KapiProject>("GetProject", tabID),
  getProjectPath: (tabID: string) => call<string>("GetProjectPath", tabID),

  // Flows (scoped to tab)
  listFlows: (tabID: string) => call<FlowInfo[]>("ListFlows", tabID),
  getFlow: (tabID: string, name: string) => call<FlowSpec>("GetFlow", tabID, name),
  saveFlow: (tabID: string, name: string, spec: FlowSpec) => call<void>("SaveFlow", tabID, name, spec),
  deleteFlow: (tabID: string, name: string) => call<void>("DeleteFlow", tabID, name),

  // Runner (scoped to tab)
  runFlow: (tabID: string, name: string, inputPaths: string[], targetLang: string) =>
    call<void>("RunFlow", tabID, name, inputPaths, targetLang),
  cancelRun: () => call<void>("CancelRun"),
  getRunState: () => call<string>("GetRunState"),

  // Tools
  listTools: () => call<ToolInfo[]>("ListTools"),
  getToolSchema: (name: string) => call<unknown>("GetToolSchema", name),
  listFormats: () => call<FormatInfo[]>("ListFormats"),
  detectFormat: (path: string) => call<string>("DetectFormat", path),

  // Plugins
  listPlugins: () => call<PluginInfo[]>("ListPlugins"),
  searchPlugins: (query: string) => call<unknown[]>("SearchPlugins", query),
  listAvailablePlugins: () => call<unknown[]>("ListAvailablePlugins"),
  installPlugin: (name: string) => call<void>("InstallPlugin", name),
  updatePlugin: (name: string) => call<void>("UpdatePlugin", name),
  removePlugin: (name: string) => call<void>("RemovePlugin", name),
  checkPluginUpdates: () => call<unknown[]>("CheckPluginUpdates"),

  // Credentials
  listProviders: () => call<ProviderConfig[]>("ListProviders"),
  saveProvider: (req: unknown) => call<ProviderConfig>("SaveProvider", req),
  deleteProvider: (id: string) => call<void>("DeleteProvider", id),
  testProvider: (id: string) => call<boolean>("TestProvider", id),

  // Files
  matchContent: (basePath: string) => call<unknown[]>("MatchContent", basePath),

  // Recent files
  listRecentFiles: () =>
    call<Array<{ path: string; name: string; opened_at: string }>>("ListRecentFiles"),
  clearRecentFiles: () => call<void>("ClearRecentFiles"),

  // Settings
  getSettings: () => call<{ theme: string; plugin_dir: string }>("GetSettings"),
  saveSettings: (s: { theme: string; plugin_dir: string }) =>
    call<void>("SaveSettings", s),
  getTheme: () => call<string>("GetTheme"),
  setTheme: (theme: string) => call<void>("SetTheme", theme),

  // Version
  getVersion: () => call<string>("GetVersion"),
} as const;

export type Api = typeof api;
