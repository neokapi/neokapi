import { useState, useCallback, useEffect, useRef } from "react";
import type { AppMode, KapiProject, TabInfo, PluginIssue } from "../types/api";
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

export function useTabManager() {
  const { showError } = useError();
  // Project-first: default to "projects" until the persisted app mode loads.
  const [mode, setMode] = useState<AppMode>("projects");
  const [globalView, setGlobalView] = useState("home");
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  // Gate session persistence until the initial restore completes so we don't
  // clobber the saved session with the empty startup state.
  const restoredRef = useRef(false);

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  // Global overlays don't change the tab's view — clicking a tab dismisses them.
  const isGlobalOverlay = globalView === "home" || globalView === "app-settings";
  const effectiveView = isGlobalOverlay
    ? globalView
    : mode === "projects" && activeTab
      ? activeTab.view
      : globalView;

  const navigate = useCallback(
    (v: string) => {
      if (v === "home" || v === "app-settings") {
        setGlobalView(v);
      } else if (mode === "projects" && activeTabID) {
        setGlobalView(""); // clear global overlay
        setTabs((prev) => prev.map((t) => (t.info.id === activeTabID ? { ...t, view: v } : t)));
      } else {
        setGlobalView(v);
      }
    },
    [mode, activeTabID],
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

  const addTab = useCallback(
    async (tab: TabInfo, project: KapiProject) => {
      const empty = await api.isEmptyProject(tab.id);
      let detected: string | undefined;
      if (!empty) {
        const preset = await api.detectPreset(tab.id);
        if (preset) detected = preset;
      }

      // Check if project plugin requirements are satisfied.
      const pluginStatus = await api.checkProjectPlugins(tab.id);

      setTabs((prev) => {
        if (prev.some((t) => t.info.id === tab.id)) return prev;
        return [
          ...prev,
          {
            info: tab,
            project,
            isEmpty: empty ?? false,
            detectedPreset: detected,
            pluginsResolved: pluginStatus?.satisfied ?? true,
            pluginIssues: pluginStatus?.issues,
            view: "project-home",
          },
        ];
      });
      setActiveTabID(tab.id);
      setMode("projects");
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

  const switchMode = useCallback(
    (m: AppMode) => {
      setMode(m);
      // Persist the chosen app mode so the next launch lands in the same place.
      void api.setAppMode(m);
      if (m === "adhoc") {
        setActiveTabID(null);
        setGlobalView("home");
      } else if (tabs.length > 0) {
        setActiveTabID(tabs[0].info.id);
      } else {
        setGlobalView("home");
      }
    },
    [tabs],
  );

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

  const openRecent = useCallback(
    async (path: string) => {
      try {
        const tab = await api.openProject(path);
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) await addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to open recent project", err);
      }
    },
    [addTab, showError],
  );

  const createProject = useCallback(
    async (name: string, savePath?: string) => {
      try {
        const tab = await api.newProject(name, "en-US", [], savePath);
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

  // --- Startup: read persisted app mode + session, reopen projects ---
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const savedMode = await api.getAppMode();
        const effectiveMode: AppMode = savedMode === "adhoc" ? "adhoc" : "projects";
        if (cancelled) return;
        setMode(effectiveMode);

        if (effectiveMode !== "projects") {
          setActiveTabID(null);
          setGlobalView("home");
          return;
        }

        const session = await api.getSessionState();
        const recipes = session?.lastOpenProjects ?? [];
        if (cancelled) return;

        if (recipes.length === 0) {
          // First-ever launch or no remembered projects → project-first home.
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
        // Fall back to project-first home on any failure.
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
    void api.saveSessionState({ mode, lastOpenProjects, activeProject });
  }, [tabs, activeTabID, mode]);

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

  return {
    mode,
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
    switchMode,
    openProject,
    openRecent,
    createProject,
    createSampleProject,
    checkPluginStatus,
  };
}
