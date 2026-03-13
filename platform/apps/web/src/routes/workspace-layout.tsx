import { useState, useCallback } from "react";
import { Outlet, useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  AppShell,
  TopBar,
  AuthProvider,
  WorkspaceProvider,
  StreamProvider,
  CreateWorkspaceDialog,
  AnimatedBackgroundGlass,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  type View,
  type Workspace,
} from "@neokapi/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useUIStore } from "../stores/ui-store";
import { viewFromPath } from "./view-from-path";
import type { WorkspaceRouteContext } from ".";

// ---------------------------------------------------------------------------
// Workspace layout shell
// ---------------------------------------------------------------------------

export function WorkspaceLayout() {
  const navigate = useNavigate();
  const { workspace: workspaceSlug, stream } = useParams({ strict: false });
  const queryClient = useQueryClient();

  // Data from route beforeLoad — already fetched, no loading state needed.
  const { serverMode, user, workspaces, activeWorkspace } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useUIStore((s) => s.setSidebarCollapsed);
  const setLastWorkspaceSlug = useUIStore((s) => s.setLastWorkspaceSlug);

  // Stream from URL path param (e.g. /$workspace/project/$pid/stream/$stream/...).
  // Falls back to "main" when on non-project routes (dashboard, settings, etc.).
  const currentStream = (stream as string | undefined) || "main";

  const handleStreamChange = useCallback(
    (newStream: string) => {
      // Replace the stream segment in the current URL.
      const path = window.location.pathname;
      const streamPattern = /\/stream\/[^/]+/;
      if (streamPattern.test(path)) {
        const newPath = path.replace(streamPattern, `/stream/${encodeURIComponent(newStream)}`);
        void navigate({ to: newPath as string, replace: true } as Parameters<typeof navigate>[0]);
      }
    },
    [navigate, workspaceSlug],
  );

  const [showCreateWs, setShowCreateWs] = useState(false);
  const [signedOut, setSignedOut] = useState(false);

  // Derive activeView from current URL for sidebar highlighting.
  const activeView = viewFromPath(window.location.pathname, workspaceSlug ?? "");

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleSignOut = useCallback(async () => {
    try {
      const resp = await fetch("/api/v1/auth/logout", {
        method: "POST",
        credentials: "same-origin",
      });
      if (resp.ok) {
        const data = await resp.json();
        // If the server returned an OIDC end_session_url, redirect the browser
        // to Keycloak to terminate the SSO session. Keycloak redirects back to
        // our origin after logout.
        if (data.end_session_url) {
          const endSessionUrl = new URL(data.end_session_url);
          endSessionUrl.searchParams.set("post_logout_redirect_uri", window.location.origin + "/");
          queryClient.clear();
          window.location.href = endSessionUrl.toString();
          return;
        }
      }
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
          void navigate({ to: "/$workspace", params: { workspace: ws } });
          break;
        case "termbase":
          void navigate({ to: "/$workspace/termbase", params: { workspace: ws } });
          break;
        case "memory":
          void navigate({ to: "/$workspace/memory", params: { workspace: ws } });
          break;
        case "settings":
          void navigate({ to: "/$workspace/settings", params: { workspace: ws } });
          break;
      }
    },
    [navigate, workspaceSlug],
  );

  const handleSelectWorkspace = useCallback(
    (ws: Workspace) => {
      setLastWorkspaceSlug(ws.slug);
      void navigate({ to: "/$workspace", params: { workspace: ws.slug } });
    },
    [navigate, setLastWorkspaceSlug],
  );

  const handleWorkspaceCreated = useCallback(
    async (ws: Workspace) => {
      setLastWorkspaceSlug(ws.slug);
      setShowCreateWs(false);
      await queryClient.refetchQueries({ queryKey: ["workspaces"] });
      void navigate({ to: "/$workspace", params: { workspace: ws.slug } });
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
                  onClick={() => {
                    window.location.href = "/api/v1/auth/login";
                  }}
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
        <AppShell
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
          showThemeToggle={false}
          headerSlot={
            <TopBar user={user} onSignOut={serverMode === "server" ? handleSignOut : undefined} />
          }
          contentClassName={isEditor ? "overflow-hidden" : "overflow-auto"}
        >
          <StreamProvider initialStream={currentStream} onStreamChange={handleStreamChange}>
            <Outlet />
          </StreamProvider>
        </AppShell>

        <CreateWorkspaceDialog
          open={showCreateWs}
          onOpenChange={setShowCreateWs}
          onCreate={handleWorkspaceCreated}
        />
      </WorkspaceProvider>
    </AuthProvider>
  );
}
