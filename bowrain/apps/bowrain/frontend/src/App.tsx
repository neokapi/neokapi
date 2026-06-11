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
import { BackendEventsProvider, useBackendEvents } from "./hooks/useBackendEvents";
import { WailsApiAdapter } from "./api/WailsApiAdapter";
import type { ProjectInfo, Workspace, User } from "@neokapi/ui";
import { Shuffle, Link, Loader2, Users, ShieldCheck } from "lucide-react";
import { Events } from "@wailsio/runtime";
import { Backend } from "./api/backend";

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

/**
 * FreshnessBridge wires the backend freshness events to the App's refetch
 * callbacks. Rendered inside BackendEventsProvider so its useBackendEvents
 * subscriptions reach the shared Wails event fan-out. On reconnect, every
 * registered listener re-runs (handled by the provider), so all open views
 * pull fresh authoritative state after an offline gap.
 */
function FreshnessBridge({
  reloadProjects,
  reloadActiveProject,
  reloadOpenEditor,
}: {
  reloadProjects: () => void;
  reloadActiveProject: () => void;
  reloadOpenEditor: () => void;
}) {
  // Block edits from other users / pushes / flows → reload the open editor and
  // the project counts.
  useBackendEvents(["blocks-changed", "flow-changed"], () => {
    reloadOpenEditor();
    reloadActiveProject();
  });
  // Item add/remove, project lifecycle, connector sync → reload project + list.
  useBackendEvents(["project-changed", "connector-sync"], () => {
    reloadActiveProject();
    reloadProjects();
  });
  // Membership/brand/termbase/stream changes → refresh the active project view
  // (members/brand/term panels read from it) and the list.
  useBackendEvents(
    ["membership-changed", "brand-voice-changed", "termbase-changed", "stream-changed"],
    () => {
      reloadActiveProject();
      reloadProjects();
    },
  );
  return null;
}

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

  // Bumped by the backend-events freshness layer to force the open editor to
  // reload blocks when an external change touches the active project.
  const [blocksReloadSignal, setBlocksReloadSignal] = useState(0);

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

  // --- Freshness: reload data on external changes ---

  // Reload the active project (item list, counts) from the server. Used when an
  // external change (another user, kapi push, connector sync, automation,
  // item add/remove) touches the open project.
  const reloadActiveProject = useCallback(() => {
    setActiveProject((current) => {
      if (!current) return current;
      void wailsAdapter
        .getProject(workspace.slug, current.id)
        .then((fresh) => {
          setActiveProject(fresh);
          setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
        })
        .catch(() => {
          /* keep current on failure */
        });
      return current;
    });
  }, [workspace.slug]);

  // Reload the workspace project list (project create/delete/rename elsewhere).
  const reloadProjects = useCallback(() => {
    Backend.ListProjects()
      .then((p: ProjectInfo[]) => setProjects(p?.length ? p : []))
      .catch(() => {
        /* keep current on failure */
      });
  }, []);

  // Force the open editor to reload its blocks.
  const reloadOpenEditor = useCallback(() => {
    setBlocksReloadSignal((n) => n + 1);
  }, []);

  // Open/close the gRPC WatchProject stream as the active project changes (server
  // mode only). This is what feeds the typed backend events the freshness layer
  // listens to; without it no external change would ever reach the desktop UI.
  useEffect(() => {
    if (!isServerMode || !activeProject) {
      void connection.stopWatching();
      return;
    }
    void connection.startWatching(activeProject.id);
    return () => {
      void connection.stopWatching();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isServerMode, activeProject?.id]);

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
          reloadSignal={blocksReloadSignal}
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
            <BackendEventsProvider>
              <FreshnessBridge
                reloadProjects={reloadProjects}
                reloadActiveProject={reloadActiveProject}
                reloadOpenEditor={reloadOpenEditor}
              />
            </BackendEventsProvider>
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
