import { useState, useEffect, useCallback } from "react";
import {
  AppShell,
  AuthProvider,
  WorkspaceProvider,
  ApiProvider,
  CreateWorkspaceDialog,
  useAuth,
  useWorkspace,
  useApi,
  RestApiAdapter,
  ProjectDashboard,
  ProjectView,
  TranslationEditor,
  TMExplorer,
  TermExplorer,
  type View,
  type Workspace,
  type ProjectInfo,
  type ConfigResponse,
  type WebVersionInfo,
  ThemeProvider,
} from "@gokapi/ui";
import { LoginPage } from "./auth/LoginPage";

const api = new RestApiAdapter();

// ---------------------------------------------------------------------------
// Translate view — state machine: Dashboard → Project → Editor
// ---------------------------------------------------------------------------

function TranslateView() {
  const adapter = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [activeProject, setActiveProject] = useState<ProjectInfo | null>(null);
  const [activeFile, setActiveFile] = useState<string | null>(null);
  const [showTM, setShowTM] = useState(false);
  const [showTerms, setShowTerms] = useState(false);

  useEffect(() => {
    if (!ws) return;
    adapter.listProjects(ws).then(setProjects).catch(() => setProjects([]));
  }, [ws, adapter]);

  const handleCreateProject = useCallback(
    async (name: string, sourceLang: string, targetLangs: string[]) => {
      const info = await adapter.createProject(ws, name, sourceLang, targetLangs);
      setProjects((prev) => [...prev, info]);
      setActiveProject(info);
    },
    [ws, adapter],
  );

  const handleOpenProject = useCallback(
    async (project: ProjectInfo) => {
      try {
        const fresh = await adapter.getProject(ws, project.id);
        setActiveProject(fresh);
        setProjects((prev) => prev.map((p) => (p.id === fresh.id ? fresh : p)));
      } catch {
        setActiveProject(project);
      }
      setActiveFile(null);
      setShowTM(false);
      setShowTerms(false);
    },
    [ws, adapter],
  );

  const handleUploadFiles = useCallback(
    async (files: File[]) => {
      if (!activeProject) return;
      const updated = await adapter.uploadFiles(ws, activeProject.id, files);
      setActiveProject(updated);
      setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
    },
    [ws, adapter, activeProject],
  );

  const handleRemoveFile = useCallback(
    async (fileName: string) => {
      if (!activeProject) return;
      const updated = await adapter.removeFile(ws, activeProject.id, fileName);
      setActiveProject(updated);
      setProjects((prev) => prev.map((p) => (p.id === updated.id ? updated : p)));
    },
    [ws, adapter, activeProject],
  );

  const handleBackToProjects = useCallback(() => {
    setActiveProject(null);
    setActiveFile(null);
    setShowTM(false);
    setShowTerms(false);
  }, []);

  const handleBackToProject = useCallback(() => {
    setActiveFile(null);
    setShowTM(false);
    setShowTerms(false);
  }, []);

  if (activeProject && showTerms) {
    return (
      <TermExplorer
        sourceLocale={activeProject.source_locale}
        targetLocales={activeProject.target_locales}
        projectName={activeProject.name}
        onBack={handleBackToProject}
      />
    );
  }

  if (activeProject && showTM) {
    return (
      <TMExplorer
        sourceLocale={activeProject.source_locale}
        targetLocales={activeProject.target_locales}
        onBack={handleBackToProject}
      />
    );
  }

  if (activeProject && activeFile) {
    return (
      <TranslationEditor
        project={activeProject}
        fileName={activeFile}
        onBack={handleBackToProject}
      />
    );
  }

  if (activeProject) {
    return (
      <ProjectView
        project={activeProject}
        onBack={handleBackToProjects}
        onOpenFile={(f) => setActiveFile(f)}
        onUploadFiles={handleUploadFiles}
        onRemoveFile={handleRemoveFile}
        onOpenTM={() => { setShowTM(true); setShowTerms(false); }}
        onOpenTerms={() => { setShowTerms(true); setShowTM(false); }}
      />
    );
  }

  return (
    <ProjectDashboard
      projects={projects}
      onCreateProject={handleCreateProject}
      onOpenProject={handleOpenProject}
    />
  );
}

// ---------------------------------------------------------------------------
// Termbase view (workspace-scoped)
// ---------------------------------------------------------------------------

function TermbaseView() {
  const { activeWorkspace } = useWorkspace();
  if (!activeWorkspace) return <EmptyState message="Select a workspace" />;
  return (
    <TermExplorer
      sourceLocale=""
      targetLocales={[]}
      onBack={() => {}}
    />
  );
}

// ---------------------------------------------------------------------------
// Translation Memory view (workspace-scoped)
// ---------------------------------------------------------------------------

function MemoryView() {
  const { activeWorkspace } = useWorkspace();
  if (!activeWorkspace) return <EmptyState message="Select a workspace" />;
  return (
    <TMExplorer
      sourceLocale=""
      targetLocales={[]}
      onBack={() => {}}
    />
  );
}

function VersionInfo() {
  const adapter = useApi();
  const [serverInfo, setServerInfo] = useState<ConfigResponse | null>(null);
  const [webInfo, setWebInfo] = useState<WebVersionInfo | null>(null);

  useEffect(() => {
    adapter.getConfig().then(setServerInfo).catch(() => {});
    fetch("/version.json")
      .then((r) => (r.ok ? r.json() : null))
      .then(setWebInfo)
      .catch(() => {});
  }, [adapter]);

  if (!serverInfo && !webInfo) return null;

  return (
    <>
      <div className="mt-6 mb-2">
        <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Version</div>
      </div>
      <div className="grid gap-4">
        {serverInfo && (
          <>
            <SettingsField label="Server Version" value={serverInfo.version} />
            <SettingsField label="Server Commit" value={serverInfo.commit} />
            <SettingsField label="Server Build Date" value={serverInfo.build_date} />
          </>
        )}
        {webInfo && (
          <>
            <SettingsField label="Web Version" value={webInfo.version} />
            <SettingsField label="Web Commit" value={webInfo.commit} />
            <SettingsField label="Web Build Date" value={webInfo.build_date} />
          </>
        )}
      </div>
    </>
  );
}

function SettingsView({ workspace }: { workspace: Workspace | null }) {
  if (!workspace) return <EmptyState message="Select a workspace" />;
  return (
    <div>
      <ViewHeader title="Settings" description="Workspace configuration" />
      <div className="mt-4 grid gap-4 max-w-[480px]">
        <SettingsField label="Name" value={workspace.name} />
        <SettingsField label="Slug" value={workspace.slug} />
        <SettingsField label="Description" value={workspace.description || "No description"} />
        <SettingsField label="Your Role" value={workspace.role} />
        <VersionInfo />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Shared UI helpers
// ---------------------------------------------------------------------------

function ViewHeader({ title, description }: { title: string; description: string }) {
  return (
    <div className="mb-2">
      <h2 className="text-xl font-semibold">{title}</h2>
      <p className="mt-1 text-[13px] text-muted-foreground">{description}</p>
    </div>
  );
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="mt-8 p-8 text-center text-muted-foreground text-sm rounded-lg border border-dashed border-border">
      {message}
    </div>
  );
}

function SettingsField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">{label}</div>
      <div className="text-sm text-foreground mt-1">{value}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main content area
// ---------------------------------------------------------------------------

function ViewContent({ activeView, workspace }: { activeView: View; workspace: Workspace | null }) {
  switch (activeView) {
    case "translate":
      return <TranslateView />;
    case "termbase":
      return <TermbaseView />;
    case "memory":
      return <MemoryView />;
    case "settings":
      return <SettingsView workspace={workspace} />;
    default:
      return <EmptyState message={`Unknown view: ${activeView}`} />;
  }
}

// ---------------------------------------------------------------------------
// App shell
// ---------------------------------------------------------------------------

function AppContent() {
  const { user, setUser } = useAuth();
  const { workspaces, setWorkspaces, activeWorkspace, setActiveWorkspace } = useWorkspace();
  const [activeView, setActiveView] = useState<View>("translate");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [serverMode, setServerMode] = useState<"standalone" | "server" | null>(null);
  const [loading, setLoading] = useState(true);
  const [showCreateWs, setShowCreateWs] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await api.getConfig();
        setServerMode(config.mode);

        if (config.mode === "standalone") {
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          setWorkspaces([{ id: "local", name: "Local", slug: "local", description: "", logo_url: "", type: "personal", role: "owner" }]);
          setActiveWorkspace({ id: "local", name: "Local", slug: "local", description: "", logo_url: "", type: "personal", role: "owner" });
        } else {
          const currentUser = await api.getCurrentUser();
          if (currentUser) {
            setUser(currentUser);
            const ws = await api.listWorkspaces();
            setWorkspaces(ws);
            if (ws.length > 0) {
              setActiveWorkspace(ws[0]);
            }
          }
        }
      } catch (e) {
        console.error("Failed to initialize:", e);
      } finally {
        setLoading(false);
      }
    })();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const handleSignOut = useCallback(() => {
    api.setToken("");
    setUser(null);
    setWorkspaces([]);
    setActiveWorkspace(null);
  }, [setUser, setWorkspaces, setActiveWorkspace]);

  const handleWorkspaceCreated = useCallback(
    (ws: Workspace) => {
      setWorkspaces([...workspaces, ws]);
      setActiveWorkspace(ws);
      setShowCreateWs(false);
    },
    [workspaces, setWorkspaces, setActiveWorkspace]
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-background text-muted-foreground">
        Loading...
      </div>
    );
  }

  if (serverMode === "server" && !user) {
    return <LoginPage />;
  }

  const isEditor = activeView === "translate";

  return (
    <>
      <AppShell
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        onSelectWorkspace={(ws: Workspace) => setActiveWorkspace(ws)}
        onCreateWorkspace={serverMode === "server" ? () => setShowCreateWs(true) : undefined}
        activeView={activeView}
        onViewChange={setActiveView}
        user={user}
        onSignOut={serverMode === "server" ? handleSignOut : undefined}
        collapsed={sidebarCollapsed}
        onCollapsedChange={setSidebarCollapsed}
        contentClassName={isEditor ? "overflow-hidden" : "overflow-auto"}
      >
        <ViewContent activeView={activeView} workspace={activeWorkspace} />
      </AppShell>

      <CreateWorkspaceDialog
        open={showCreateWs}
        onOpenChange={setShowCreateWs}
        onCreate={handleWorkspaceCreated}
      />
    </>
  );
}

export function App() {
  return (
    <ThemeProvider>
      <ApiProvider adapter={api}>
        <AuthProvider>
          <WorkspaceProvider>
            <AppContent />
          </WorkspaceProvider>
        </AuthProvider>
      </ApiProvider>
    </ThemeProvider>
  );
}
