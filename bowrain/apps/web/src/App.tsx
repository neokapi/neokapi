import { useState, useEffect, useCallback } from "react";
import {
  AppSidebar,
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
  ThemeProvider,
  InviteManager,
  cn,
  Button,
  Input,
  Label,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@gokapi/ui";
import { JoinPage } from "./auth/JoinPage";
import { ClaimPage } from "./auth/ClaimPage";
import { DeviceVerifyPage } from "./auth/DeviceVerifyPage";
import { DeviceAuthorizedPage } from "./auth/DeviceAuthorizedPage";

const api = new RestApiAdapter();

// When both access and refresh tokens are invalid, redirect to OIDC.
// Skip redirect for /claim/ and /join/ routes — those pages handle auth themselves.
api.onSessionExpired = () => {
  const path = window.location.pathname;
  if (path.startsWith("/claim/") || path.startsWith("/join/") || path.startsWith("/device/")) {
    return;
  }
  window.location.href = "/api/v1/auth/login";
};

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
        serverMode={ws ? { serverURL: window.location.origin, workspaceSlug: ws } : undefined}
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
      <div className="mt-4 grid gap-4 max-w-[480px]">
        <SettingsField label="Name" value={workspace.name} />
        <SettingsField label="Slug" value={workspace.slug} />
        <SettingsField label="Description" value={workspace.description || "No description"} />
        <SettingsField label="Your Role" value={workspace.role} />
      </div>
      <div className="max-w-[480px]">
        <InviteManager workspace={workspace} />
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
      className="fixed inset-0 flex items-center justify-center bg-black/50 z-[1000]"
      onClick={onClose}
    >
      <div
        className="bg-card rounded-xl border border-border p-8 min-w-[400px] max-w-[480px]"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-semibold mb-5">Create Workspace</h3>

        <Label className="text-muted-foreground">Name</Label>
        <Input
          value={name}
          onChange={(e) => handleNameChange(e.target.value)}
          placeholder="My Workspace"
          autoFocus
          className="mt-1 mb-4"
        />

        <Label className="text-muted-foreground">Slug</Label>
        <Input
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          placeholder="my-workspace"
          className="mt-1 mb-4"
        />

        {error && <div className="text-destructive text-[13px] mb-3">{error}</div>}

        <div className="flex gap-2 justify-end">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            disabled={creating || !name.trim() || !slug.trim()}
          >
            {creating ? "Creating..." : "Create"}
          </Button>
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
  const [joinCode, setJoinCode] = useState<string | null>(null);
  const [claimToken, setClaimToken] = useState<string | null>(null);
  const [deviceVerify, setDeviceVerify] = useState(false);
  const [deviceAuthorized, setDeviceAuthorized] = useState(false);
  const [signedOut, setSignedOut] = useState(false);

  // Detect /join/:code and /claim/:token routes on mount.
  useEffect(() => {
    const joinMatch = window.location.pathname.match(/^\/join\/([^/]+)$/);
    if (joinMatch) {
      setJoinCode(joinMatch[1]);
    }
    const claimMatch = window.location.pathname.match(/^\/claim\/([^/]+)$/);
    if (claimMatch) {
      setClaimToken(claimMatch[1]);
    }
    if (window.location.pathname === "/device/verify") {
      setDeviceVerify(true);
    }
    if (window.location.pathname === "/device/authorized") {
      setDeviceAuthorized(true);
    }
  }, []);

  // Detect server mode and authenticate.
  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await api.getConfig();
        setServerMode(config.mode);

        if (config.mode === "local") {
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          setWorkspaces([{ id: "local", name: "Local", slug: "local", description: "", logo_url: "", type: "personal", role: "owner" }]);
          setActiveWorkspace({ id: "local", name: "Local", slug: "local", description: "", logo_url: "", type: "personal", role: "owner" });
        } else {
          // In server mode, cookies are sent automatically. Try to get current user.
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

  const handleSignOut = useCallback(async () => {
    try {
      await fetch("/api/v1/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // Best-effort
    }
    setUser(null);
    setWorkspaces([]);
    setActiveWorkspace(null);
    setSignedOut(true);
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
      <div className="flex items-center justify-center h-screen bg-background text-muted-foreground">
        Loading...
      </div>
    );
  }

  // Device auth routes: standalone pages, no authentication required.
  if (deviceVerify) return <DeviceVerifyPage />;
  if (deviceAuthorized) return <DeviceAuthorizedPage />;

  // Join route: show join page for invite acceptance.
  if (joinCode) {
    return (
      <JoinPage
        code={joinCode}
        onJoined={() => {
          setJoinCode(null);
          window.history.replaceState({}, "", "/");
        }}
      />
    );
  }

  // Claim route: show claim page for anonymous project claiming.
  if (claimToken) {
    return (
      <ClaimPage
        token={claimToken}
        onClaimed={() => {
          setClaimToken(null);
          window.history.replaceState({}, "", "/");
        }}
      />
    );
  }

  // Server mode without auth: show signed-out card or redirect to OIDC.
  if (serverMode === "server" && !user) {
    if (signedOut) {
      return (
        <div className="flex items-center justify-center h-screen flex-col gap-6 bg-background text-foreground">
          <Card className="min-w-[360px]">
            <CardHeader className="items-center text-center">
              <CardTitle className="text-xl font-semibold">Signed out</CardTitle>
              <p className="text-sm text-muted-foreground">
                You have been signed out successfully.
              </p>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Button
                onClick={() => { window.location.href = "/api/v1/auth/login"; }}
                className="w-full"
                size="lg"
              >
                Sign in again
              </Button>
            </CardContent>
          </Card>
        </div>
      );
    }

    // Auto-redirect to OIDC.
    window.location.href = "/api/v1/auth/login";
    return (
      <div className="flex items-center justify-center h-screen bg-background text-muted-foreground">
        Redirecting to sign in...
      </div>
    );
  }

  const isEditor = activeView === "translate";

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <AppSidebar
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        onSelectWorkspace={(ws: Workspace) => setActiveWorkspace(ws)}
        onCreateWorkspace={serverMode === "server" ? handleCreateWorkspace : undefined}
        activeView={activeView}
        onViewChange={setActiveView}
        user={user}
        onSignOut={serverMode === "server" ? handleSignOut : undefined}
        collapsed={sidebarCollapsed}
        onCollapsedChange={setSidebarCollapsed}
      />
      <main className={cn("flex-1 p-6 flex flex-col min-h-0", isEditor ? "overflow-hidden" : "overflow-auto")}>
        <ViewContent activeView={activeView} workspace={activeWorkspace} />
      </main>

      {showCreateWs && <CreateWorkspaceDialog onClose={() => setShowCreateWs(false)} onCreate={handleWorkspaceCreated} />}
    </div>
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
