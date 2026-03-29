/**
 * API hooks that bridge the React frontend to the Wails Go backend.
 *
 * In production (Wails runtime), these call the generated bindings directly.
 * In Storybook/tests, they fall back to mock implementations.
 *
 * Usage:
 *   const { project, openProject, saveProject } = useProject();
 *   const { tools } = useTools();
 *   const { runFlow, state } = useFlowRunner();
 */

import type {
  KapiProject,
  FlowInfo,
  FlowSpec,
  ToolInfo,
  FormatInfo,
  PluginInfo,
  ProviderConfig,
  View,
} from "../types/api";

// Check if Wails runtime is available (injected by desktop app).
const isWails = typeof window !== "undefined" && "go" in window;

/**
 * Dynamically import Wails-generated backend bindings.
 * Returns null when running outside Wails (Storybook, vitest).
 */
async function getBackend(): Promise<Record<string, (...args: unknown[]) => Promise<unknown>> | null> {
  if (!isWails) return null;
  try {
    // Wails v3 generates bindings at this path after `wails3 generate bindings`.
    // The variable indirection prevents Vite from statically resolving the import.
    const bindingPath = "../../bindings/github.com/neokapi/neokapi/kapi-desktop/backend/app.js";
    const mod = await import(/* @vite-ignore */ bindingPath);
    return mod as Record<string, (...args: unknown[]) => Promise<unknown>>;
  } catch {
    return null;
  }
}

// Cached backend reference.
let backendPromise: Promise<Record<string, (...args: unknown[]) => Promise<unknown>> | null> | null = null;

function backend() {
  if (!backendPromise) {
    backendPromise = getBackend();
  }
  return backendPromise;
}

/**
 * Call a Wails backend method. Falls back gracefully when not in Wails.
 */
export async function call<T>(method: string, ...args: unknown[]): Promise<T | null> {
  const b = await backend();
  if (!b || !(method in b)) {
    // Expected when running outside Wails (Storybook, vitest, browser dev).
    return null;
  }
  return b[method](...args) as Promise<T>;
}

// --- Typed API functions ---

export const api = {
  // Project
  newProject: (name: string, sourceLang: string, targetLangs: string[]) =>
    call<KapiProject>("NewProject", name, sourceLang, targetLangs),
  openProject: (path: string) => call<KapiProject>("OpenProject", path),
  openProjectDialog: () => call<KapiProject>("OpenProjectDialog"),
  saveProject: () => call<void>("SaveProject"),
  saveProjectAs: (path: string) => call<void>("SaveProjectAs", path),
  saveProjectDialog: () => call<void>("SaveProjectDialog"),
  getProject: () => call<KapiProject>("GetProject"),
  getProjectPath: () => call<string>("GetProjectPath"),

  // Flows
  listFlows: () => call<FlowInfo[]>("ListFlows"),
  getFlow: (name: string) => call<FlowSpec>("GetFlow", name),
  saveFlow: (name: string, spec: FlowSpec) => call<void>("SaveFlow", name, spec),
  deleteFlow: (name: string) => call<void>("DeleteFlow", name),

  // Runner
  runFlow: (name: string, inputPaths: string[], targetLang: string) =>
    call<void>("RunFlow", name, inputPaths, targetLang),
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
  listRecentFiles: () => call<Array<{ path: string; name: string; opened_at: string }>>("ListRecentFiles"),
  clearRecentFiles: () => call<void>("ClearRecentFiles"),

  // Settings
  getSettings: () => call<{ theme: string; plugin_dir: string }>("GetSettings"),
  saveSettings: (s: { theme: string; plugin_dir: string }) => call<void>("SaveSettings", s),
  getTheme: () => call<string>("GetTheme"),
  setTheme: (theme: string) => call<void>("SetTheme", theme),

  // Version
  getVersion: () => call<string>("GetVersion"),
} as const;

export type Api = typeof api;
