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
  type User,
  type ConfigResponse,
} from "@gokapi/ui";
import { LoginPage } from "./auth/LoginPage";

const api = new RestApiAdapter();

function AppContent() {
  const { user, setUser } = useAuth();
  const { workspaces, setWorkspaces, activeWorkspace, setActiveWorkspace } = useWorkspace();
  const [activeView, setActiveView] = useState<View>("translate");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [serverMode, setServerMode] = useState<"local" | "server" | null>(null);
  const [loading, setLoading] = useState(true);

  // Detect server mode and authenticate
  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await api.getConfig();
        setServerMode(config.mode);

        if (config.mode === "local") {
          // Local mode: no auth needed, create a synthetic user
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          setWorkspaces([{ id: "local", name: "Local", slug: "local", description: "", logo_url: "", role: "owner" }]);
          setActiveWorkspace({ id: "local", name: "Local", slug: "local", description: "", logo_url: "", role: "owner" });
        } else {
          // Server mode: check if we have a token
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
    setUser(null);
    setWorkspaces([]);
    setActiveWorkspace(null);
  }, [setUser, setWorkspaces, setActiveWorkspace]);

  const handleCreateWorkspace = useCallback(() => {
    // TODO: show create workspace dialog
  }, []);

  if (loading) {
    return (
      <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100vh", color: "var(--text-secondary)" }}>
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
      <div style={{ display: "flex", height: "100vh", overflow: "hidden" }}>
        <MainSidebar
          workspace={activeWorkspace}
          activeView={activeView}
          onViewChange={setActiveView}
          collapsed={sidebarCollapsed}
          onCollapsedChange={setSidebarCollapsed}
        />
        <main style={{ flex: 1, padding: 24, overflow: "auto", color: "var(--text-secondary)" }}>
          <h2>gokapi — Local Project Server</h2>
          <p style={{ marginTop: 12 }}>Active view: {activeView}</p>
        </main>
      </div>
    );
  }

  // Server mode: full layout with workspace rail
  return (
    <div style={{ display: "flex", height: "100vh", overflow: "hidden" }}>
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
      <main style={{ flex: 1, padding: 24, overflow: "auto", color: "var(--text-secondary)" }}>
        <h2>{activeWorkspace?.name || "No workspace"}</h2>
        <p style={{ marginTop: 12 }}>Active view: {activeView}</p>
      </main>
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
