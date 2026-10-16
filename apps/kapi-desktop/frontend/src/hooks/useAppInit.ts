import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "./useInvalidateOnEvent";
import type { ContextProjectStanding, WorkspaceHome } from "../types/api";

/** The schemes a link may hand to the system browser. */
const EXTERNAL_SCHEMES = new Set(["http:", "https:", "mailto:"]);

/** What a click on a link does: stay in the app, open a URL outside it, or nothing. */
export type LinkClickAction = { kind: "app" } | { kind: "open"; url: string } | { kind: "refuse" };

/**
 * What a click on a link with this href does. A fragment or a path ("#…",
 * "/…") stays in the app. An absolute http, https or mailto URL opens in the
 * system browser. Anything else is refused and neither opens nor navigates the
 * webview: a `javascript:`, `file:` or `data:` URL, a protocol-relative
 * "//host" that would load another origin into the webview, and an href that
 * does not parse. Links can carry project data, such as a recipe's URLs, so
 * the scheme is checked here for every link in the app.
 */
export function linkClickAction(href: string | null): LinkClickAction {
  if (!href || href.startsWith("#")) return { kind: "app" };
  if (href.startsWith("/") && !href.startsWith("//")) return { kind: "app" };
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return { kind: "refuse" };
  }
  return EXTERNAL_SCHEMES.has(url.protocol) ? { kind: "open", url: url.href } : { kind: "refuse" };
}

/** Handle a click on the document: act on the link it landed in, if any. */
export function interceptLinkClick(e: MouseEvent, open: (url: string) => void): void {
  const target = e.target as Element | null;
  const anchor = target?.closest?.("a[href]") as HTMLAnchorElement | null;
  if (!anchor) return;
  const action = linkClickAction(anchor.getAttribute("href"));
  if (action.kind === "app") return;
  e.preventDefault();
  if (action.kind === "open") open(action.url);
}

/** Open a URL in the system browser, or in a new window outside Wails. */
function openInSystemBrowser(url: string): void {
  import("@wailsio/runtime")
    .then((m) => m.Browser.OpenURL(url))
    .catch(() => {
      window.open(url, "_blank", "noopener,noreferrer");
    });
}

/**
 * App-level initialization: theme, the workspace's projects, external link
 * intercept.
 *
 * Server state (settings, the workspace) reads through react-query; the
 * persisted theme is applied as a side effect of the settings query resolving.
 *
 * The workspace list follows other processes. A CLI run or an agent's MCP
 * server registering a project moves the workspace's operation log, the backend
 * watcher notices within a second and emits `workspace:changed`, and this
 * refetches. Nothing tells the app directly.
 *
 * Each project's context in numbers rides on its row of the workspace list,
 * so the home's standings follow the same query and the same events.
 */
export function useAppInit() {
  const qc = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: qk.settings(),
    queryFn: () => api.getSettings(),
  });
  const workspaceQuery = useQuery({
    queryKey: qk.workspaceProjects(),
    queryFn: () => api.listWorkspaceProjects(),
  });
  // Start here can be dismissed on the home and brought back from Settings,
  // so its override holds either answer until the settings refetch agrees.
  const [startHereOverride, setStartHereOverride] = useState<boolean | null>(null);

  const workspace = workspaceQuery.data ?? null;
  const workspaceError = workspaceQuery.error;
  // Hidden until the settings are read, so it never flashes up and away.
  const startHereDismissed = settingsQuery.data
    ? (startHereOverride ?? !!settingsQuery.data.start_here_dismissed)
    : true;
  const standings = useMemo(() => standingsOf(workspace), [workspace]);

  const refreshWorkspace = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
  }, [qc]);

  // Remove a project and the context the workspace holds for it. The caller
  // confirms first: this deletes the project's terms, voice profiles, content
  // memory and recorded decisions.
  const forgetProject = useCallback(
    async (key: string) => {
      try {
        await api.forgetWorkspaceProject(key);
      } finally {
        void qc.invalidateQueries({ queryKey: qk.workspaceProjects() });
      }
    },
    [qc],
  );

  // Apply the persisted theme once settings load.
  useEffect(() => {
    const s = settingsQuery.data;
    if (!s) return;
    const mode = s.theme || "system";
    if (mode === "system") {
      const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      document.documentElement.classList.toggle("dark", prefersDark);
    } else {
      document.documentElement.classList.toggle("dark", mode === "dark");
    }
  }, [settingsQuery.data]);

  // Per Wails v3 docs: common:ApplicationStarted fires after all
  // ServiceStartup hooks complete — data is guaranteed available.
  useInvalidateOnEvent("common:ApplicationStarted", [qk.workspaceProjects()]);
  // The backend's workspace watcher, and every registration this app makes.
  useInvalidateOnEvent("workspace:changed", [qk.workspaceProjects()]);

  // Intercept link clicks: an http, https or mailto link opens in the system
  // browser, and any other scheme is refused.
  useEffect(() => {
    const handler = (e: MouseEvent) => interceptLinkClick(e, openInSystemBrowser);
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, []);

  const setStartHere = useCallback(
    (dismissed: boolean) => {
      setStartHereOverride(dismissed);
      (dismissed ? api.dismissStartHere() : api.restoreStartHere())
        .catch(() => {})
        .finally(() => {
          void qc.invalidateQueries({ queryKey: qk.settings() });
        });
    },
    [qc],
  );
  const dismissStartHere = useCallback(() => setStartHere(true), [setStartHere]);
  const restoreStartHere = useCallback(() => setStartHere(false), [setStartHere]);

  return {
    workspace,
    workspaceError,
    standings,
    startHereDismissed,
    refreshWorkspace,
    forgetProject,
    dismissStartHere,
    restoreStartHere,
  };
}

/**
 * Every project's context in numbers, from the rows of the workspace list.
 * A row the backend could not read the standing of is left out. Null while
 * the workspace is being read.
 */
export function standingsOf(workspace: WorkspaceHome | null): ContextProjectStanding[] | null {
  if (!workspace) return null;
  const out: ContextProjectStanding[] = [];
  for (const row of workspace.projects) {
    if (!row.context) continue;
    out.push({ ...row.context, project_name: row.context.project_name || row.name });
  }
  return out;
}
