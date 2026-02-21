import { useState, useEffect, useCallback } from "react";
import { Outlet, useNavigate, useParams } from "@tanstack/react-router";
import {
  AppSidebar,
  useAuth,
  useWorkspace,
  useApi,
  cn,
  Button,
  Input,
  Label,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  type View,
  type Workspace,
  type ConfigResponse,
} from "@gokapi/ui";
import { useUIStore } from "../stores/ui-store";
import { api } from "../api";

/** Map a URL sub-path to a sidebar View id. */
function viewFromPath(pathname: string, workspace: string): View {
  const after = pathname.slice(`/${workspace}`.length);
  if (after.startsWith("/termbase")) return "termbase";
  if (after.startsWith("/memory")) return "memory";
  if (after.startsWith("/settings")) return "settings";
  return "translate";
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
        .replace(/^-|-$/g, ""),
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
// Workspace layout shell
// ---------------------------------------------------------------------------

export function WorkspaceLayout() {
  const navigate = useNavigate();
  const { workspace: workspaceSlug } = useParams({ strict: false });
  const adapter = useApi();
  const { user, setUser } = useAuth();
  const { workspaces, setWorkspaces, activeWorkspace, setActiveWorkspace } = useWorkspace();

  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useUIStore((s) => s.setSidebarCollapsed);
  const setLastWorkspaceSlug = useUIStore((s) => s.setLastWorkspaceSlug);

  const [serverMode, setServerMode] = useState<"standalone" | "server" | null>(null);
  const [loading, setLoading] = useState(true);
  const [showCreateWs, setShowCreateWs] = useState(false);
  const [signedOut, setSignedOut] = useState(false);

  // Derive activeView from current URL for sidebar highlighting.
  const activeView = viewFromPath(
    window.location.pathname,
    workspaceSlug ?? "",
  );

  // -----------------------------------------------------------------------
  // Initialization: detect server mode, authenticate, resolve workspace
  // -----------------------------------------------------------------------
  useEffect(() => {
    (async () => {
      try {
        const config: ConfigResponse = await adapter.getConfig();
        setServerMode(config.mode);

        if (config.mode === "standalone") {
          setUser({ id: "local", email: "", name: "Local User", avatar_url: "" });
          const localWs: Workspace = {
            id: "local", name: "Local", slug: "local",
            description: "", logo_url: "", type: "personal", role: "owner",
          };
          setWorkspaces([localWs]);
          setActiveWorkspace(localWs);
        } else {
          const currentUser = await adapter.getCurrentUser();
          if (currentUser) {
            setUser(currentUser);
            const ws = (await adapter.listWorkspaces()) || [];
            setWorkspaces(ws);
            // Match workspace from URL slug.
            const match = ws.find((w) => w.slug === workspaceSlug);
            if (match) {
              setActiveWorkspace(match);
              setLastWorkspaceSlug(match.slug);
            } else if (ws.length > 0) {
              // URL workspace not found — redirect to first workspace.
              setActiveWorkspace(ws[0]);
              setLastWorkspaceSlug(ws[0].slug);
              navigate({ to: "/$workspace", params: { workspace: ws[0].slug }, replace: true });
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

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

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

  const handleViewChange = useCallback(
    (view: View) => {
      const ws = workspaceSlug ?? "";
      switch (view) {
        case "translate":
          navigate({ to: "/$workspace", params: { workspace: ws } });
          break;
        case "termbase":
          navigate({ to: "/$workspace/termbase", params: { workspace: ws } });
          break;
        case "memory":
          navigate({ to: "/$workspace/memory", params: { workspace: ws } });
          break;
        case "settings":
          navigate({ to: "/$workspace/settings", params: { workspace: ws } });
          break;
      }
    },
    [navigate, workspaceSlug],
  );

  const handleSelectWorkspace = useCallback(
    (ws: Workspace) => {
      setActiveWorkspace(ws);
      setLastWorkspaceSlug(ws.slug);
      navigate({ to: "/$workspace", params: { workspace: ws.slug } });
    },
    [navigate, setActiveWorkspace, setLastWorkspaceSlug],
  );

  const handleWorkspaceCreated = useCallback(
    (ws: Workspace) => {
      setWorkspaces([...workspaces, ws]);
      setActiveWorkspace(ws);
      setLastWorkspaceSlug(ws.slug);
      setShowCreateWs(false);
      navigate({ to: "/$workspace", params: { workspace: ws.slug } });
    },
    [workspaces, setWorkspaces, setActiveWorkspace, setLastWorkspaceSlug, navigate],
  );

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-background text-muted-foreground">
        Loading...
      </div>
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
        onSelectWorkspace={handleSelectWorkspace}
        onCreateWorkspace={serverMode === "server" ? () => setShowCreateWs(true) : undefined}
        activeView={activeView}
        onViewChange={handleViewChange}
        user={user}
        onSignOut={serverMode === "server" ? handleSignOut : undefined}
        collapsed={sidebarCollapsed}
        onCollapsedChange={setSidebarCollapsed}
      />
      <main className={cn("flex-1 p-6 flex flex-col min-h-0", isEditor ? "overflow-hidden" : "overflow-auto")}>
        <Outlet />
      </main>

      {showCreateWs && (
        <CreateWorkspaceDialog
          onClose={() => setShowCreateWs(false)}
          onCreate={handleWorkspaceCreated}
        />
      )}
    </div>
  );
}
