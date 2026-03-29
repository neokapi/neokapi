import { useState, useCallback, useEffect } from "react";
import { Settings } from "lucide-react";
import type { View, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { WelcomePage } from "./components/WelcomePage";
import { HomePage } from "./components/HomePage";
import { ContentPage } from "./components/ContentPage";
import { ProjectPage } from "./components/ProjectPage";
import { FlowPage } from "./components/FlowPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { SettingsPage } from "./components/SettingsPage";
import { Sidebar } from "./components/Sidebar";
import { TabBar } from "./components/TabBar";

interface TabState {
  info: TabInfo;
  project: KapiProject;
  view: View;
}

export default function App() {
  const [tabs, setTabs] = useState<TabState[]>([]);
  const [activeTabID, setActiveTabID] = useState<string | null>(null);
  const [showSettings, setShowSettings] = useState(false);
  const [selectedFlow, setSelectedFlow] = useState<string | null>(null);

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  const addTab = useCallback(
    (tab: TabInfo, project: KapiProject) => {
      setTabs((prev) => {
        if (prev.some((t) => t.info.id === tab.id)) return prev;
        return [...prev, { info: tab, project, view: "home" }];
      });
      setActiveTabID(tab.id);
    },
    [],
  );

  const handleNewProject = useCallback(
    async (proj: KapiProject) => {
      const tab = await api.newProject(
        proj.name,
        proj.source_language ?? "en-US",
        proj.target_languages ?? [],
        "", // empty savePath → defaults to ~/KapiProjects/{name}/project.kapi
      );
      if (tab) {
        addTab(tab, proj);
      }
    },
    [addTab],
  );

  const handleOpenTab = useCallback(
    async (tab: TabInfo) => {
      const proj = await api.getProject(tab.id);
      if (proj) addTab(tab, proj);
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

  const setActiveView = useCallback(
    (view: View) => {
      if (!activeTabID) return;
      setTabs((prev) =>
        prev.map((t) => (t.info.id === activeTabID ? { ...t, view } : t)),
      );
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

  // Listen for native menu events.
  useEffect(() => {
    const cleanups: Array<() => void> = [];

    import("@wailsio/runtime")
      .then(({ Events }) => {
        cleanups.push(
          Events.On("menu:new-project", async () => {
            // Create a new project with a default name.
            const tab = await api.newProject("New Project", "en-US", []);
            if (tab) {
              const proj = await api.getProject(tab.id);
              if (proj) addTab(tab, proj);
            }
          }),
        );

        cleanups.push(
          Events.On("menu:open-project", async () => {
            const tab = await api.openProjectDialog();
            if (tab) await handleOpenTab(tab);
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

        // Handle files opened from OS (double-click, drag to dock icon).
        cleanups.push(
          Events.On("open-project-tab", async (event: { data: unknown }) => {
            const tab = event.data as TabInfo;
            if (tab?.id) await handleOpenTab(tab);
          }),
        );

        cleanups.push(
          Events.On("menu:save-project-as", async () => {
            if (!activeTabID) return;
            const updated = await api.saveProjectDialog(activeTabID);
            if (updated) updateActiveTab(updated);
          }),
        );
      })
      .catch(() => {});

    return () => cleanups.forEach((fn) => fn());
  }, [activeTabID, handleNewProject, handleOpenTab, updateActiveTab]);

  // No tabs → welcome page or standalone settings.
  if (tabs.length === 0) {
    if (showSettings) {
      return (
        <div className="flex h-screen flex-col bg-background text-foreground">
          <div
            className="flex shrink-0 items-center justify-between border-b border-border bg-sidebar px-4 pt-10 pb-2"
            style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
          >
            <button
              onClick={() => setShowSettings(false)}
              className="text-xs text-muted-foreground hover:text-foreground"
              style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            >
              &larr; Back
            </button>
          </div>
          <SettingsPage />
        </div>
      );
    }

    return (
      <WelcomePage
        onOpen={async (tab) => await handleOpenTab(tab)}
        onNew={handleNewProject}
        onSettings={() => setShowSettings(true)}
      />
    );
  }

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      {/* Title bar with tab bar + settings gear */}
      <div
        className="flex shrink-0 items-end border-b border-border bg-sidebar pt-10"
        style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      >
        <div className="flex-1" style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
          <TabBar
            tabs={tabs.map((t) => t.info)}
            activeTabID={activeTabID}
            onSelect={setActiveTabID}
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
        <div
          className="shrink-0 px-3 pb-1.5"
          style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
        >
          <button
            onClick={() => setShowSettings((v) => !v)}
            className={`rounded-md p-1.5 transition-colors ${
              showSettings
                ? "bg-accent text-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-foreground"
            }`}
            aria-label="Settings"
          >
            <Settings size={16} />
          </button>
        </div>
      </div>

      {/* Main content */}
      <div className="flex flex-1 overflow-hidden">
        {showSettings ? (
          <main className="flex-1 overflow-auto">
            <SettingsPage />
          </main>
        ) : (
          <>
            <Sidebar
              activeView={activeTab?.view ?? "home"}
              onViewChange={(view) => {
                setShowSettings(false);
                setActiveView(view);
                if (view !== "flows") setSelectedFlow(null);
              }}
              flowNames={Object.keys(activeTab?.project.flows ?? {})}
              selectedFlow={selectedFlow}
              onSelectFlow={setSelectedFlow}
              onAddFlow={() => {
                if (!activeTab) return;
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
                if (!activeTab) return;
                const flows = { ...activeTab.project.flows };
                delete flows[name];
                updateActiveProject({ ...activeTab.project, flows });
                if (selectedFlow === name) setSelectedFlow(null);
              }}
            />
            <main className="flex-1 overflow-auto">
              {activeTab?.view === "home" && (
                <HomePage
                  project={activeTab.project}
                  onNavigate={(view) => {
                    setActiveView(view);
                  }}
                />
              )}
              {activeTab?.view === "content" && (
                <ContentPage
                  project={activeTab.project}
                  projectPath={activeTab.info.path}
                  onUpdate={updateActiveProject}
                  tabID={activeTab.info.id}
                />
              )}
              {activeTab?.view === "flows" && selectedFlow && activeTab.project.flows?.[selectedFlow] && (
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
              {activeTab?.view === "flows" && !selectedFlow && (
                <div className="flex h-full items-center justify-center text-muted-foreground">
                  <p className="text-sm">Select a flow or create a new one</p>
                </div>
              )}
              {activeTab?.view === "tools" && <ToolRunnerPage />}
            </main>
          </>
        )}
      </div>
    </div>
  );
}
