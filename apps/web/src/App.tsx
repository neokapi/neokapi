import { useState, useEffect, useCallback } from "react";
import {
  WorkspaceRail,
  MainSidebar,
  AuthProvider,
  WorkspaceProvider,
  ApiProvider,
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

  // Load projects when workspace changes.
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

  // TM/Term explorer within project context
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

  // Translation editor
  if (activeProject && activeFile) {
    return (
      <TranslationEditor
        project={activeProject}
        fileName={activeFile}
        onBack={handleBackToProject}
      />
    );
  }

  // Project detail view
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

  // Project dashboard
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

function SettingsView({ workspace }: { workspace: Workspace | null }) {
  if (!workspace) return <EmptyState message="Select a workspace" />;
  return (
    <div>
      <ViewHeader title="Settings" description="Workspace configuration" />
      <div style={{ marginTop: 16, display: "grid", gap: 16, maxWidth: 480 }}>
        <SettingsField label="Name" value={workspace.name} />
        <SettingsField label="Slug" value={workspace.slug} />
        <SettingsField label="Description" value={workspace.description || "No description"} />
        <SettingsField label="Your Role" value={workspace.role} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Shared UI helpers
// ---------------------------------------------------------------------------

function ViewHeader({ title, description }: { title: string; description: string }) {
  return (
    <div style={{ marginBottom: 8 }}>
      <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "#e6edf3" }}>{title}</h2>
      <p style={{ margin: "4px 0 0", fontSize: 13, color: "#8b949e" }}>{description}</p>
    </div>
  );
}

function EmptyState({ message }: { message: string }) {
  return (
    <div
      style={{
        marginTop: 32,
        padding: 32,
        textAlign: "center",
        color: "#484f58",
        fontSize: 14,
        borderRadius: 8,
        border: "1px dashed #30363d",
      }}
    >
      {message}
    </div>
  );
}

function SettingsField({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div style={{ fontSize: 12, fontWeight: 600, color: "#8b949e", textTransform: "uppercase", letterSpacing: 0.5 }}>{label}</div>
      <div style={{ fontSize: 14, color: "#e6edf3", marginTop: 4 }}>{value}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Create workspace dialog
// ---------------------------------------------------------------------------

function CreateWorkspaceDialog({
  onClose,
  onCreate,
}: {
  onClose: () => void;
  onCreate: (ws: Workspace) => void;
}) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const handleNameChange = (value: string) => {
    setName(value);
    setSlug(
      value
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-|-$/g, "")
    );
  };

  const handleCreate = async () => {
    if (!name.trim() || !slug.trim()) return;
    setCreating(true);
    setError("");
    try {
      const ws = await api.createWorkspace(name.trim(), slug.trim());
      onCreate(ws);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to create workspace");
    } finally {
      setCreating(false);
    }
  };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "rgba(0,0,0,0.6)",
        zIndex: 1000,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: "#161b22",
          borderRadius: 12,
          border: "1px solid #30363d",
          padding: 32,
          minWidth: 400,
          maxWidth: 480,
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <h3 style={{ margin: "0 0 20px", fontSize: 18, fontWeight: 600, color: "#e6edf3" }}>Create Workspace</h3>

        <label style={{ fontSize: 13, fontWeight: 600, color: "#8b949e" }}>Name</label>
        <input
          value={name}
          onChange={(e) => handleNameChange(e.target.value)}
          placeholder="My Workspace"
          autoFocus
          style={{
            display: "block",
            width: "100%",
            padding: "8px 12px",
            marginTop: 4,
            marginBottom: 16,
            fontSize: 14,
            background: "#0d1117",
            border: "1px solid #30363d",
            borderRadius: 6,
            color: "#e6edf3",
            outline: "none",
            boxSizing: "border-box",
          }}
        />

        <label style={{ fontSize: 13, fontWeight: 600, color: "#8b949e" }}>Slug</label>
        <input
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          placeholder="my-workspace"
          style={{
            display: "block",
            width: "100%",
            padding: "8px 12px",
            marginTop: 4,
            marginBottom: 16,
            fontSize: 14,
            background: "#0d1117",
            border: "1px solid #30363d",
            borderRadius: 6,
            color: "#e6edf3",
            outline: "none",
            boxSizing: "border-box",
          }}
        />

        {error && <div style={{ color: "#f85149", fontSize: 13, marginBottom: 12 }}>{error}</div>}

        <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
          <button
            onClick={onClose}
            style={{
              padding: "8px 16px",
              fontSize: 14,
              background: "transparent",
              color: "#8b949e",
              border: "1px solid #30363d",
              borderRadius: 6,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={creating || !name.trim() || !slug.trim()}
            style={{
              padding: "8px 16px",
              fontSize: 14,
              fontWeight: 600,
              background: creating ? "#1a7f37" : "#238636",
              color: "#fff",
              border: "none",
              borderRadius: 6,
              cursor: creating ? "default" : "pointer",
              opacity: !name.trim() || !slug.trim() ? 0.5 : 1,
            }}
          >
            {creating ? "Creating..." : "Create"}
          </button>
        </div>
      </div>
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
  const [serverMode, setServerMode] = useState<"local" | "server" | null>(null);
  const [loading, setLoading] = useState(true);
  const [showCreateWs, setShowCreateWs] = useState(false);

  // Check for token in URL (OIDC redirect callback sets /?token=...&user=...)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token");
    if (token) {
      api.setToken(token);
      window.history.replaceState({}, "", "/");
    }
  }, []);

  // Detect server mode and authenticate
  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await api.getConfig();
        setServerMode(config.mode);

        if (config.mode === "local") {
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          setWorkspaces([{ id: "local", name: "Local", slug: "local", description: "", logo_url: "", role: "owner" }]);
          setActiveWorkspace({ id: "local", name: "Local", slug: "local", description: "", logo_url: "", role: "owner" });
        } else {
          const currentUser = await api.getCurrentUser();
          if (currentUser) {
            setUser(currentUser);
            const ws = (await api.listWorkspaces()) || [];
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

  const handleCreateWorkspace = useCallback(() => {
    setShowCreateWs(true);
  }, []);

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
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100vh", background: "#0d1117", color: "#8b949e" }}>
        Loading...
      </div>
    );
  }

  // Server mode without auth: show login page
  if (serverMode === "server" && !user) {
    return <LoginPage />;
  }

  const isEditor = activeView === "translate";

  // Local mode: simplified layout (no workspace rail)
  if (serverMode === "local") {
    return (
      <div style={{ display: "flex", height: "100vh", overflow: "hidden", background: "#0d1117" }}>
        <MainSidebar
          workspace={activeWorkspace}
          activeView={activeView}
          onViewChange={setActiveView}
          collapsed={sidebarCollapsed}
          onCollapsedChange={setSidebarCollapsed}
        />
        <main style={{ flex: 1, padding: 24, overflow: isEditor ? "hidden" : "auto", display: "flex", flexDirection: "column", minHeight: 0 }}>
          <ViewContent activeView={activeView} workspace={activeWorkspace} />
        </main>
      </div>
    );
  }

  // Server mode: full layout with workspace rail
  return (
    <div style={{ display: "flex", height: "100vh", overflow: "hidden", background: "#0d1117" }}>
      <WorkspaceRail
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        onSelectWorkspace={(ws: Workspace) => setActiveWorkspace(ws)}
        onCreateWorkspace={handleCreateWorkspace}
        user={user}
        onAvatarClick={handleSignOut}
      />
      <MainSidebar
        workspace={activeWorkspace}
        activeView={activeView}
        onViewChange={setActiveView}
        collapsed={sidebarCollapsed}
        onCollapsedChange={setSidebarCollapsed}
      />
      <main style={{ flex: 1, padding: 24, overflow: isEditor ? "hidden" : "auto", display: "flex", flexDirection: "column", minHeight: 0 }}>
        <ViewContent activeView={activeView} workspace={activeWorkspace} />
      </main>

      {showCreateWs && <CreateWorkspaceDialog onClose={() => setShowCreateWs(false)} onCreate={handleWorkspaceCreated} />}
    </div>
  );
}

export function App() {
  return (
    <ApiProvider adapter={api}>
      <AuthProvider>
        <WorkspaceProvider>
          <AppContent />
        </WorkspaceProvider>
      </AuthProvider>
    </ApiProvider>
  );
}
