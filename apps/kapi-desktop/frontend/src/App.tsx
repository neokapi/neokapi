import { useState, useCallback, useEffect, useRef } from "react";
import type { AppMode, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { useWailsEvent } from "./hooks/useWailsEvent";
import { ErrorProvider, useError } from "./components/ErrorBanner";
import { IconSidebar } from "./components/IconSidebar";
import { ModeToggle } from "./components/ModeToggle";
import { TabBar } from "./components/TabBar";
import { AppHome } from "./components/AppHome";

import { TermbasesPage } from "./components/TermbasesPage";
import { MemoriesPage } from "./components/MemoriesPage";
import { FormatsPage } from "./components/FormatsPage";

import { FlowsPage } from "./components/FlowsPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { SettingsPage } from "./components/SettingsPage";
import { HomePage } from "./components/HomePage";
import { ContentPage } from "./components/ContentPage";
import { ProjectSetupPage } from "./components/ProjectSetupPage";
import { ProjectSettingsPage } from "./components/ProjectSettingsPage";
import { ProjectPresetPage } from "./components/ProjectPresetPage";
import { useShortenHome } from "./hooks/useShortenHome";
import { Button, Label, Input } from "@neokapi/ui-primitives";

interface TabState {
  info: TabInfo;
  project: KapiProject;
  isEmpty?: boolean;
  detectedPreset?: string;
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
  const [, setSelectedFlow] = useState<string | null>(null);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  const [samplesDismissed, setSamplesDismissed] = useState(true); // default hidden until settings load
  const shortenHome = useShortenHome();

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  const refreshRecent = useCallback(() => {
    void api.listRecentFiles().then((f) => {
      if (f) setRecentFiles(f);
    });
  }, []);

  // Refresh when tabs change.
  useEffect(() => {
    refreshRecent();
  }, [refreshRecent, tabs.length]);

  // Apply persisted theme and load settings on startup.
  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
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
      })
      .catch(() => {});
  }, []);

  // Per Wails v3 docs: common:ApplicationStarted fires after all
  // ServiceStartup hooks complete — data is guaranteed available.
  useWailsEvent("common:ApplicationStarted", () => refreshRecent());

  // Intercept external link clicks and open in the system browser
  // instead of navigating within the Wails webview.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const anchor = (e.target as HTMLElement).closest("a[href]") as HTMLAnchorElement | null;
      if (!anchor) return;
      const href = anchor.getAttribute("href");
      if (!href || href.startsWith("#") || href.startsWith("/")) return;
      // External URL — open in system browser.
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
    // For non-empty folders, detect if a framework preset matches.
    let detected: string | undefined;
    if (!empty) {
      const preset = await api.detectPreset(tab.id);
      if (preset) detected = preset;
    }
    setTabs((prev) => {
      if (prev.some((t) => t.info.id === tab.id)) return prev;
      return [...prev, { info: tab, project, isEmpty: empty ?? false, detectedPreset: detected }];
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
          await addTab(tab, proj ?? { version: "v1", name: tab.name });
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
        if (proj) await addTab(tab, proj);
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
          if (proj) await addTab(tab, proj);
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
          if (proj) await addTab(tab, proj);
        }
      } catch (err) {
        showError("Failed to open recent project", err);
      }
    },
    [addTab, showError],
  );

  const handleCloseTab = useCallback((tabID: string) => {
    void api.closeProject(tabID);
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
    if (p) void handleOpenRecent(p);
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
      if (proj) await addTab(tab, proj);
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
          {mode === "adhoc" && view === "flows" && <FlowsPage />}
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
                    t.info.id === activeTab.info.id
                      ? { ...t, isEmpty: false, detectedPreset: undefined }
                      : t,
                  ),
                );
              }}
            />
          )}
          {mode === "projects" &&
            activeTab &&
            view === "project-home" &&
            !activeTab.isEmpty &&
            activeTab.detectedPreset && (
              <ProjectPresetPage
                tabID={activeTab.info.id}
                detectedPreset={activeTab.detectedPreset}
                onApplied={(updated) => {
                  updateActiveProject(updated);
                  setTabs((prev) =>
                    prev.map((t) =>
                      t.info.id === activeTab.info.id ? { ...t, detectedPreset: undefined } : t,
                    ),
                  );
                }}
                onSkip={() => {
                  setTabs((prev) =>
                    prev.map((t) =>
                      t.info.id === activeTab.info.id ? { ...t, detectedPreset: undefined } : t,
                    ),
                  );
                }}
              />
            )}
          {mode === "projects" &&
            activeTab &&
            view === "project-home" &&
            !activeTab.isEmpty &&
            !activeTab.detectedPreset && (
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
          {mode === "projects" && activeTab && view === "settings" && (
            <ProjectSettingsPage
              project={activeTab.project}
              onUpdate={updateActiveProject}
              tabID={activeTab.info.id}
            />
          )}

          {view === "settings" && !(mode === "projects" && activeTab) && <SettingsPage />}
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
  // eslint-disable-next-line no-control-regex -- intentional check for control characters in filenames
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
            <Label className="mb-1 block text-xs text-muted-foreground">
              {customPath ? "Location" : "Name"}
            </Label>
            <div className="flex items-center gap-1.5">
              <Input
                type="text"
                value={customPath || name}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                  if (customPath) return;
                  setName(e.target.value);
                }}
                onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
                  if (e.key === "Enter") handleCreate();
                }}
                placeholder={customPath ? "" : "My App"}
                readOnly={!!customPath}
                autoFocus={!customPath}
                className={`flex-1 ${name && !nameValid && !customPath ? "border-destructive" : ""} ${customPath ? "text-muted-foreground" : ""}`}
              />
              <Button
                variant="outline"
                size="icon-sm"
                onClick={handleBrowse}
                className="shrink-0"
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
              </Button>
              {customPath && (
                <Button
                  variant="outline"
                  size="icon-sm"
                  onClick={() => setCustomPath("")}
                  className="shrink-0"
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
                </Button>
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
            <Button onClick={handleCreate} disabled={!canCreate} className="flex-1">
              Create Project
            </Button>
            <Button variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
