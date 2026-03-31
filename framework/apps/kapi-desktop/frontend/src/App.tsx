import { useState, useCallback, useEffect, useRef } from "react";
import type { AppMode, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { useWailsEvent } from "./hooks/useWailsEvent";
import { ErrorProvider, useError } from "./components/ErrorBanner";
import { IconSidebar } from "./components/IconSidebar";
import { ModeToggle } from "./components/ModeToggle";
import { TabBar } from "./components/TabBar";
import { AppHome } from "./components/AppHome";
import { ProjectsPage } from "./components/ProjectsPage";
import { TermbasesPage } from "./components/TermbasesPage";
import { MemoriesPage } from "./components/MemoriesPage";
import { FormatsPage } from "./components/FormatsPage";
import { FlowPage } from "./components/FlowPage";
import { FlowsPage } from "./components/FlowsPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { SettingsPage } from "./components/SettingsPage";
import { HomePage } from "./components/HomePage";
import { ContentPage } from "./components/ContentPage";
import { ProjectSetupPage } from "./components/ProjectSetupPage";
import { useShortenHome } from "./hooks/useShortenHome";

interface TabState {
  info: TabInfo;
  project: KapiProject;
  isEmpty?: boolean;
}

export default function App() {
  return (
    <ErrorProvider>
      <AppInner />
    </ErrorProvider>
  );
}

function AppInner() {
  const { showError } = useError();
  const [mode, setMode] = useState<AppMode>("adhoc");
  const [view, setView] = useState<string>("home");
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  const [samplesDismissed, setSamplesDismissed] = useState(true); // default hidden until settings load
  const shortenHome = useShortenHome();

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  const refreshRecent = useCallback(() => {
    api.listRecentFiles().then((f) => {
      if (f) setRecentFiles(f);
    });
  }, []);

  // Refresh when tabs change.
  useEffect(() => {
    refreshRecent();
  }, [refreshRecent, tabs.length]);

  // Apply persisted theme and load settings on startup.
  useEffect(() => {
    api.getSettings().then((s) => {
      if (s) {
        setSamplesDismissed(!!s.samples_dismissed);
        const mode = s.theme || "system";
        if (mode === "system") {
          const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
          document.documentElement.classList.toggle("dark", prefersDark);
        } else {
          document.documentElement.classList.toggle("dark", mode === "dark");
        }
      }
    }).catch(() => {});
  }, []);

  // Per Wails v3 docs: common:ApplicationStarted fires after all
  // ServiceStartup hooks complete — data is guaranteed available.
  useWailsEvent("common:ApplicationStarted", () => refreshRecent());

  const handleModeChange = useCallback(
    (m: AppMode) => {
      setMode(m);
      if (m === "adhoc") {
        setActiveTabID(null);
        setView("home");
      } else {
        if (tabs.length > 0) {
          setActiveTabID(tabs[0].info.id);
          setView("project-home");
        } else {
          setView("home");
        }
      }
    },
    [tabs],
  );

  const handleViewChange = useCallback((v: string) => {
    setView(v);
    if (v !== "flows") setSelectedFlow(null);
  }, []);

  const addTab = useCallback(async (tab: TabInfo, project: KapiProject) => {
    const empty = await api.isEmptyProject(tab.id);
    setTabs((prev) => {
      if (prev.some((t) => t.info.id === tab.id)) return prev;
      return [...prev, { info: tab, project, isEmpty: empty ?? false }];
    });
    setActiveTabID(tab.id);
    setMode("projects");
    setView("project-home");
  }, []);

  const handleNewProject = useCallback(
    async (name: string, savePath?: string) => {
      try {
        const tab = await api.newProject(name, "en-US", [], savePath);
        if (tab) {
          const proj = await api.getProject(tab.id);
          addTab(tab, proj ?? { version: "v1", name: tab.name });
        }
        setShowNewProjectForm(false);
      } catch (err) {
        showError("Failed to create project", err);
      }
    },
    [addTab, showError],
  );

  const handleOpenProject = useCallback(async () => {
    try {
      const tab = await api.openProjectDialog();
      if (tab) {
        const proj = await api.getProject(tab.id);
        if (proj) addTab(tab, proj);
      }
    } catch (err) {
      showError("Failed to open project", err);
    }
  }, [addTab, showError]);

  const handleCreateSampleProject = useCallback(
    async (name: string) => {
      try {
        const tab = await api.createSampleProject(name);
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to create sample project", err);
      }
    },
    [addTab, showError],
  );

  const handleDismissSamples = useCallback(() => {
    setSamplesDismissed(true);
    api.dismissSamples().catch(() => {});
  }, []);

  const handleOpenRecent = useCallback(
    async (path: string) => {
      try {
        const tab = await api.openProject(path);
        if (tab) {
          const proj = await api.getProject(tab.id);
          if (proj) addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to open recent project", err);
      }
    },
    [addTab, showError],
  );

  const handleCloseTab = useCallback((tabID: string) => {
    api.closeProject(tabID);
    setTabs((prev) => {
      const remaining = prev.filter((t) => t.info.id !== tabID);
      setActiveTabID((cur) => {
        if (cur !== tabID) return cur;
        if (remaining.length > 0) return remaining[remaining.length - 1].info.id;
        setView("home"); // back to shared Home
        return null;
      });
      return remaining;
    });
  }, []);

  const updateActiveProject = useCallback(
    (project: KapiProject) => {
      if (!activeTabID) return;
      setTabs((prev) => prev.map((t) => (t.info.id === activeTabID ? { ...t, project } : t)));
    },
    [activeTabID],
  );

  const updateActiveTab = useCallback((updated: TabInfo) => {
    setTabs((prev) =>
      prev.map((t) =>
        t.info.id === updated.id
          ? { ...t, info: updated, project: { ...t.project, name: updated.name } }
          : t,
      ),
    );
  }, []);

  // Menu events — use refs for state that changes between renders
  // so the event callbacks always access the latest values.
  const activeTabIDRef = useRef(activeTabID);
  activeTabIDRef.current = activeTabID;

  useWailsEvent("menu:new-project", () => {
    setMode("projects");
    setShowNewProjectForm(true);
  });
  useWailsEvent("menu:open-project", () => handleOpenProject());
  useWailsEvent("menu:open-recent", (data) => {
    const p = data as string;
    if (p) handleOpenRecent(p);
  });
  useWailsEvent("menu:save-project", async () => {
    const tabID = activeTabIDRef.current;
    if (!tabID) return;
    try {
      const path = await api.getProjectPath(tabID);
      if (path) await api.saveProject(tabID);
      else {
        const u = await api.saveProjectDialog(tabID);
        if (u) updateActiveTab(u);
      }
    } catch (err) {
      showError("Failed to save project", err);
    }
  });
  useWailsEvent("menu:save-project-as", async () => {
    const tabID = activeTabIDRef.current;
    if (!tabID) return;
    try {
      const u = await api.saveProjectDialog(tabID);
      if (u) updateActiveTab(u);
    } catch (err) {
      showError("Failed to save project", err);
    }
  });
  useWailsEvent("open-project-tab", async (data) => {
    const tab = data as TabInfo;
    if (tab?.id) {
      const proj = await api.getProject(tab.id);
      if (proj) addTab(tab, proj);
    }
  });

  return (
    <div className="flex h-screen bg-background text-foreground">
      {/* Icon sidebar — full height */}
      <div className="flex shrink-0 flex-col bg-sidebar">
        <div className="h-12 shrink-0" style={{ WebkitAppRegion: "drag" } as React.CSSProperties} />
        <div className="flex-1 border-r border-border">
          <IconSidebar
            mode={mode}
            active={view}
            onChange={handleViewChange}
            projectDisabled={mode === "projects" && !activeTab}
          />
        </div>
      </div>

      {/* Right: top bar + content */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar: [traffic lights space] [tabs] ... [mode toggle] */}
        <div
          className="flex h-12 shrink-0 items-end border-b border-border bg-sidebar"
          style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
        >
          {/* Tabs or spacer */}
          <div
            className="flex-1 pl-16"
            style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
          >
            {mode === "projects" && tabs.length > 0 && (
              <TabBar
                tabs={tabs.map((t) => t.info)}
                activeTabID={activeTabID}
                onSelect={setActiveTabID}
                onClose={handleCloseTab}
                onRename={(id, name) => {
                  setTabs((prev) =>
                    prev.map((t) =>
                      t.info.id === id
                        ? { ...t, info: { ...t.info, name }, project: { ...t.project, name } }
                        : t,
                    ),
                  );
                }}
              />
            )}
          </div>
          {/* Mode toggle */}
          <div
            className="shrink-0 px-3 pb-1.5"
            style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
          >
            <ModeToggle mode={mode} onChange={handleModeChange} />
          </div>
        </div>

        {/* Content */}
        <main className="flex-1 overflow-auto">
          {/* Home — always the global home page in both modes */}
          {view === "home" && (
            <AppHome
              recentFiles={recentFiles}
              samplesDismissed={samplesDismissed}
              onOpenRecent={handleOpenRecent}
              onNewProject={() => {
                setMode("projects");
                setShowNewProjectForm(true);
              }}
              onOpenProject={handleOpenProject}
              onNavigate={handleViewChange}
              onCreateSampleProject={handleCreateSampleProject}
              onDismissSamples={handleDismissSamples}
            />
          )}

          {/* Ad-hoc views */}
          {mode === "adhoc" && view === "flows" && (
            <FlowsPage />
          )}
          {mode === "adhoc" && view === "tools" && <ToolRunnerPage />}
          {mode === "adhoc" && view === "termbases" && <TermbasesPage />}
          {mode === "adhoc" && view === "memories" && <MemoriesPage />}
          {mode === "adhoc" && view === "formats" && <FormatsPage />}

          {/* Project views (only when a project tab is active) */}
          {mode === "projects" && activeTab && view === "project-home" && activeTab.isEmpty && (
            <ProjectSetupPage
              tabID={activeTab.info.id}
              onDone={() => {
                setTabs((prev) =>
                  prev.map((t) =>
                    t.info.id === activeTab.info.id ? { ...t, isEmpty: false } : t,
                  ),
                );
              }}
            />
          )}
          {mode === "projects" && activeTab && view === "project-home" && !activeTab.isEmpty && (
            <HomePage
              project={activeTab.project}
              displayName={activeTab.info.name}
              onNavigate={handleViewChange}
            />
          )}
          {mode === "projects" && activeTab && view === "content" && (
            <ContentPage
              project={activeTab.project}
              projectPath={activeTab.info.path}
              onUpdate={updateActiveProject}
              tabID={activeTab.info.id}
            />
          )}
          {mode === "projects" && activeTab && view === "flows" && (
            <FlowsPage
              tabID={activeTab.info.id}
              projectFlows={activeTab.project.flows}
              onFlowChange={(name, spec) => {
                updateActiveProject({
                  ...activeTab.project,
                  flows: { ...activeTab.project.flows, [name]: spec },
                });
              }}
              onFlowDelete={(name) => {
                const { [name]: _, ...rest } = activeTab.project.flows ?? {};
                updateActiveProject({ ...activeTab.project, flows: rest });
              }}
            />
          )}
          {mode === "projects" && activeTab && view === "tools" && <ToolRunnerPage />}
          {mode === "projects" && activeTab && view === "termbases" && <TermbasesPage />}
          {mode === "projects" && activeTab && view === "memories" && <MemoriesPage />}

          {view === "settings" && <SettingsPage />}
        </main>
      </div>

      {showNewProjectForm && (
        <NewProjectDialog
          onCreate={handleNewProject}
          onCancel={() => setShowNewProjectForm(false)}
          shortenHome={shortenHome}
        />
      )}
    </div>
  );
}

function NewProjectDialog({
  onCreate,
  onCancel,
  shortenHome,
}: {
  onCreate: (name: string, savePath?: string) => void;
  onCancel: () => void;
  shortenHome: (path: string) => string;
}) {
  const [name, setName] = useState("");
  const [customPath, setCustomPath] = useState("");
  const INVALID = /[<>:"/\\|?*\x00-\x1f]/;
  const trimmed = name.trim();
  const nameValid =
    trimmed.length > 0 && !INVALID.test(trimmed) && trimmed !== "." && trimmed !== "..";
  // When a folder is browsed, use it directly. Otherwise require a name for ~/KapiProjects/{name}.
  const canCreate = customPath ? true : nameValid;
  const saveDir = customPath ? customPath : nameValid ? `~/KapiProjects/${trimmed}` : "";
  const handleBrowse = async () => {
    const dir = await api.browseProjectLocation();
    if (dir) setCustomPath(shortenHome(dir));
  };
  const handleCreate = () => {
    if (canCreate) onCreate(trimmed, saveDir ? `${saveDir}/project.kapi` : undefined);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-4 text-lg font-semibold">New Project</h2>
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">
              {customPath ? "Location" : "Name"}
            </label>
            <div className="flex items-center gap-1.5">
              <input
                type="text"
                value={customPath || name}
                onChange={(e) => {
                  if (customPath) return;
                  setName(e.target.value);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCreate();
                }}
                placeholder={customPath ? "" : "My App"}
                readOnly={!!customPath}
                autoFocus={!customPath}
                className={`flex-1 rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring ${name && !nameValid && !customPath ? "border-destructive" : "border-input"} ${customPath ? "text-muted-foreground" : ""}`}
              />
              <button
                onClick={handleBrowse}
                className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label="Choose location"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2" />
                </svg>
              </button>
              {customPath && (
                <button
                  onClick={() => setCustomPath("")}
                  className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                  aria-label="Clear location"
                >
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <path d="M18 6 6 18" />
                    <path d="m6 6 12 12" />
                  </svg>
                </button>
              )}
            </div>
            {customPath ? (
              <p className="mt-1 text-xs">&nbsp;</p>
            ) : name && !nameValid ? (
              <p className="mt-1 text-xs text-destructive">Invalid directory name</p>
            ) : saveDir ? (
              <p className="mt-1 text-xs text-muted-foreground">{saveDir}</p>
            ) : (
              <p className="mt-1 text-xs">&nbsp;</p>
            )}
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleCreate}
              disabled={!canCreate}
              className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              Create Project
            </button>
            <button
              onClick={onCancel}
              className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
