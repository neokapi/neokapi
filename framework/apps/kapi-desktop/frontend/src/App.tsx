import { useState, useCallback, useEffect } from "react";
import type { AppSection, ProjectView, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { IconRail } from "./components/IconRail";
import { TabBar } from "./components/TabBar";
import { ProjectSidebar } from "./components/ProjectSidebar";
import { AppHome } from "./components/AppHome";
import { ProjectsPage } from "./components/ProjectsPage";
import { TermbasesPage } from "./components/TermbasesPage";
import { MemoriesPage } from "./components/MemoriesPage";
import { FormatsPage } from "./components/FormatsPage";
import { FlowPage } from "./components/FlowPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { SettingsPage } from "./components/SettingsPage";
import { HomePage } from "./components/HomePage";
import { ContentPage } from "./components/ContentPage";
import { useShortenHome } from "./hooks/useShortenHome";

interface TabState {
  info: TabInfo;
  project: KapiProject;
  view: ProjectView;
}

export default function App() {
  const [section, setSection] = useState<AppSection>("home");
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  const shortenHome = useShortenHome();

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  // Load recent files on mount and whenever tabs change.
  const refreshRecent = useCallback(() => {
    api.listRecentFiles().then((files) => {
      if (files) setRecentFiles(files);
    });
  }, []);

  useEffect(() => {
    refreshRecent();
  }, [refreshRecent, tabs.length]);

  // --- Tab management ---

  const addTab = useCallback(
    (tab: TabInfo, project: KapiProject) => {
      setTabs((prev) => {
        if (prev.some((t) => t.info.id === tab.id)) return prev;
        return [...prev, { info: tab, project, view: "project-home" }];
      });
      setActiveTabID(tab.id);
      setSection("projects");
    },
    [],
  );

  const handleNewProject = useCallback(
    async (name: string, savePath?: string) => {
      const tab = await api.newProject(name, "en-US", [], savePath);
      if (tab) {
        const proj = await api.getProject(tab.id);
        addTab(tab, proj ?? { version: "v1", name });
      }
      setShowNewProjectForm(false);
    },
    [addTab],
  );

  const handleOpenProject = useCallback(async () => {
    const tab = await api.openProjectDialog();
    if (tab) {
      const proj = await api.getProject(tab.id);
      if (proj) addTab(tab, proj);
    }
  }, [addTab]);

  const handleOpenRecent = useCallback(
    async (path: string) => {
      const tab = await api.openProject(path);
      if (tab) {
        const proj = await api.getProject(tab.id);
        if (proj) addTab(tab, proj);
      }
    },
    [addTab],
  );

  const handleCloseTab = useCallback(
    (tabID: string) => {
      api.closeProject(tabID);
      setTabs((prev) => {
        const remaining = prev.filter((t) => t.info.id !== tabID);
        setActiveTabID((cur) => {
          if (cur !== tabID) return cur;
          return remaining.length > 0
            ? remaining[remaining.length - 1].info.id
            : null;
        });
        return remaining;
      });
    },
    [],
  );

  const setProjectView = useCallback(
    (view: ProjectView) => {
      if (!activeTabID) return;
      setTabs((prev) =>
        prev.map((t) => (t.info.id === activeTabID ? { ...t, view } : t)),
      );
      if (view !== "project-flows") setSelectedFlow(null);
    },
    [activeTabID],
  );

  const updateActiveProject = useCallback(
    (project: KapiProject) => {
      if (!activeTabID) return;
      setTabs((prev) =>
        prev.map((t) => (t.info.id === activeTabID ? { ...t, project } : t)),
      );
    },
    [activeTabID],
  );

  const updateActiveTab = useCallback(
    (updated: TabInfo) => {
      setTabs((prev) =>
        prev.map((t) =>
          t.info.id === updated.id
            ? { ...t, info: updated, project: { ...t.project, name: updated.name } }
            : t,
        ),
      );
    },
    [],
  );

  // --- Menu events ---

  useEffect(() => {
    const cleanups: Array<() => void> = [];
    import("@wailsio/runtime")
      .then(({ Events }) => {
        cleanups.push(
          Events.On("menu:new-project", async () => {
            setSection("projects");
            setShowNewProjectForm(true);
          }),
        );
        cleanups.push(
          Events.On("menu:open-project", async () => {
            await handleOpenProject();
          }),
        );
        cleanups.push(
          Events.On("menu:open-recent", async (event: { data: unknown }) => {
            const path = event.data as string;
            if (path) await handleOpenRecent(path);
          }),
        );
        cleanups.push(
          Events.On("menu:save-project", async () => {
            if (!activeTabID) return;
            const path = await api.getProjectPath(activeTabID);
            if (path) {
              await api.saveProject(activeTabID);
            } else {
              const updated = await api.saveProjectDialog(activeTabID);
              if (updated) updateActiveTab(updated);
            }
          }),
        );
        cleanups.push(
          Events.On("menu:save-project-as", async () => {
            if (!activeTabID) return;
            const updated = await api.saveProjectDialog(activeTabID);
            if (updated) updateActiveTab(updated);
          }),
        );
        cleanups.push(
          Events.On("open-project-tab", async (event: { data: unknown }) => {
            const tab = event.data as TabInfo;
            if (tab?.id) {
              const proj = await api.getProject(tab.id);
              if (proj) addTab(tab, proj);
            }
          }),
        );
      })
      .catch(() => {});
    return () => cleanups.forEach((fn) => fn());
  }, [activeTabID, handleOpenProject, addTab, updateActiveTab]);

  // --- Section change ---

  const handleSectionChange = useCallback(
    (s: AppSection) => {
      if (s === "settings") {
        // Toggle settings overlay without changing section or project context.
        setShowSettings((v) => !v);
        return;
      }
      setShowSettings(false);
      setSection(s);
      if (s !== "projects") {
        setActiveTabID(null);
      } else if (tabs.length > 0 && !activeTabID) {
        setActiveTabID(tabs[0].info.id);
      }
    },
    [tabs, activeTabID],
  );

  // --- Render ---

  const isProjectActive = section === "projects" && activeTab !== null;

  return (
    <div className="flex h-screen bg-background text-foreground">
      {/* Icon rail — full height, extends into title bar */}
      <div className="flex shrink-0 flex-col bg-sidebar">
        {/* Traffic light spacer — no border here */}
        <div
          className="h-12 shrink-0"
          style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
        />
        {/* Rail with border starts below traffic lights */}
        <div className="flex-1 border-r border-border">
          <IconRail active={section} onChange={handleSectionChange} projectActive={isProjectActive} settingsActive={showSettings} />
        </div>
      </div>

      {/* Right side: tab bar + content */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Title bar / tab bar area — h-12 matches icon rail spacer so borders meet */}
        <div
          className="flex h-12 shrink-0 items-end border-b border-border bg-sidebar pl-16"
          style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
        >
          {section === "projects" && tabs.length > 0 ? (
            <div
              className="flex-1"
              style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            >
              <TabBar
                tabs={tabs.map((t) => t.info)}
                activeTabID={activeTabID}
                onSelect={(id) => {
                  setActiveTabID(id);
                  setSection("projects");
                }}
                onClose={handleCloseTab}
                onRename={(tabID, name) => {
                  setTabs((prev) =>
                    prev.map((t) =>
                      t.info.id === tabID
                        ? { ...t, info: { ...t.info, name }, project: { ...t.project, name } }
                        : t,
                    ),
                  );
                }}
              />
            </div>
          ) : (
            <div className="h-6" />
          )}
        </div>

        {/* Main layout */}
        <div className="flex flex-1 overflow-hidden">

        {/* Project secondary sidebar (only when project tab is active and settings is not open) */}
        {isProjectActive && activeTab && !showSettings && (
          <ProjectSidebar
            activeView={activeTab.view}
            onViewChange={setProjectView}
            projectName={activeTab.project.name}
            flowNames={Object.keys(activeTab.project.flows ?? {})}
            selectedFlow={selectedFlow}
            onSelectFlow={setSelectedFlow}
            onAddFlow={() => {
              const flows = activeTab.project.flows ?? {};
              let counter = Object.keys(flows).length + 1;
              let name = `flow-${counter}`;
              while (flows[name]) { counter++; name = `flow-${counter}`; }
              updateActiveProject({
                ...activeTab.project,
                flows: { ...flows, [name]: { steps: [{ tool: "pseudo-translate" }] } },
              });
              setSelectedFlow(name);
            }}
            onDeleteFlow={(name) => {
              const flows = { ...activeTab.project.flows };
              delete flows[name];
              updateActiveProject({ ...activeTab.project, flows });
              if (selectedFlow === name) setSelectedFlow(null);
            }}
          />
        )}

        {/* Main content */}
        <main className="flex-1 overflow-auto">
          {/* Settings overlay — replaces content when active */}
          {showSettings && <SettingsPage />}

          {/* App-level views (hidden when settings is open) */}
          {!showSettings && section === "home" && (
            <AppHome
              recentFiles={recentFiles}
              onOpenRecent={handleOpenRecent}
              onNewProject={() => {
                setSection("projects");
                setShowNewProjectForm(true);
              }}
              onNavigate={handleSectionChange}
            />
          )}

          {!showSettings && section === "projects" && !isProjectActive && (
            <ProjectsPage
              tabs={tabs.map((t) => t.info)}
              onSelectTab={(id) => setActiveTabID(id)}
              onNewProject={() => setShowNewProjectForm(true)}
              onOpenProject={handleOpenProject}
            />
          )}

          {!showSettings && section === "termbases" && <TermbasesPage />}
          {!showSettings && section === "memories" && <MemoriesPage />}

          {!showSettings && section === "flows" && (
            <div className="p-6">
              <h1 className="mb-4 text-xl font-semibold">Flows</h1>
              <p className="text-sm text-muted-foreground">
                Design and run flows on ad-hoc files without a project.
              </p>
            </div>
          )}

          {!showSettings && section === "tools" && <ToolRunnerPage />}
          {!showSettings && section === "formats" && <FormatsPage />}

          {/* Project-level views */}
          {!showSettings && isProjectActive && activeTab && (
            <>
              {activeTab.view === "project-home" && (
                <HomePage
                  project={activeTab.project}
                  onNavigate={(view) => setProjectView(view as ProjectView)}
                />
              )}
              {activeTab.view === "content" && (
                <ContentPage
                  project={activeTab.project}
                  projectPath={activeTab.info.path}
                  onUpdate={updateActiveProject}
                  tabID={activeTab.info.id}
                />
              )}
              {activeTab.view === "project-flows" && selectedFlow && activeTab.project.flows?.[selectedFlow] && (
                <FlowPage
                  flowName={selectedFlow}
                  flow={activeTab.project.flows[selectedFlow]}
                  onChange={(spec) => {
                    updateActiveProject({
                      ...activeTab.project,
                      flows: { ...activeTab.project.flows, [selectedFlow]: spec },
                    });
                  }}
                />
              )}
              {activeTab.view === "project-flows" && !selectedFlow && (
                <div className="flex h-full items-center justify-center text-muted-foreground">
                  <p className="text-sm">Select a flow or create a new one</p>
                </div>
              )}
              {activeTab.view === "project-tools" && <ToolRunnerPage />}
            </>
          )}
        </main>
        </div>
      </div>

      {/* New project modal */}
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

// --- New Project Dialog ---

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
  const INVALID_DIR_CHARS = /[<>:"/\\|?*\x00-\x1f]/;
  const valid = name.trim().length > 0 && !INVALID_DIR_CHARS.test(name) && name !== "." && name !== "..";
  const trimmedName = name.trim();
  const saveDir = customPath
    ? `${customPath}/${trimmedName}`
    : valid
      ? `~/KapiProjects/${trimmedName}`
      : "";

  const handleBrowse = async () => {
    const dir = await api.browseProjectLocation();
    if (dir) setCustomPath(shortenHome(dir));
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-4 text-lg font-semibold">New Project</h2>
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">Project Name</label>
            <div className="flex items-center gap-1.5">
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter" && valid) onCreate(trimmedName, saveDir ? `${saveDir}/project.kapi` : undefined); }}
                placeholder="My App"
                autoFocus
                className={`flex-1 rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring ${
                  name && !valid ? "border-destructive" : "border-input"
                }`}
              />
              <button
                onClick={handleBrowse}
                className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground"
                aria-label="Choose location"
                title="Choose location"
              >
                <FolderOpenIcon />
              </button>
            </div>
            {name && !valid ? (
              <p className="mt-1 text-xs text-destructive">Invalid directory name</p>
            ) : saveDir ? (
              <p className="mt-1 text-xs text-muted-foreground">Saved to {saveDir}</p>
            ) : null}
          </div>
          <div className="flex gap-2">
            <button
              onClick={() => onCreate(trimmedName, saveDir ? `${saveDir}/project.kapi` : undefined)}
              disabled={!valid}
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

function FolderOpenIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"/>
    </svg>
  );
}
