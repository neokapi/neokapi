import { useState, useCallback, useEffect } from "react";
import type { AppMode, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { IconSidebar } from "./components/IconSidebar";
import { ModeToggle } from "./components/ModeToggle";
import { TabBar } from "./components/TabBar";
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
}

export default function App() {
  const [mode, setMode] = useState<AppMode>("adhoc");
  const [view, setView] = useState<string>("home");
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);
  const [showNewProjectForm, setShowNewProjectForm] = useState(false);
  const shortenHome = useShortenHome();

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  const refreshRecent = useCallback(() => {
    api.listRecentFiles().then((f) => { if (f) setRecentFiles(f); });
  }, []);
  useEffect(() => { refreshRecent(); }, [refreshRecent, tabs.length]);

  const handleModeChange = useCallback((m: AppMode) => {
    setMode(m);
    setView("home");
    if (m === "adhoc") setActiveTabID(null);
    else if (tabs.length > 0) setActiveTabID(tabs[0].info.id);
  }, [tabs]);

  const handleViewChange = useCallback((v: string) => {
    setView(v);
    if (v !== "flows") setSelectedFlow(null);
  }, []);

  const addTab = useCallback((tab: TabInfo, project: KapiProject) => {
    setTabs((prev) => {
      if (prev.some((t) => t.info.id === tab.id)) return prev;
      return [...prev, { info: tab, project }];
    });
    setActiveTabID(tab.id);
    setMode("projects");
    setView("home");
  }, []);

  const handleNewProject = useCallback(async (name: string, savePath?: string) => {
    const tab = await api.newProject(name, "en-US", [], savePath);
    if (tab) {
      const proj = await api.getProject(tab.id);
      addTab(tab, proj ?? { version: "v1", name });
    }
    setShowNewProjectForm(false);
  }, [addTab]);

  const handleOpenProject = useCallback(async () => {
    const tab = await api.openProjectDialog();
    if (tab) { const proj = await api.getProject(tab.id); if (proj) addTab(tab, proj); }
  }, [addTab]);

  const handleOpenRecent = useCallback(async (path: string) => {
    const tab = await api.openProject(path);
    if (tab) { const proj = await api.getProject(tab.id); if (proj) addTab(tab, proj); }
  }, [addTab]);

  const handleCloseTab = useCallback((tabID: string) => {
    api.closeProject(tabID);
    setTabs((prev) => {
      const remaining = prev.filter((t) => t.info.id !== tabID);
      setActiveTabID((cur) => {
        if (cur !== tabID) return cur;
        if (remaining.length > 0) return remaining[remaining.length - 1].info.id;
        setMode("adhoc"); setView("home");
        return null;
      });
      return remaining;
    });
  }, []);

  const updateActiveProject = useCallback((project: KapiProject) => {
    if (!activeTabID) return;
    setTabs((prev) => prev.map((t) => t.info.id === activeTabID ? { ...t, project } : t));
  }, [activeTabID]);

  const updateActiveTab = useCallback((updated: TabInfo) => {
    setTabs((prev) => prev.map((t) =>
      t.info.id === updated.id ? { ...t, info: updated, project: { ...t.project, name: updated.name } } : t
    ));
  }, []);

  // Menu events
  useEffect(() => {
    const cleanups: Array<() => void> = [];
    import("@wailsio/runtime").then(({ Events }) => {
      cleanups.push(Events.On("menu:new-project", () => { setMode("projects"); setShowNewProjectForm(true); }));
      cleanups.push(Events.On("menu:open-project", () => handleOpenProject()));
      cleanups.push(Events.On("menu:open-recent", (e: { data: unknown }) => { const p = e.data as string; if (p) handleOpenRecent(p); }));
      cleanups.push(Events.On("menu:save-project", async () => {
        if (!activeTabID) return;
        const path = await api.getProjectPath(activeTabID);
        if (path) await api.saveProject(activeTabID);
        else { const u = await api.saveProjectDialog(activeTabID); if (u) updateActiveTab(u); }
      }));
      cleanups.push(Events.On("menu:save-project-as", async () => {
        if (!activeTabID) return;
        const u = await api.saveProjectDialog(activeTabID); if (u) updateActiveTab(u);
      }));
      cleanups.push(Events.On("open-project-tab", async (e: { data: unknown }) => {
        const tab = e.data as TabInfo;
        if (tab?.id) { const proj = await api.getProject(tab.id); if (proj) addTab(tab, proj); }
      }));
    }).catch(() => {});
    return () => cleanups.forEach((fn) => fn());
  }, [activeTabID, handleOpenProject, handleOpenRecent, addTab, updateActiveTab]);

  return (
    <div className="flex h-screen bg-background text-foreground">
      {/* Icon sidebar — full height */}
      <div className="flex shrink-0 flex-col bg-sidebar">
        <div className="h-12 shrink-0" style={{ WebkitAppRegion: "drag" } as React.CSSProperties} />
        <div className="flex-1 border-r border-border">
          <IconSidebar mode={mode} active={view} onChange={handleViewChange} projectDisabled={mode === "projects" && !activeTab} />
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
          <div className="flex-1 pl-16" style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
            {mode === "projects" && (
              <TabBar
                tabs={[
                  { id: "__home__", name: "Home", path: "" },
                  ...tabs.map((t) => t.info),
                ]}
                activeTabID={activeTabID ?? "__home__"}
                onSelect={(id) => {
                  if (id === "__home__") {
                    setActiveTabID(null);
                    setView("home");
                  } else {
                    setActiveTabID(id);
                    if (view === "home") setView("home"); // stay on project home
                  }
                }}
                onClose={(id) => {
                  if (id === "__home__") return; // can't close Home
                  handleCloseTab(id);
                }}
                onRename={(id, name) => {
                  if (id === "__home__") return;
                  setTabs((prev) => prev.map((t) =>
                    t.info.id === id ? { ...t, info: { ...t.info, name }, project: { ...t.project, name } } : t
                  ));
                }}
              />
            )}
          </div>
          {/* Mode toggle */}
          <div className="shrink-0 px-3 pb-1.5" style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
            <ModeToggle mode={mode} onChange={handleModeChange} />
          </div>
        </div>

        {/* Content */}
        <main className="flex-1 overflow-auto">
          {mode === "adhoc" && view === "home" && (
            <AppHome recentFiles={recentFiles} onOpenRecent={handleOpenRecent}
              onNewProject={() => { setMode("projects"); setShowNewProjectForm(true); }}
              onNavigate={handleViewChange} />
          )}
          {mode === "adhoc" && view === "flows" && (
            <div className="p-6">
              <h1 className="mb-4 text-xl font-semibold">Flows</h1>
              <p className="text-sm text-muted-foreground">Design and run flows on ad-hoc files.</p>
            </div>
          )}
          {mode === "adhoc" && view === "tools" && <ToolRunnerPage />}
          {mode === "adhoc" && view === "termbases" && <TermbasesPage />}
          {mode === "adhoc" && view === "memories" && <MemoriesPage />}
          {mode === "adhoc" && view === "formats" && <FormatsPage />}

          {mode === "projects" && !activeTab && (
            <ProjectsPage tabs={tabs.map((t) => t.info)} onSelectTab={setActiveTabID}
              onNewProject={() => setShowNewProjectForm(true)} onOpenProject={handleOpenProject} />
          )}
          {mode === "projects" && view === "home" && activeTab && (
            <HomePage project={activeTab.project} onNavigate={handleViewChange} />
          )}
          {mode === "projects" && view === "content" && activeTab && (
            <ContentPage project={activeTab.project} projectPath={activeTab.info.path}
              onUpdate={updateActiveProject} tabID={activeTab.info.id} />
          )}
          {mode === "projects" && view === "flows" && activeTab && (
            <div className="p-6">
              <h1 className="mb-4 text-xl font-semibold">Project Flows</h1>
              <p className="text-sm text-muted-foreground">
                {Object.keys(activeTab.project.flows ?? {}).length} flow(s) defined.
              </p>
            </div>
          )}
          {mode === "projects" && view === "tools" && <ToolRunnerPage />}

          {view === "settings" && <SettingsPage />}
        </main>
      </div>

      {showNewProjectForm && (
        <NewProjectDialog onCreate={handleNewProject} onCancel={() => setShowNewProjectForm(false)} shortenHome={shortenHome} />
      )}
    </div>
  );
}

function NewProjectDialog({ onCreate, onCancel, shortenHome }: {
  onCreate: (name: string, savePath?: string) => void; onCancel: () => void; shortenHome: (path: string) => string;
}) {
  const [name, setName] = useState("");
  const [customPath, setCustomPath] = useState("");
  const INVALID = /[<>:"/\\|?*\x00-\x1f]/;
  const valid = name.trim().length > 0 && !INVALID.test(name) && name !== "." && name !== "..";
  const trimmed = name.trim();
  const saveDir = customPath ? `${customPath}/${trimmed}` : valid ? `~/KapiProjects/${trimmed}` : "";
  const handleBrowse = async () => { const dir = await api.browseProjectLocation(); if (dir) setCustomPath(shortenHome(dir)); };
  const handleCreate = () => { if (valid) onCreate(trimmed, saveDir ? `${saveDir}/project.kapi` : undefined); };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-xl border border-border bg-background p-6 shadow-lg">
        <h2 className="mb-4 text-lg font-semibold">New Project</h2>
        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">Project Name</label>
            <div className="flex items-center gap-1.5">
              <input type="text" value={name} onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") handleCreate(); }} placeholder="My App" autoFocus
                className={`flex-1 rounded-lg border bg-transparent px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring ${name && !valid ? "border-destructive" : "border-input"}`} />
              <button onClick={handleBrowse} className="shrink-0 rounded-lg border border-border p-2 text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Choose location">
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"/></svg>
              </button>
            </div>
            {name && !valid ? <p className="mt-1 text-xs text-destructive">Invalid directory name</p>
              : saveDir ? <p className="mt-1 text-xs text-muted-foreground">Saved to {saveDir}</p> : null}
          </div>
          <div className="flex gap-2">
            <button onClick={handleCreate} disabled={!valid} className="flex-1 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50">Create Project</button>
            <button onClick={onCancel} className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent">Cancel</button>
          </div>
        </div>
      </div>
    </div>
  );
}
