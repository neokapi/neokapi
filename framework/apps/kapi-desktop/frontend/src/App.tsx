import { useState, useCallback, useEffect } from "react";
import type { View, KapiProject, TabInfo } from "./types/api";
import { api } from "./hooks/useApi";
import { WelcomePage } from "./components/WelcomePage";
import { ProjectPage } from "./components/ProjectPage";
import { FlowPage } from "./components/FlowPage";
import { ToolRunnerPage } from "./components/ToolRunnerPage";
import { PluginManager } from "./components/PluginManager";
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

  const activeTab = tabs.find((t) => t.info.id === activeTabID) ?? null;

  const addTab = useCallback(
    (tab: TabInfo, project: KapiProject) => {
      setTabs((prev) => {
        if (prev.some((t) => t.info.id === tab.id)) return prev;
        return [...prev, { info: tab, project, view: "project" }];
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
          Events.On("menu:new-project", () => {
            handleNewProject({
              version: "v1",
              name: "Untitled Project",
              source_language: "en-US",
              target_languages: [],
              flows: {},
            });
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

  // No tabs → welcome page.
  if (tabs.length === 0) {
    return (
      <WelcomePage
        onOpen={async (tab) => await handleOpenTab(tab)}
        onNew={handleNewProject}
      />
    );
  }

  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      {/* Title bar with tab bar — draggable region with non-draggable tabs */}
      <div
        className="flex shrink-0 items-end border-b border-border bg-sidebar pt-8"
        style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
      >
        <div style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}>
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
      </div>

      {/* Main content */}
      <div className="flex flex-1 overflow-hidden">
        <Sidebar
          activeView={activeTab?.view ?? "project"}
          onViewChange={setActiveView}
        />
        <main className="flex-1 overflow-auto">
          {activeTab?.view === "project" && (
            <ProjectPage
              project={activeTab.project}
              projectPath={activeTab.info.path}
              onSaved={updateActiveTab}
              tabID={activeTab.info.id}
            />
          )}
          {activeTab?.view === "flows" && (
            <FlowPage
              project={activeTab.project}
              onUpdate={updateActiveProject}
            />
          )}
          {activeTab?.view === "tools" && <ToolRunnerPage />}
          {activeTab?.view === "plugins" && <PluginManager />}
          {activeTab?.view === "settings" && <SettingsPage />}
        </main>
      </div>
    </div>
  );
}
