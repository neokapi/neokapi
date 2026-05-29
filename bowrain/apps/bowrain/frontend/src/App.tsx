import { useState, useCallback, useMemo, useEffect } from "react";
import { TopBar } from "@neokapi/ui";
import { ServerConnect } from "./components/ServerConnect";
import { SettingsPage } from "./components/SettingsPage";
import {
  ApiProvider,
  WorkspaceProvider,
  ThemeProvider,
  TooltipProvider,
  AppShell,
  CreateWorkspaceDialog,
  ProjectDashboard,
  ProjectView,
  TMExplorer,
  TermExplorer,
  cn,
  type View,
  type NavItem,
  type SidebarContext,
} from "@neokapi/ui";
import {
  ReviewSurface,
  PreProcessSurface,
  EditorSurfaceTabs,
  type EditorSurface,
} from "@neokapi/ui";
import { FlowBuilder } from "./components/FlowBuilder";
import { ConnectorPanel } from "./components/ConnectorPanel";
import { DesktopTranslateView } from "./components/DesktopTranslateView";
import { MembersPage } from "./components/MembersPage";
import { BrandPage } from "./components/BrandPage";
import { useConnection } from "./hooks/useApi";
import { WailsApiAdapter } from "./api/WailsApiAdapter";
import type { ProjectInfo, Workspace, User } from "@neokapi/ui";
import { Shuffle, Link, Loader2, Users, ShieldCheck } from "lucide-react";
import { Events } from "@wailsio/runtime";
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";

type AppView = View | "flows" | "connectors" | "members" | "brand";
type AppMode = "loading" | "connecting" | "ready";

function toWorkspace(ws: {
  id: string;
  slug: string;
  name: string;
  description: string;
  role: string;
}): Workspace {
  return { ...ws, logo_url: "", type: "team" as const, role: ws.role as "owner" };
}

const baseNavItems: NavItem[] = [
  { id: "flows", label: "Flows", icon: <Shuffle className="w-4 h-4" /> },
  { id: "connectors", label: "Connectors", icon: <Link className="w-4 h-4" /> },
];

// Governance nav entries (members, brand) require a connected server +
// workspace, so they only appear in server mode.
const governanceNavItems: NavItem[] = [
  { id: "members", label: "Members", icon: <Users className="w-4 h-4" /> },
  { id: "brand", label: "Brand", icon: <ShieldCheck className="w-4 h-4" /> },
];

const wailsAdapter = new WailsApiAdapter();
const localWorkspace = {
  id: "local",
  name: "Personal",
  slug: "personal",
  description: "",
  logo_url: "",
  type: "personal" as const,
  role: "owner" as const,
};

function App() {
  const connection = useConnection();

  // Connection flow state
  const [mode, setMode] = useState<AppMode>("loading");
  const [workspace, setWorkspace] = useState<Workspace>(localWorkspace);
  const [allWorkspaces, setAllWorkspaces] = useState<Workspace[]>([localWorkspace]);
  const [isServerMode, setIsServerMode] = useState(false);

  // App state
  const [activeView, setActiveView] = useState<AppView>("translate");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  // Project state
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  // Which per-file editor surface is showing (translate / review / pre-process).
  const [fileSurface, setFileSurface] = useState<EditorSurface>("translate");
  const [showTMExplorer, setShowTMExplorer] = useState(false);
  const [showTermExplorer, setShowTermExplorer] = useState(false);
  const [pendingChanges, setPendingChanges] = useState(0);
  const [showCreateWs, setShowCreateWs] = useState(false);

  // Listen for connection state changes from the backend (e.g. going offline).
  useEffect(() => {
    const cancel = Events.On("connection-state-changed", (event: { data: unknown }) => {
      const info = event.data as { state: string };
      void connection.refresh();
      if (info?.state === "offline") {
        Backend.GetPendingChangesCount?.()
          .then((n: number) => setPendingChanges(n))
          .catch(() => {});
      } else if (info?.state === "connected") {
        setPendingChanges(0);
      }
    });
    return () => {
      cancel?.();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Listen for deep-link-project events from bowrain:// URL handler.
  useEffect(() => {
    const cancel = Events.On("deep-link-project", (event: { data: unknown }) => {
      const info = event.data as { project_id: string; server_url: string; workspace: string };
      if (!info?.project_id) return;
      void connection.refresh().then((ci) => {
        if (ci.state === "connected") {
          wailsAdapter
            .getProject(info.workspace || "", info.project_id)
            .then((proj) => {
              setActiveProject(proj);
              setActiveFile(null);
              setShowTMExplorer(false);
              setShowTermExplorer(false);
            })
            .catch(() => {
              console.warn("Deep link: could not open project", info.project_id);
            });
        }
      });
    });
    return () => {
      cancel?.();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // --- Connection flow ---

  // Transition out of "connecting" once the backend reports "connected".
  // This handles the auto-connect (BOWRAIN_TOKEN) race where the first
  // refresh() may return "disconnected" because the autoconnect path
  // hasn't completed yet — connectWithToken emits "connection-state-changed"
  // when it succeeds, useConnection's listener calls refresh(), info updates,
  // and this effect promotes mode to "ready".
  useEffect(() => {
    if (connection.info.state === "connected" && mode === "connecting") {
      setMode("ready");
    }
  }, [connection.info.state, mode]);

  useEffect(() => {
    connection
      .refresh()
      .then(async (ci) => {
        if (ci.state === "connected") {
          setIsServerMode(true);
          try {
            const wsList = await connection.getServerWorkspaces();
            const mapped = wsList.map(toWorkspace);
            setAllWorkspaces(mapped);
            if (ci.workspace) {
              const ws = mapped.find((w) => w.slug === ci.workspace);
              setWorkspace(
                ws ??
                  toWorkspace({
                    id: ci.workspace,
                    slug: ci.workspace,
                    name: ci.workspace,
                    description: "",
                    role: "owner",
                  }),
              );
            } else if (mapped.length > 0) {
              await connection.selectWorkspace(mapped[0].slug);
              setWorkspace(mapped[0]);
            }
          } catch {
            if (ci.workspace) {
              setWorkspace(
                toWorkspace({
                  id: ci.workspace,
                  slug: ci.workspace,
                  name: ci.workspace,
                  description: "",
                  role: "owner",
                }),
              );
            }
          }
          setMode("ready");
        } else if (ci.state === "offline" && ci.workspace) {
          setWorkspace(
            toWorkspace({
              id: ci.workspace,
              slug: ci.workspace,
              name: ci.workspace,
              description: "",
              role: "owner",
            }),
          );
          setIsServerMode(true);
          setMode("ready");
          Backend.GetPendingChangesCount?.()
            .then((n: number) => setPendingChanges(n))
            .catch(() => {});
        } else if ((window as any).__skipConnection) {
          // Headless server mode (e2e tests): skip connection screen, use local workspace.
          setMode("ready");
        } else {
          setMode("connecting");
        }
      })
      .catch(() => {
        if ((window as any).__skipConnection) {
          setMode("ready");
        } else {
          setMode("connecting");
        }
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (mode !== "ready") return;
    Backend.ListProjects()
      .then((serverProjects: ProjectInfo[]) => {
        if (serverProjects?.length) {
          setProjects(serverProjects);
        }
      })
      .catch(() => {});
  }, [mode]);

  const handleServerConnect = useCallback(
    async (serverURL: string) => {
      const ci = await connection.connect(serverURL);
      if (ci.state === "connected") {
        setIsServerMode(true);
        try {
          const wsList = await connection.getServerWorkspaces();
          const mapped = wsList.map(toWorkspace);
          setAllWorkspaces(mapped);
          if (mapped.length > 0) {
            await connection.selectWorkspace(mapped[0].slug);
            setWorkspace(mapped[0]);
          }
        } catch {
          /* ignore */
        }
        setMode("ready");
      }
      return ci;
    },
    [connection],
  );

  const handleSelectWorkspace = useCallback(
    async (ws: Workspace) => {
      if (isServerMode) {
        await connection.selectWorkspace(ws.slug);
      }
      setWorkspace(ws);
      setActiveProject(null);
      setActiveFile(null);
      setShowTMExplorer(false);
      setShowTermExplorer(false);
      Backend.ListProjects()
        .then((p: ProjectInfo[]) => setProjects(p?.length ? p : []))
        .catch(() => setProjects([]));
    },
    [connection, isServerMode],
  );

  const handleWorkspaceCreated = useCallback(
    async (ws: Workspace) => {
      setShowCreateWs(false);
      setAllWorkspaces((prev) => [...prev, ws]);
      if (isServerMode) {
        await connection.selectWorkspace(ws.slug);
      }
      setWorkspace(ws);
      setActiveProject(null);
      setActiveFile(null);
      setShowTMExplorer(false);
      setShowTermExplorer(false);
      Backend.ListProjects()
        .then((p: ProjectInfo[]) => setProjects(p?.length ? p : []))
        .catch(() => setProjects([]));
    },
    [connection, isServerMode],
  );

  const handleSignOut = useCallback(async () => {
    await connection.logout();
    setIsServerMode(false);
    setWorkspace(localWorkspace);
    setAllWorkspaces([localWorkspace]);
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

  const handleOpenProject = useCallback(async (project: ProjectInfo) => {
    try {
      const fresh = await wailsAdapter.getProject("personal", project.id);
      setActiveProject(fresh);
      setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
    } catch {
      setActiveProject(project);
    }
    setActiveFile(null);
  }, []);

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

  const handleOpenFile = useCallback(
    (itemId: string) => {
      // Resolve item ID to filename from active project's items.
      const item = activeProject?.items?.find((i) => i.id === itemId);
      setActiveFile(item?.name ?? itemId);
      setFileSurface("translate");
    },
    [activeProject],
  );

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

  const handleDesktopExport = useCallback((_blob: Blob, _fileName: string) => {
    // No-op: WailsApiAdapter.exportTranslatedFile already exported to disk and opened in OS
  }, []);

  const sidebarUser: User | null =
    isServerMode && connection.info.user_name
      ? {
          id: "server",
          email: connection.info.user_name,
          name: connection.info.user_name,
          avatar_url: "",
        }
      : null;

  const connectionState = isServerMode
    ? (connection.info.state as "disconnected" | "connecting" | "connected" | "offline")
    : undefined;

  // Show governance nav (members, brand) only in server mode — those screens
  // proxy server REST endpoints and need a connected workspace.
  const desktopNavItems = useMemo<NavItem[]>(
    () => (isServerMode ? [...baseNavItems, ...governanceNavItems] : baseNavItems),
    [isServerMode],
  );

  // Build sidebar context so the sidebar transforms based on navigation depth.
  // Must be before early returns to maintain consistent hook call order.
  const sidebarContext = useMemo<SidebarContext | undefined>(() => {
    if (activeView !== "translate" || !activeProject) {
      return { level: "workspace", activeView: activeView as View };
    }
    return {
      level: "project",
      project: activeProject,
      activeStream: "main",
      activeProjectView: "dashboard" as const,
      // Home goes up one level: editor → project, project → workspace
      onBack: activeFile ? handleBackToProject : handleBackToProjects,
      onOpenDashboard: handleBackToProject,
      onOpenFile: handleOpenFile,
      onStreamChange: () => {},
    };
  }, [
    activeView,
    activeProject,
    activeFile,
    handleBackToProjects,
    handleBackToProject,
    handleOpenFile,
  ]);

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

  // --- Main app (mode === "ready") ---

  const renderView = () => {
    if (activeView === "translate" && activeProject && showTermExplorer) {
      return (
        <TermExplorer
          sourceLocale={activeProject.default_source_language}
          targetLocales={activeProject.target_languages}
          projectName={activeProject.name}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && showTMExplorer) {
      return (
        <TMExplorer
          sourceLocale={activeProject.default_source_language}
          targetLocales={activeProject.target_languages}
          onBack={handleBackToProject}
        />
      );
    }

    if (activeView === "translate" && activeProject && activeFile) {
      const surfaceTabs = <EditorSurfaceTabs active={fileSurface} onSelect={setFileSurface} />;
      if (fileSurface === "review") {
        return (
          <ReviewSurface
            project={activeProject}
            fileName={activeFile}
            onBack={handleBackToProject}
            surfaceTabs={surfaceTabs}
          />
        );
      }
      if (fileSurface === "pre-process") {
        return (
          <PreProcessSurface
            project={activeProject}
            fileName={activeFile}
            onBack={handleBackToProject}
            surfaceTabs={surfaceTabs}
          />
        );
      }
      return (
        <DesktopTranslateView
          adapter={wailsAdapter}
          project={activeProject}
          fileName={activeFile}
          workspaceSlug={isServerMode ? workspace.slug : ""}
          onBack={handleBackToProject}
          onExport={handleDesktopExport}
          surfaceTabs={surfaceTabs}
        />
      );
    }

    if (activeView === "translate" && activeProject) {
      return (
        <ProjectView
          project={activeProject}
          onBack={handleBackToProjects}
          onOpenFile={handleOpenFile}
          onUploadFiles={handleUploadFiles}
          onRemoveFile={handleRemoveFile}
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
            onCreateProject={handleCreateProject}
            onOpenProject={handleOpenProject}
          />
        );
      case "termbase":
        return (
          <div className="text-muted-foreground p-6">Select a project to explore its termbase.</div>
        );
      case "memory":
        return (
          <div className="text-muted-foreground p-6">
            Select a project to explore its translation memory.
          </div>
        );
      case "settings":
        return <SettingsPage />;
      case "flows":
        return <FlowBuilder projectId={activeProject?.id} />;
      case "connectors":
        return <ConnectorPanel />;
      case "members":
        return <MembersPage />;
      case "brand":
        return <BrandPage projects={projects} />;
    }
  };

  const isEditor = activeView === "translate" && activeProject != null && activeFile != null;
  const isFlowBuilder = activeView === "flows";

  return (
    <ThemeProvider>
      <TooltipProvider>
        <ApiProvider adapter={wailsAdapter}>
          <WorkspaceProvider initialWorkspace={workspace}>
            <AppShell
              workspaces={allWorkspaces}
              activeWorkspace={workspace}
              onSelectWorkspace={handleSelectWorkspace}
              onCreateWorkspace={isServerMode ? () => setShowCreateWs(true) : undefined}
              activeView={activeView}
              onViewChange={handleViewChange}
              extraNavItems={desktopNavItems}
              user={sidebarUser}
              onSignOut={isServerMode ? handleSignOut : undefined}
              collapsed={sidebarCollapsed}
              onCollapsedChange={setSidebarCollapsed}
              topBar
              connectionState={connectionState}
              pendingChanges={pendingChanges}
              showThemeToggle={false}
              sidebarContext={sidebarContext}
              headerSlot={
                <TopBar
                  user={sidebarUser}
                  onSignOut={isServerMode ? handleSignOut : undefined}
                  connectionState={
                    isServerMode
                      ? (connection.info.state as
                          | "disconnected"
                          | "connecting"
                          | "connected"
                          | "offline")
                      : undefined
                  }
                  pendingChanges={pendingChanges}
                />
              }
              contentClassName={cn(isEditor || isFlowBuilder ? "overflow-hidden" : "overflow-auto")}
            >
              {renderView()}
            </AppShell>

            <CreateWorkspaceDialog
              open={showCreateWs}
              onOpenChange={setShowCreateWs}
              onCreate={handleWorkspaceCreated}
            />
          </WorkspaceProvider>
        </ApiProvider>
      </TooltipProvider>
    </ThemeProvider>
  );
}

export default App;
