import { useCallback, useEffect, useState } from "react";
import type { KapiProject } from "./types/api";
import { api } from "./hooks/useApi";
import { useProjectHistory } from "./hooks/useProjectHistory";
import { useTabManager } from "./hooks/useTabManager";
import { useAppInit } from "./hooks/useAppInit";
import { useMenuEvents } from "./hooks/useMenuEvents";
import { ErrorProvider } from "./components/ErrorBanner";
import { IconSidebar } from "./components/IconSidebar";
import { ModeToggle } from "./components/ModeToggle";
import { TabBar } from "./components/TabBar";
import { SaveBar } from "./components/SaveBar";
import { UnsavedDialog } from "./components/UnsavedDialog";
import { ViewSwitch } from "./components/ViewSwitch";
import { NewProjectDialog } from "./components/NewProjectDialog";
import { useShortenHome } from "./hooks/useShortenHome";
import { Undo2, Redo2 } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";

export default function App() {
  return (
    <ErrorProvider>
      <AppInner />
    </ErrorProvider>
  );
}

function AppInner() {
  const shortenHome = useShortenHome();
  const { recentFiles, samplesDismissed, refreshRecent, dismissSamples } = useAppInit();
  const tm = useTabManager();

  const emptyProject: KapiProject = { version: "v1", name: "" };
  const history = useProjectHistory(
    tm.activeTab?.project ?? emptyProject,
    tm.activeTabID,
  ) as ReturnType<typeof useProjectHistory> & { cleanup: (id: string) => void };

  // Refresh recent files when tabs change.
  useEffect(() => {
    refreshRecent();
  }, [refreshRecent, tm.tabs.length]);

  // Warn before window close/quit if there are unsaved changes.
  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (history.isDirty) e.preventDefault();
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [history.isDirty]);

  // --- Project save ---
  const handleSaveProject = useCallback(async () => {
    if (!tm.activeTabID) return;
    const proj = history.project;
    await api.updateProject(tm.activeTabID, proj);
    await api.saveProject(tm.activeTabID);
    history.markSaved();
    tm.updateTab(tm.activeTabID, { project: proj });
  }, [tm, history]);

  // --- Keyboard shortcuts ---
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (tm.mode !== "projects" || !tm.activeTab) return;
      const meta = e.metaKey || e.ctrlKey;
      if (meta && e.key === "z" && !e.shiftKey) {
        e.preventDefault();
        history.undo();
      } else if (meta && e.key === "z" && e.shiftKey) {
        e.preventDefault();
        history.redo();
      } else if (meta && e.key === "s") {
        e.preventDefault();
        void handleSaveProject();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [tm.mode, tm.activeTab, history, handleSaveProject]);

  // --- Unsaved-changes guard for tab close ---
  const [pendingCloseTabID, setPendingCloseTabID] = useState<string | null>(null);

  const handleCloseTab = useCallback(
    (tabID: string) => {
      if (tabID === tm.activeTabID && history.isDirty) {
        setPendingCloseTabID(tabID);
        return;
      }
      history.cleanup(tabID);
      tm.closeTab(tabID);
    },
    [tm, history],
  );

  const handleUnsavedSave = useCallback(async () => {
    if (!pendingCloseTabID) return;
    setPendingCloseTabID(null);
    await handleSaveProject();
    history.cleanup(pendingCloseTabID);
    tm.closeTab(pendingCloseTabID);
  }, [pendingCloseTabID, handleSaveProject, history, tm]);

  const handleUnsavedDiscard = useCallback(() => {
    if (!pendingCloseTabID) return;
    setPendingCloseTabID(null);
    history.cleanup(pendingCloseTabID);
    tm.closeTab(pendingCloseTabID);
  }, [pendingCloseTabID, history, tm]);

  // --- Update project via history ---
  const updateProject = useCallback((project: KapiProject) => history.set(project), [history]);

  // --- Menu events ---
  useMenuEvents({
    activeTabID: tm.activeTabID,
    openProject: tm.openProject,
    openRecent: tm.openRecent,
    addTab: tm.addTab,
    updateTabInfo: tm.updateTabInfo,
    setShowNewProjectForm: tm.setShowNewProjectForm,
    setMode: tm.switchMode as (m: "projects") => void,
  });

  // --- Render ---
  return (
    <div className="flex h-screen flex-col bg-background text-foreground">
      <div className="flex min-h-0 flex-1">
        {/* Icon sidebar */}
        <div className="flex shrink-0 flex-col bg-sidebar">
          <div
            className="h-12 shrink-0"
            style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
          />
          <div className="flex-1 border-r border-border">
            <IconSidebar
              mode={tm.mode}
              active={tm.effectiveView}
              onChange={tm.navigate}
              projectDisabled={tm.mode === "projects" && !tm.activeTab}
            />
          </div>
        </div>

        {/* Right: top bar + content */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Top bar */}
          <div
            className="flex h-12 shrink-0 items-end border-b border-border bg-sidebar"
            style={{ WebkitAppRegion: "drag" } as React.CSSProperties}
          >
            {/* Undo / Redo */}
            {tm.mode === "projects" && tm.activeTab && (
              <div
                className="flex shrink-0 items-center gap-0.5 pb-1.5 pl-16"
                style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
              >
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={history.undo}
                  disabled={!history.canUndo}
                  aria-label="Undo"
                  title="Undo (⌘Z)"
                  className="h-7 w-7"
                >
                  <Undo2 size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={history.redo}
                  disabled={!history.canRedo}
                  aria-label="Redo"
                  title="Redo (⌘⇧Z)"
                  className="h-7 w-7"
                >
                  <Redo2 size={14} />
                </Button>
              </div>
            )}
            {/* Tabs or spacer */}
            <div
              className={`flex-1 ${tm.mode === "projects" && tm.activeTab ? "pl-2" : "pl-16"}`}
              style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            >
              {tm.mode === "projects" && tm.tabs.length > 0 && (
                <TabBar
                  tabs={tm.tabs.map((t) => t.info)}
                  activeTabID={tm.activeTabID}
                  onSelect={tm.selectTab}
                  onClose={handleCloseTab}
                  onRename={(id, name) =>
                    tm.updateTab(id, {
                      info: { ...tm.tabs.find((t) => t.info.id === id)!.info, name },
                    })
                  }
                />
              )}
            </div>
            {/* Mode toggle */}
            <div
              className="shrink-0 px-3 pb-1.5"
              style={{ WebkitAppRegion: "no-drag" } as React.CSSProperties}
            >
              <ModeToggle mode={tm.mode} onChange={tm.switchMode} />
            </div>
          </div>

          {/* Content */}
          <main className="flex-1 overflow-auto">
            <ViewSwitch
              mode={tm.mode}
              effectiveView={tm.effectiveView}
              activeTab={tm.activeTab}
              history={history}
              updateProject={updateProject}
              navigate={tm.navigate}
              updateTab={tm.updateTab}
              recentFiles={recentFiles}
              samplesDismissed={samplesDismissed}
              onOpenRecent={tm.openRecent}
              onNewProject={() => {
                tm.switchMode("projects");
                tm.setShowNewProjectForm(true);
              }}
              onOpenProject={tm.openProject}
              onCreateSampleProject={tm.createSampleProject}
              onDismissSamples={dismissSamples}
            />
          </main>
        </div>
      </div>

      {/* Save bar — full width below sidebar + content */}
      {tm.mode === "projects" && tm.activeTab && (
        <SaveBar isDirty={history.isDirty} onSave={handleSaveProject} />
      )}

      {/* Dialogs */}
      {tm.showNewProjectForm && (
        <NewProjectDialog
          onCreate={tm.createProject}
          onCancel={() => tm.setShowNewProjectForm(false)}
          shortenHome={shortenHome}
        />
      )}
      {pendingCloseTabID && (
        <UnsavedDialog
          onSave={() => void handleUnsavedSave()}
          onDiscard={handleUnsavedDiscard}
          onCancel={() => setPendingCloseTabID(null)}
        />
      )}
    </div>
  );
}
