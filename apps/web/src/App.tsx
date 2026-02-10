import { useState, useEffect, useCallback } from "react";
import {
  WorkspaceRail,
  MainSidebar,
  AuthProvider,
  WorkspaceProvider,
  useAuth,
  useWorkspace,
  RestApiAdapter,
  type View,
  type Workspace,
  type ProjectInfo,
  type ConfigResponse,
} from "@gokapi/ui";
import { LoginPage } from "./auth/LoginPage";

const api = new RestApiAdapter();

// ---------------------------------------------------------------------------
// View panels
// ---------------------------------------------------------------------------

function TranslateView({ workspace }: { workspace: Workspace | null }) {
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!workspace) return;
    setLoading(true);
    api
      .listProjects(workspace.slug)
      .then(setProjects)
      .catch(() => setProjects([]))
      .finally(() => setLoading(false));
  }, [workspace]);

  if (!workspace) return <EmptyState message="Select a workspace" />;
  if (loading) return <EmptyState message="Loading projects..." />;

  return (
    <div>
      <ViewHeader title="Projects" description={`${projects.length} project${projects.length !== 1 ? "s" : ""} in ${workspace.name}`} />
      {projects.length === 0 ? (
        <EmptyState message="No projects yet. Create one to get started." />
      ) : (
        <div style={{ display: "grid", gap: 12, marginTop: 16 }}>
          {projects.map((p) => (
            <div
              key={p.id}
              style={{
                padding: 16,
                borderRadius: 8,
                border: "1px solid #30363d",
                background: "#161b22",
              }}
            >
              <div style={{ fontWeight: 600, fontSize: 15, color: "#e6edf3" }}>{p.name}</div>
              <div style={{ fontSize: 13, color: "#8b949e", marginTop: 4 }}>
                {p.source_locale} &rarr; {p.target_locales?.join(", ") || "none"}
              </div>
              <div style={{ fontSize: 12, color: "#484f58", marginTop: 4 }}>
                {p.items?.length ?? 0} files &middot; Created {new Date(p.created_at).toLocaleDateString()}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function TermbaseView() {
  return (
    <div>
      <ViewHeader title="Termbase" description="Manage terminology across your projects" />
      <EmptyState message="No termbases configured. Add terminology to enforce consistent translations." />
    </div>
  );
}

function MemoryView() {
  return (
    <div>
      <ViewHeader title="Translation Memory" description="Reuse previous translations" />
      <EmptyState message="No translation memories loaded. Import a TMX file or translate content to build memory." />
    </div>
  );
}

function FlowsView() {
  return (
    <div>
      <ViewHeader title="Flows" description="Automated processing pipelines" />
      <EmptyState message="No flows configured. Create a flow to automate document processing." />
    </div>
  );
}

function ConnectorsView() {
  return (
    <div>
      <ViewHeader title="Connectors" description="External service integrations" />
      <EmptyState message="No connectors configured. Add a connector to sync content with external services." />
    </div>
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
    // Auto-generate slug from name
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
// Main content area — renders view for the active navigation item
// ---------------------------------------------------------------------------

function ViewContent({ activeView, workspace }: { activeView: View; workspace: Workspace | null }) {
  switch (activeView) {
    case "translate":
      return <TranslateView workspace={workspace} />;
    case "termbase":
      return <TermbaseView />;
    case "memory":
      return <MemoryView />;
    case "flows":
      return <FlowsView />;
    case "connectors":
      return <ConnectorsView />;
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
      // Clean URL
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
        <main style={{ flex: 1, padding: 24, overflow: "auto" }}>
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
      <main style={{ flex: 1, padding: 24, overflow: "auto" }}>
        <ViewContent activeView={activeView} workspace={activeWorkspace} />
      </main>

      {showCreateWs && <CreateWorkspaceDialog onClose={() => setShowCreateWs(false)} onCreate={handleWorkspaceCreated} />}
    </div>
  );
}

export function App() {
  return (
    <AuthProvider>
      <WorkspaceProvider>
        <AppContent />
      </WorkspaceProvider>
    </AuthProvider>
  );
}
