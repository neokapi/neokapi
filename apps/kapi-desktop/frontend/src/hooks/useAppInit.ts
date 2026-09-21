import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "./useInvalidateOnEvent";

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

  // Optimistic dismissal — flips immediately, before the settings refetch.
  const [dismissedOverride, setDismissedOverride] = useState(false);

  const workspace = workspaceQuery.data ?? null;
  const workspaceError = workspaceQuery.error;
  const samplesDismissed = settingsQuery.data
    ? dismissedOverride || !!settingsQuery.data.samples_dismissed
    : true;

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

  // Intercept external link clicks and open in the system browser.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const anchor = (e.target as HTMLElement).closest("a[href]") as HTMLAnchorElement | null;
      if (!anchor) return;
      const href = anchor.getAttribute("href");
      if (!href || href.startsWith("#") || href.startsWith("/")) return;
      e.preventDefault();
      import("@wailsio/runtime")
        .then((m) => m.Browser.OpenURL(href))
        .catch(() => {
          window.open(href, "_blank");
        });
    };
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, []);

  const dismissSamples = useCallback(() => {
    setDismissedOverride(true);
    api
      .dismissSamples()
      .catch(() => {})
      .finally(() => {
        void qc.invalidateQueries({ queryKey: qk.settings() });
      });
  }, [qc]);

  return {
    workspace,
    workspaceError,
    samplesDismissed,
    refreshWorkspace,
    forgetProject,
    dismissSamples,
  };
}
