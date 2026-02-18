import { useState, useCallback, useMemo, useEffect } from "react";
import { Header } from "./components/Header";
import { ServerConnect } from "./components/ServerConnect";
import { WorkspaceSelector } from "./components/WorkspaceSelector";
import type { WorkspaceOption } from "./components/WorkspaceSelector";
import { SettingsPage } from "./components/SettingsPage";
import {
  ApiProvider,
  WorkspaceProvider,
  ThemeProvider,
  AppSidebar,
  ProjectDashboard,
  ProjectView,
  TranslationEditor,
  TMExplorer,
  TermExplorer,
  cn,
  type View,
  type NavItem,
} from "@gokapi/ui";
import { FlowBuilder } from "./components/FlowBuilder";
import { ConnectorPanel } from "./components/ConnectorPanel";
import { DocumentPreview } from "./components/DocumentPreview";
import { useConnection } from "./hooks/useApi";
import { WailsApiAdapter } from "./api/WailsApiAdapter";
import type { ProjectInfo, BlockInfo, Workspace, User } from "@gokapi/ui";
import { Shuffle, Link, Loader2 } from "lucide-react";
import { Events } from "@wailsio/runtime";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../bindings/github.com/gokapi/gokapi/bowrain/apps/bowrain/backend/app.js";

type AppView = View | "flows" | "connectors";
type AppMode = "loading" | "connecting" | "workspace-select" | "ready";

const desktopNavItems: NavItem[] = [
  { id: "flows", label: "Flows", icon: <Shuffle className="w-4 h-4" /> },
  { id: "connectors", label: "Connectors", icon: <Link className="w-4 h-4" /> },
];

const wailsAdapter = new WailsApiAdapter();
const localWorkspace = { id: "local", name: "Personal", slug: "personal", description: "", logo_url: "", type: "personal" as const, role: "owner" as const };

function App() {
  const connection = useConnection();

  // Connection flow state
  const [mode, setMode] = useState<AppMode>("loading");
  const [workspace, setWorkspace] = useState<Workspace>(localWorkspace);
  const [isServerMode, setIsServerMode] = useState(false);

  // App state
  const [activeView, setActiveView] = useState<AppView>("translate");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  // Project state
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  const [showTMExplorer, setShowTMExplorer] = useState(false);
  const [showTermExplorer, setShowTermExplorer] = useState(false);
  const [pendingChanges, setPendingChanges] = useState(0);

  // Listen for connection state changes from the backend (e.g. going offline).
  useEffect(() => {
    const cancel = Events.On("connection-state-changed", (event: { data: unknown }) => {
      const info = event.data as { state: string };
      connection.refresh();
      if (info?.state === "offline") {
        Backend.GetPendingChangesCount?.()
          .then((n: number) => setPendingChanges(n))
          .catch(() => {});
      } else if (info?.state === "connected") {
        setPendingChanges(0);
      }
    });
    return () => { cancel?.(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- Connection flow ---

  // Initial check: auto-reconnect from stored auth
  useEffect(() => {
    connection.refresh().then((ci) => {
      if (ci.state === "connected" && ci.workspace) {
        // Auto-reconnected with workspace already selected
        connection.getServerWorkspaces().then((wsList) => {
          const ws = wsList.find((w) => w.slug === ci.workspace);
          if (ws) {
            setWorkspace({ ...ws, logo_url: "", type: "team" as const, role: ws.role as "owner" });
          } else {
            setWorkspace({ id: ci.workspace!, name: ci.workspace!, slug: ci.workspace!, description: "", logo_url: "", type: "team" as const, role: "owner" });
          }
          setIsServerMode(true);
          setMode("ready");
        }).catch(() => {
          setIsServerMode(true);
          setWorkspace({ id: ci.workspace!, name: ci.workspace!, slug: ci.workspace!, description: "", logo_url: "", type: "team" as const, role: "owner" });
          setMode("ready");
        });
      } else if (ci.state === "connected") {
        setIsServerMode(true);
        setMode("workspace-select");
      } else if (ci.state === "offline" && ci.workspace) {
        // Server unreachable but cached auth exists — go to app in offline mode
        setWorkspace({ id: ci.workspace, name: ci.workspace, slug: ci.workspace, description: "", logo_url: "", type: "team" as const, role: "owner" });
        setIsServerMode(true);
        setMode("ready");
        Backend.GetPendingChangesCount?.()
          .then((n: number) => setPendingChanges(n))
          .catch(() => {});
      } else {
        setMode("connecting");
      }
    }).catch(() => {
      setMode("connecting");
    });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleServerConnect = useCallback(async (serverURL: string) => {
    const ci = await connection.connect(serverURL);
    if (ci.state === "connected") {
      setIsServerMode(true);
      setMode("workspace-select");
    }
    return ci;
  }, [connection]);

  const handleSelectWorkspace = useCallback(async (ws: WorkspaceOption) => {
    await connection.selectWorkspace(ws.slug);
    setWorkspace({ ...ws, logo_url: "", type: "team" as const, role: ws.role as "owner" });
    setIsServerMode(true);
    setMode("ready");
  }, [connection]);

  const handleDisconnect = useCallback(async () => {
    await connection.disconnect();
    setIsServerMode(false);
    setWorkspace(localWorkspace);
    setActiveProject(null);
    setActiveFile(null);
    setProjects([]);
    setMode("connecting");
  }, [connection]);

  // --- Project callbacks ---

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      try {
        const info = await wailsAdapter.createProject("personal", name, sourceLang, targetLangs);
        setProjects((prev) => [...prev, info]);
        setActiveProject(info);
      } catch (e) {
        console.error("Create project failed:", e);
      }
    },
    [],
  );

  const handleOpenProject = useCallback(
    async (project: ProjectInfo) => {
      try {
        const fresh = await wailsAdapter.getProject("personal", project.id);
        setActiveProject(fresh);
        setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
      } catch {
        setActiveProject(project);
      }
      setActiveFile(null);
    },
    [],
  );

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      if (!activeProject) return;
      try {
        const updated = await wailsAdapter.uploadFiles("personal", activeProject.id, files);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Add files failed:", e);
      }
    },
    [activeProject],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      if (!activeProject) return;
      try {
        const updated = await wailsAdapter.removeFile("personal", activeProject.id, fileName);
        setActiveProject(updated);
        setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
      } catch (e) {
        console.error("Remove file failed:", e);
      }
    },
    [activeProject],
  );

  const handleOpenFile = useCallback((fileName: string) => {
    setActiveFile(fileName);
  }, []);

  const handleBackToProjects = useCallback(() => {
    setActiveProject(null);
    setActiveFile(null);
    setShowTMExplorer(false);
    setShowTermExplorer(false);
  }, []);

  const handleBackToProject = useCallback(() => {
    setActiveFile(null);
    setShowTMExplorer(false);
    setShowTermExplorer(false);
  }, []);

  const handleOpenTM = useCallback(() => {
    setShowTMExplorer(true);
    setShowTermExplorer(false);
  }, []);

  const handleOpenTerms = useCallback(() => {
    setShowTermExplorer(true);
    setShowTMExplorer(false);
  }, []);

  const handleViewChange = useCallback((view: AppView) => {
    setActiveView(view);
    if (view !== "translate") {
      setActiveProject(null);
      setActiveFile(null);
      setShowTMExplorer(false);
    }
  }, []);

  // Export handler for desktop: the WailsApiAdapter already handles export + open
  const handleDesktopExport = useCallback((_blob: Blob, _fileName: string) => {
    // No-op: WailsApiAdapter.exportTranslatedFile already exported to disk and opened in OS
  }, []);

  // Render preview for split layout modes
  const renderDesktopPreview = useMemo(() => {
    return (props: {
      projectId: string;
      itemName: string;
      targetLocale: string;
      selectedBlockId?: string;
      onBlockSelect: (blockId: string) => void;
      blocks: BlockInfo[];
    }) => (
      <DocumentPreview
        projectId={props.projectId}
        itemName={props.itemName}
        targetLocale={props.targetLocale}
        selectedBlockId={props.selectedBlockId}
        onBlockSelect={props.onBlockSelect}
        blocks={props.blocks}
      />
    );
  }, []);

  // Construct user object from connection info for the sidebar
  const sidebarUser: User | null = isServerMode && connection.info.user_name
    ? { id: "server", email: connection.info.user_name, name: connection.info.user_name, avatar_url: "" }
    : null;

  // Connection state for sidebar badge
  const connectionState = isServerMode ? connection.info.state as "disconnected" | "connecting" | "connected" | "offline" : undefined;

  // --- Pre-app screens ---

  if (mode === "loading") {
    return (
      <ThemeProvider>
        <div className="flex items-center justify-center h-screen bg-background">
          <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
        </div>
      </ThemeProvider>
    );
  }

  if (mode === "connecting") {
    return (
      <ThemeProvider>
        <div className="h-screen bg-background flex flex-col">
          <div
            className="h-10 shrink-0"
            style={{
              // @ts-expect-error non-standard CSS property for Wails
              "--wails-draggable": "drag",
            }}
          />
          <ServerConnect
            info={connection.info}
            onConnect={handleServerConnect}
            onStartLogin={connection.startLogin}
            onWaitForLogin={connection.waitForLogin}
            onCancelLogin={connection.cancelLogin}
          />
        </div>
      </ThemeProvider>
    );
  }

  if (mode === "workspace-select") {
    return (
      <ThemeProvider>
        <div className="h-screen bg-background flex flex-col">
          <div
            className="h-10 shrink-0"
            style={{
              // @ts-expect-error non-standard CSS property for Wails
              "--wails-draggable": "drag",
            }}
          />
          <WorkspaceSelector
            userName={connection.info.user_name}
            onSelect={handleSelectWorkspace}
            onBack={() => {
              connection.disconnect();
              setIsServerMode(false);
              setMode("connecting");
            }}
            getWorkspaces={connection.getServerWorkspaces}
          />
        </div>
      </ThemeProvider>
    );
  }

  // --- Main app (mode === "ready") ---

  const renderView = () => {
    if (activeView === "translate" && activeProject && showTermExplorer) {
      return (
        <TermExplorer
          sourceLocale={activeProject.source_locale}
          targetLocales={activeProject.target_locales}
          projectName={activeProject.name}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && showTMExplorer) {
      return (
        <TMExplorer
          sourceLocale={activeProject.source_locale}
          targetLocales={activeProject.target_locales}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && activeFile) {
      return (
        <TranslationEditor
          project={activeProject}
          fileName={activeFile}
          onBack={handleBackToProject}
          onExport={handleDesktopExport}
          renderPreview={renderDesktopPreview}
        />
      );
    }

    if (activeView === "translate" && activeProject) {
      return (
        <ProjectView
          project={activeProject}
          onBack={handleBackToProjects}
          onOpenFile={handleOpenFile}
          onUploadFiles={isServerMode ? () => {} : handleUploadFiles}
          onRemoveFile={isServerMode ? () => {} : handleRemoveFile}
          onOpenTM={handleOpenTM}
          onOpenTerms={handleOpenTerms}
        />
      );
    }

    switch (activeView) {
      case "translate":
        return (
          <ProjectDashboard
            projects={projects}
            onCreateProject={isServerMode ? () => {} : handleCreateProject}
            onOpenProject={handleOpenProject}
          />
        );
      case "termbase":
        return <div className="text-muted-foreground p-6">Select a project to explore its termbase.</div>;
      case "memory":
        return <div className="text-muted-foreground p-6">Select a project to explore its translation memory.</div>;
      case "settings":
        return <SettingsPage />;
      case "flows":
        return <FlowBuilder />;
      case "connectors":
        return <ConnectorPanel />;
    }
  };

  const isEditor = activeView === "translate" && activeProject != null && activeFile != null;
  const isFlowBuilder = activeView === "flows";

  return (
    <ThemeProvider>
      <ApiProvider adapter={wailsAdapter}>
        <WorkspaceProvider initialWorkspace={workspace}>
          <div className="flex h-screen overflow-hidden">
            <AppSidebar
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={() => {}}
              activeView={activeView}
              onViewChange={handleViewChange}
              extraNavItems={desktopNavItems}
              user={sidebarUser}
              onSignOut={isServerMode ? handleDisconnect : undefined}
              collapsed={sidebarCollapsed}
              onCollapsedChange={setSidebarCollapsed}
              topSpacer={38}
              collapsedWidth={60}
              connectionState={connectionState}
              pendingChanges={pendingChanges}
              showThemeToggle={false}
            />
            <div className="flex-1 flex flex-col min-h-0">
              <Header
                sidebarCollapsed={sidebarCollapsed}
                connectionState={isServerMode ? connection.info.state : "disconnected"}
                pendingChanges={pendingChanges}
                onDisconnect={isServerMode ? handleDisconnect : undefined}
              />
              <main
                className={cn(
                  "flex-1 p-6 flex flex-col min-h-0",
                  isEditor || isFlowBuilder ? "overflow-hidden" : "overflow-auto",
                )}
              >
                {renderView()}
              </main>
            </div>
          </div>
        </WorkspaceProvider>
      </ApiProvider>
    </ThemeProvider>
  );
}

export default App;
