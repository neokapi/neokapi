import { useState, useCallback } from "react";
import { Outlet, useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  AnimatedBackgroundGlass,
  AppSidebar,
  TopBar,
  AuthProvider,
  WorkspaceProvider,
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
} from "@gokapi/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useUIStore } from "../stores/ui-store";
import { viewFromPath } from "./view-from-path";
import type { WorkspaceRouteContext } from ".";

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
  const adapter = useApi();

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
      const ws = await adapter.createWorkspace(name.trim(), slug.trim());
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
  const queryClient = useQueryClient();

  // Data from route beforeLoad — already fetched, no loading state needed.
  const { serverMode, user, workspaces, activeWorkspace } =
    useRouteContext({ strict: false }) as WorkspaceRouteContext;

  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useUIStore((s) => s.setSidebarCollapsed);
  const setLastWorkspaceSlug = useUIStore((s) => s.setLastWorkspaceSlug);

  const [showCreateWs, setShowCreateWs] = useState(false);
  const [signedOut, setSignedOut] = useState(false);

  // Derive activeView from current URL for sidebar highlighting.
  const activeView = viewFromPath(
    window.location.pathname,
    workspaceSlug ?? "",
  );

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleSignOut = useCallback(async () => {
    try {
      await fetch("/api/v1/auth/logout", { method: "POST", credentials: "same-origin" });
    } catch {
      // Best-effort
    }
    queryClient.clear();
    setSignedOut(true);
  }, [queryClient]);

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
      setLastWorkspaceSlug(ws.slug);
      navigate({ to: "/$workspace", params: { workspace: ws.slug } });
    },
    [navigate, setLastWorkspaceSlug],
  );

  const handleWorkspaceCreated = useCallback(
    (ws: Workspace) => {
      setLastWorkspaceSlug(ws.slug);
      setShowCreateWs(false);
      queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      navigate({ to: "/$workspace", params: { workspace: ws.slug } });
    },
    [setLastWorkspaceSlug, navigate, queryClient],
  );

  // -----------------------------------------------------------------------
  // Render
  // -----------------------------------------------------------------------

  // After sign-out: show signed-out card or redirect to OIDC.
  if (signedOut) {
    if (serverMode === "server") {
      return (
        <>
          <AnimatedBackgroundGlass />
          <div className="relative z-10 flex items-center justify-center h-screen flex-col gap-6 text-foreground">
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
        </>
      );
    }
    // Standalone: shouldn't happen, but reload to recover.
    window.location.href = "/";
    return null;
  }

  const isEditor = activeView === "translate";

  return (
    <AuthProvider initialUser={user}>
      <WorkspaceProvider
        key={activeWorkspace.slug}
        initialWorkspace={activeWorkspace}
        initialWorkspaces={workspaces}
      >
        <AnimatedBackgroundGlass />
        <div className="relative z-10 flex h-screen overflow-hidden">
          <AppSidebar
            workspaces={workspaces}
            activeWorkspace={activeWorkspace}
            onSelectWorkspace={handleSelectWorkspace}
            onCreateWorkspace={serverMode === "server" ? () => setShowCreateWs(true) : undefined}
            activeView={activeView}
            onViewChange={handleViewChange}
            user={user}
            collapsed={sidebarCollapsed}
            onCollapsedChange={setSidebarCollapsed}
            collapsedWidth={60}
            showThemeToggle={false}
          />
          <div className="flex-1 flex flex-col min-h-0">
            <TopBar user={user} onSignOut={serverMode === "server" ? handleSignOut : undefined} />
            <main className={cn("flex-1 p-6 flex flex-col min-h-0", isEditor ? "overflow-hidden" : "overflow-auto")}>
              <Outlet />
            </main>
          </div>

          {showCreateWs && (
            <CreateWorkspaceDialog
              onClose={() => setShowCreateWs(false)}
              onCreate={handleWorkspaceCreated}
            />
          )}
        </div>
      </WorkspaceProvider>
    </AuthProvider>
  );
}
