import { useState, useCallback, useEffect, useRef } from "react";
import type { KapiProject, TabInfo, PluginIssue } from "../types/api";
import { api } from "./useApi";
import { useError } from "../components/ErrorBanner";
import { useWailsEvent } from "./useWailsEvent";

export interface TabState {
  info: TabInfo;
  project: KapiProject;
  isEmpty?: boolean;
  detectedPreset?: string;
  /** Whether all project plugin requirements are satisfied by installed plugins. */
  pluginsResolved?: boolean;
  /** Details of unsatisfied plugin requirements (missing or version mismatch). */
  pluginIssues?: PluginIssue[];
  /** Per-tab view (content, flows, settings, etc.). */
  view: string;
}

/**
 * Views that sit over every tab rather than inside one: the home, the
 * workspace's history, and the app's settings. Clicking a tab dismisses them.
 */
export const GLOBAL_OVERLAYS: ReadonlySet<string> = new Set(["home", "history", "app-settings"]);

/**
 * Where a newly opened project tab lands: its Map, whatever state the project
 * is in. The Map's own empty state is a new project's first screen, and the
 * page that sets a project up stays on the project home.
 */
export function landingView(): string {
  return "map";
}

export function useTabManager() {
  const { showError } = useError();
  const [globalView, setGlobalView] = useState("home");
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  // Gate session persistence until the initial restore completes so we don't
  // clobber the saved session with the empty startup state.
  const restoredRef = useRef(false);

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  // Global overlays don't change the tab's view — clicking a tab dismisses them.
  const isGlobalOverlay = GLOBAL_OVERLAYS.has(globalView);
  const effectiveView = isGlobalOverlay ? globalView : activeTab ? activeTab.view : globalView;

  const navigate = useCallback(
    (v: string) => {
      // `content` folds into the merged project home (issue #1068) — normalize
      // the alias so the Project sidebar item highlights and the tab's stored
      // view stays canonical.
      const view = v === "content" ? "project-home" : v;
      if (GLOBAL_OVERLAYS.has(view)) {
        setGlobalView(view);
      } else if (activeTabID) {
        setGlobalView(""); // clear global overlay
        setTabs((prev) => prev.map((t) => (t.info.id === activeTabID ? { ...t, view } : t)));
      } else {
        setGlobalView(view);
      }
    },
    [activeTabID],
  );

  const selectTab = useCallback((id: string) => {
    setActiveTabID(id);
    setGlobalView(""); // clear global overlay
  }, []);

  const checkPluginStatus = useCallback(async (tabID: string) => {
    const status = await api.checkProjectPlugins(tabID);
    if (status) {
      setTabs((prev) =>
        prev.map((t) =>
          t.info.id === tabID
            ? { ...t, pluginsResolved: status.satisfied, pluginIssues: status.issues }
            : t,
        ),
      );
    }
  }, []);

  // `view` opens the tab on a view of the caller's choosing, a tab already
  // open included; absent, a new tab lands where landingView says.
  const addTab = useCallback(
    async (tab: TabInfo, project: KapiProject, view?: string) => {
      const reopen = (prev: TabState[]) =>
        view ? prev.map((t) => (t.info.id === tab.id ? { ...t, view } : t)) : prev;
      // A context-only tab carries no recipe and no files, so the questions
      // asked of a checkout (is it empty, which preset, are its plugins
      // installed) have no subject. It opens straight on the Context hub.
      if (tab.context_only) {
        setTabs((prev) => {
          if (prev.some((t) => t.info.id === tab.id)) return reopen(prev);
          return [
            ...prev,
            {
              info: tab,
              project,
              isEmpty: false,
              pluginsResolved: true,
              view: view ?? "context",
            },
          ];
        });
        setActiveTabID(tab.id);
        setGlobalView("");
        return;
      }
      const empty = await api.isEmptyProject(tab.id);
      let detected: string | undefined;
      if (!empty) {
        const preset = await api.detectPreset(tab.id);
        if (preset) detected = preset;
      }

      // Check if project plugin requirements are satisfied.
      const pluginStatus = await api.checkProjectPlugins(tab.id);

      setTabs((prev) => {
        if (prev.some((t) => t.info.id === tab.id)) return reopen(prev);
        return [
          ...prev,
          {
            info: tab,
            project,
            isEmpty: empty ?? false,
            detectedPreset: detected,
            pluginsResolved: pluginStatus?.satisfied ?? true,
            pluginIssues: pluginStatus?.issues,
            view: view ?? landingView(),
          },
        ];
      });
      setActiveTabID(tab.id);
      setGlobalView(""); // clear home overlay so tab's view shows
    },
    [checkPluginStatus],
  );

  const closeTab = useCallback((tabID: string) => {
    void api.closeProject(tabID);
    setTabs((prev) => {
      const remaining = prev.filter((t) => t.info.id !== tabID);
      setActiveTabID((cur) => {
        if (cur !== tabID) return cur;
        if (remaining.length > 0) return remaining[remaining.length - 1].info.id;
        setGlobalView("home");
        return null;
      });
      return remaining;
    });
  }, []);

  // Bring an open tab forward on one view, e.g. a project's Assistant section.
  const openTabOn = useCallback((id: string, view: string) => {
    setTabs((prev) => prev.map((t) => (t.info.id === id ? { ...t, view } : t)));
    setActiveTabID(id);
    setGlobalView("");
  }, []);

  const updateTab = useCallback((id: string, patch: Partial<TabState>) => {
    setTabs((prev) => prev.map((t) => (t.info.id === id ? { ...t, ...patch } : t)));
  }, []);

  const updateTabInfo = useCallback((updated: TabInfo) => {
    setTabs((prev) =>
      prev.map((t) =>
        t.info.id === updated.id
          ? { ...t, info: updated, project: { ...t.project, name: updated.name } }
          : t,
      ),
    );
  }, []);

  const openProject = useCallback(async () => {
    try {
      const tab = await api.openProjectDialog();
      if (tab) {
        const proj = await api.getProject(tab.id);
        if (proj) await addTab(tab, proj);
      }
    } catch (err) {
      showError("Failed to open project", err);
    }
  }, [addTab, showError]);

  // Open a project from one of its checkouts, by recipe path. The workspace
  // home and the native Projects menu both land here.
  const openRecent = useCallback(
    async (path: string, view?: string) => {
      try {
        const tab = await api.openProject(path);
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) await addTab(tab, proj, view);
        }
      } catch (err) {
        showError("Failed to open the project", err);
      }
    },
    [addTab, showError],
  );

  // Open a project the workspace holds and no checkout on this machine
  // carries. The tab reads its context and has no files to show.
  const openWorkspaceContext = useCallback(
    async (key: string, view?: string) => {
      try {
        const tab = await api.openWorkspaceContext(key);
        if (tab) await addTab(tab, { version: "v1", name: tab.name }, view);
      } catch (err) {
        showError("Failed to open the project's context", err);
      }
    },
    [addTab, showError],
  );

  const createProject = useCallback(
    async (name: string, sourceLang = "en-US", savePath?: string) => {
      try {
        const tab = await api.newProject(name, sourceLang, [], savePath);
        if (tab) {
          const proj = await api.getProject(tab.id);
          await addTab(tab, proj ?? { version: "v1", name: tab.name });
        }
        setShowNewProjectForm(false);
      } catch (err) {
        showError("Failed to create project", err);
      }
    },
    [addTab, showError],
  );

  const createSampleProject = useCallback(
    async (name: string) => {
      try {
        const tab = await api.createSampleProject(name);
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) await addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to create sample project", err);
      }
    },
    [addTab, showError],
  );

  // Refresh an out-of-date sample to the version bundled with this kapi. The
  // backend backs up the old copy, re-scaffolds, and returns a NEW tab (the
  // project is reopened), so swap the old tab for the new one in local state.
  const resetSample = useCallback(
    async (tabID: string) => {
      try {
        const tab = await api.resetSampleProject(tabID);
        setTabs((prev) => prev.filter((t) => t.info.id !== tabID));
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) await addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to reset sample", err);
      }
    },
    [addTab, showError],
  );

  // --- Startup: read the persisted session, reopen projects ---
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const session = await api.getSessionState();
        const recipes = session?.lastOpenProjects ?? [];
        if (cancelled) return;

        if (recipes.length === 0) {
          // First-ever launch or no remembered projects → the Open screen.
          setGlobalView("home");
          return;
        }

        // Reopen remembered projects via the normal OpenProject path. Map each
        // recipe path to its opened tab id so we can restore the active tab.
        // recipes are most-recent first; open oldest-first so the resulting tab
        // order (append) matches and the session round-trips on re-persist.
        const openedByPath = new Map<string, string>();
        for (const path of [...recipes].reverse()) {
          try {
            const tab = await api.openProject(path);
            if (!tab) continue;
            const proj = await api.getProject(tab.id);
            if (cancelled) return;
            await addTab(tab, proj ?? { version: "v1", name: tab.name });
            openedByPath.set(path, tab.id);
          } catch {
            // Skip projects that no longer open (moved/deleted recipe).
          }
        }

        if (cancelled) return;
        const activeID = session?.activeProject
          ? openedByPath.get(session.activeProject)
          : undefined;
        if (activeID) {
          setActiveTabID(activeID);
          setGlobalView("");
        } else if (openedByPath.size === 0) {
          // Nothing reopened — land on the home screen.
          setGlobalView("home");
        }
      } catch {
        // Fall back to the home on any failure.
        if (!cancelled) setGlobalView("home");
      } finally {
        if (!cancelled) restoredRef.current = true;
      }
    })();
    return () => {
      cancelled = true;
    };
    // Run once on mount; addTab is stable enough for this one-shot restore.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- Persist the open-project set + active tab whenever they change ---
  // lastOpenProjects is most-recent first: tabs are appended in open order, so
  // the newest tab is last — reverse to put it first.
  const persistSession = useCallback(() => {
    if (!restoredRef.current) return;
    const lastOpenProjects = tabs
      .map((t) => t.info.path)
      .filter((p): p is string => !!p)
      .reverse();
    const activeProject = tabs.find((t) => t.info.id === activeTabID)?.info.path ?? "";
    void api.saveSessionState({ lastOpenProjects, activeProject });
  }, [tabs, activeTabID]);

  useEffect(() => {
    persistSession();
  }, [persistSession]);

  // --- Best-effort persist on shutdown ---
  useEffect(() => {
    window.addEventListener("beforeunload", persistSession);
    return () => window.removeEventListener("beforeunload", persistSession);
  }, [persistSession]);

  // Re-check all open tabs when plugins change (install/remove/update).
  useWailsEvent("plugins-changed", () => {
    for (const tab of tabs) {
      void checkPluginStatus(tab.info.id);
    }
  });

  // A folder opened without a recipe gets one the first time a person
  // changes it; the tab follows.
  useWailsEvent("project:recipe-written", (data) => {
    const tabID = (data as { tabID?: string } | null)?.tabID;
    if (!tabID) return;
    setTabs((prev) =>
      prev.map((t) =>
        t.info.id === tabID ? { ...t, info: { ...t.info, recipe_written: true } } : t,
      ),
    );
  });

  return {
    globalView,
    tabs,
    activeTabID,
    activeTab,
    effectiveView,
    showNewProjectForm,
    setShowNewProjectForm,
    navigate,
    selectTab,
    addTab,
    closeTab,
    updateTab,
    updateTabInfo,
    openTabOn,
    openProject,
    openRecent,
    openWorkspaceContext,
    createProject,
    createSampleProject,
    resetSample,
    checkPluginStatus,
  };
}
