import { useState, useCallback, useMemo, useEffect, type ReactNode } from "react";
import { usePlatform } from "../platform";
import {
  Outlet,
  useNavigate,
  useParams,
  useRouteContext,
  useLocation,
} from "@tanstack/react-router";
import {
  AppShell,
  TopBar,
  StreamSelector,
  AuthProvider,
  WorkspaceProvider,
  StreamProvider,
  CreateWorkspaceDialog,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  useApi,
  type User,
  type View,
  type Workspace,
  type SidebarContext,
  type ProjectInfo,
  type StreamInfo,
  StreamActionsProvider,
  useStreamActions,
  BravoProvider,
  BravoSidebar,
  BravoPanelTrigger,
  useBravo,
  useBravoAssistantRuntime,
} from "@neokapi/ui";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useUIStore } from "../stores/ui-store";
import { subNavTarget } from "./sub-nav-targets";
import { viewFromPath, type WorkspaceView } from "./view-from-path";
import {
  activitiesQueryOptions,
  myTaskCountsQueryOptions,
  myTasksQueryOptions,
  pendingChangesetsQueryOptions,
} from "../queries";
import { useWorkspaceEvents } from "../hooks/useWorkspaceEvents";
import { useDesktopFreshness } from "../hooks/useDesktopFreshness";
import { useConnectivity } from "../hooks/useConnectivity";
import type { WorkspaceRouteContext } from ".";

// ---------------------------------------------------------------------------
// Helpers to extract project context from URL
// ---------------------------------------------------------------------------

/** The per-file editor surfaces, each of which trails the item name as a splat. */
const EDITOR_SURFACES = ["translate", "review", "pre-process"];

/** Parse project-level params from the current URL path. */
function parseProjectParams(pathname: string, workspaceSlug: string) {
  // Pattern: /$workspace/p/$projectId/s/$stream[/translate/<item name>]
  const prefix = `/${workspaceSlug}/p/`;
  if (!pathname.startsWith(prefix)) return null;

  const rest = pathname.slice(prefix.length);
  const parts = rest.split("/");
  // parts: [projectId, "s", streamName, ...]
  if (parts.length < 3 || parts[1] !== "s") return null;

  const projectId = decodeURIComponent(parts[0]);
  const stream = decodeURIComponent(parts[2]);
  let itemName: string | undefined;

  // The item name is everything after the surface segment: it is a path and
  // keeps its slashes, so it spans as many segments as it has parts. A bare
  // /review with nothing trailing is the project-level review session, not a
  // file, which is why this needs a segment beyond the surface.
  if (parts.length >= 5 && EDITOR_SURFACES.includes(parts[3])) {
    itemName = parts.slice(4).map(decodeURIComponent).join("/");
  }

  const isSource = parts.length >= 4 && parts[3] === "source";
  const isAutomations = parts.length >= 4 && parts[3] === "automations";
  const isRuns = parts.length >= 4 && parts[3] === "runs";
  const isConnectors = parts.length >= 4 && parts[3] === "connectors";

  return { projectId, stream, itemName, isSource, isAutomations, isRuns, isConnectors };
}

// ---------------------------------------------------------------------------
// Workspace layout shell
// ---------------------------------------------------------------------------

/** Top bar with activity/task indicators — fetches data via React Query. */
function ConnectedTopBar({
  user,
  onSignOut,
  onSettings,
  workspaceSlug,
  leftSlot,
  beforeAvatarSlot,
  onViewAllActivities,
  onViewAllTasks,
  onTaskClick,
  onCompleteTask,
}: {
  user: User | null;
  onSignOut?: () => void;
  onSettings?: () => void;
  workspaceSlug: string;
  leftSlot?: React.ReactNode;
  beforeAvatarSlot?: React.ReactNode;
  onViewAllActivities?: () => void;
  onViewAllTasks?: () => void;
  onTaskClick?: (task: import("@neokapi/ui").TaskInfo) => void;
  onCompleteTask?: (taskId: string) => void;
}) {
  const api = useApi();
  const queryClient = useQueryClient();
  const connectivity = useConnectivity();

  const { data: activitiesData } = useQuery(activitiesQueryOptions(api, workspaceSlug));
  const { data: myTasksData } = useQuery(myTasksQueryOptions(api, workspaceSlug));
  // The dropdown shows one page; the badge counts every open task assigned to
  // the caller, which only the server can answer.
  const { data: myTaskCounts } = useQuery(myTaskCountsQueryOptions(api, workspaceSlug));

  const markSeen = useCallback(() => {
    void api.markActivitiesSeen(workspaceSlug).then(() => {
      void queryClient.invalidateQueries({
        queryKey: ["activities", workspaceSlug],
      });
    });
  }, [api, workspaceSlug, queryClient]);

  return (
    <TopBar
      user={user}
      onSignOut={onSignOut}
      onSettings={onSettings}
      connectionState={connectivity.state}
      pendingChanges={connectivity.pendingChanges}
      failedChanges={connectivity.failedChanges}
      leftSlot={leftSlot}
      beforeAvatarSlot={beforeAvatarSlot}
      activities={activitiesData?.activities}
      newActivityCount={activitiesData?.new_count}
      myTasks={myTasksData?.tasks}
      myTaskCount={myTaskCounts?.total}
      onViewAllActivities={onViewAllActivities}
      onMarkActivitiesSeen={markSeen}
      onViewAllTasks={onViewAllTasks}
      onTaskClick={onTaskClick}
      onCompleteTask={onCompleteTask}
    />
  );
}

/** @bravo trigger button for the top bar. */
function ConnectedBravoTrigger() {
  const { state, actions } = useBravo();
  return <BravoPanelTrigger onClick={actions.togglePanel} active={state.panelOpen} />;
}

/**
 * Mounts BravoProvider only when @bravo is entitled for the workspace. @bravo is
 * dark by default (billing.FeatureBravo is false on every plan; epic 015): the
 * server returns `features.bravo` on the workspace, gated by plan + per-workspace
 * override. When off we render children without the provider so no @bravo network
 * calls or chrome exist; the trigger/panel are also withheld (they useBravo() and
 * would throw without the provider).
 */
function MaybeBravoProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  return enabled ? <BravoProvider>{children}</BravoProvider> : <>{children}</>;
}

/** @bravo chat panel — renders as an assistant-ui powered sidebar. */
function ConnectedBravoPanel() {
  const { state, actions } = useBravo();
  const runtime = useBravoAssistantRuntime();

  const [view, setView] = useState<"list" | "chat">(state.activeConversation ? "chat" : "list");

  // Switch to chat view when a conversation becomes active.
  useEffect(() => {
    if (state.activeConversation) setView("chat");
  }, [state.activeConversation]);

  return (
    <BravoSidebar
      open={state.panelOpen}
      onOpenChange={(open) => (open ? actions.openPanel() : actions.closePanel())}
      runtime={runtime}
      view={view}
      onBack={() => setView("list")}
      activeTitle={state.activeConversation?.title || "Conversation"}
      conversationListProps={{
        conversations: state.conversations,
        activeId: state.activeConversation?.id,
        onSelect: (conv) => {
          void actions.selectConversation(conv);
          setView("chat");
        },
        onDelete: (conv) => void actions.deleteConversation(conv),
        onNew: () => {
          void actions.newConversation();
          setView("chat");
        },
        loading: state.loading,
      }}
      coldStarting={state.coldStarting}
      mode={state.mode}
      onModeChange={actions.setMode}
      stepUp={state.stepUp}
      onStepUpSwitch={actions.handleModeSwitch}
      onStepUpDismiss={actions.dismissStepUp}
    />
  );
}

/** Stream selector in the top bar — reads action callbacks from StreamActionsContext. */
function TopBarStreamSelector({
  sidebarContext,
  onStreamChange,
}: {
  sidebarContext: Extract<SidebarContext, { level: "project" }>;
  onStreamChange: (name: string) => void;
}) {
  const { actions } = useStreamActions();
  const streams = sidebarContext.project.streams ?? [];

  return (
    <StreamSelector
      streams={streams}
      activeStream={streams.find((s: StreamInfo) => s.name === sidebarContext.activeStream) ?? null}
      defaultStream={sidebarContext.project.default_stream}
      onStreamChange={(s: StreamInfo) => onStreamChange(s.name)}
      onCreateStream={actions.onCreateStream}
      onEditStream={actions.onEditStream}
      onMergeStream={actions.onMergeStream}
      onDiffStream={actions.onDiffStream}
      onDeleteStream={actions.onDeleteStream}
    />
  );
}

export function WorkspaceLayout() {
  const navigate = useNavigate();
  const { workspace: workspaceSlug, stream } = useParams({ strict: false });
  const queryClient = useQueryClient();
  const adapter = useApi();
  const platform = usePlatform();

  // Governed changes waiting on a review. The Context sub-nav wears the count so
  // a workspace says it has something to approve before anyone opens the page it
  // is waiting on — the summons the founder never got.
  const { data: pendingChangesets } = useQuery({
    ...pendingChangesetsQueryOptions(adapter, workspaceSlug ?? ""),
    enabled: !!workspaceSlug,
  });

  // Data from route beforeLoad — already fetched, no loading state needed.
  const { serverMode, user, workspaces, activeWorkspace } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  useEffect(() => {
    if (user && user.id !== "local") {
      platform.analytics?.identify({ id: user.id, email: user.email, name: user.name });
    }
  }, [user, platform]);

  // Workspace group analytics (epic 018): scope subsequent events to the
  // active workspace so funnels can be cut per workspace. Standalone mode has
  // no real workspace identity, so it stays out of group analytics.
  useEffect(() => {
    if (serverMode === "server" && activeWorkspace?.id) {
      platform.analytics?.group?.("workspace", activeWorkspace.id);
    }
  }, [serverMode, activeWorkspace?.id, platform]);

  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const setSidebarCollapsed = useUIStore((s) => s.setSidebarCollapsed);
  const setLastWorkspaceSlug = useUIStore((s) => s.setLastWorkspaceSlug);

  // Stream from URL path param (e.g. /$workspace/p/$pid/s/$stream/...).
  // Falls back to "main" when on non-project routes (dashboard, settings, etc.).
  const currentStream = (stream as string | undefined) || "main";

  const handleStreamChange = useCallback(
    (newStream: string) => {
      // Replace the stream segment in the current URL.
      const path = pathname;
      const streamPattern = /\/s\/[^/]+/;
      if (streamPattern.test(path)) {
        const newPath = path.replace(streamPattern, `/s/${encodeURIComponent(newStream)}`);
        void navigate({ to: newPath as string, replace: true } as Parameters<typeof navigate>[0]);
      }
    },
    [navigate, workspaceSlug],
  );

  const [showCreateWs, setShowCreateWs] = useState(false);
  const [signedOut, setSignedOut] = useState(false);

  // Reactive pathname — triggers re-render on every navigation.
  const { pathname } = useLocation();

  // Derive activeView from current URL for sidebar highlighting.
  const activeView = viewFromPath(pathname, workspaceSlug ?? "");

  // Map auditlog and recycle bin to settings for sidebar highlighting (they're now sub-items of settings).
  const effectiveView =
    activeView === "auditlog" || activeView === "bin" ? ("settings" as const) : activeView;

  // Derive Context hub sub-nav from URL (Profiles · Concepts · Voice · Content
  // memory · Changes · Activity). Profile-detail, concept-story and
  // change-detail pages keep their section highlighted; bare /context falls
  // back to Profiles (the landing section). /context/dashboard is deliberately
  // absent: it resolves to the Insights view, so it never reaches this branch.
  const contextSubNav = useMemo(() => {
    if (activeView !== "context") return undefined;
    const contextPath = `/${workspaceSlug}/context`;
    const rest = pathname.slice(contextPath.length).replace(/^\//, "");
    if (rest.startsWith("concepts")) return "concepts";
    if (rest.startsWith("voice")) return "voice";
    if (rest.startsWith("memory")) return "memory";
    if (rest.startsWith("changes")) return "changes";
    if (rest.startsWith("activity")) return "activity";
    return "profiles";
  }, [activeView, pathname, workspaceSlug]);

  // Derive the insights sub-nav from the URL. Without this the section had no
  // highlight at all: activeSubNav fell through to settingsSubNav, which
  // returns undefined for any view that is not settings.
  const insightsSubNav = useMemo(() => {
    if (activeView !== "insights") return undefined;
    if (pathname.startsWith(`/${workspaceSlug}/locale-demand`)) return "locale-demand";
    return "dashboard";
  }, [activeView, pathname, workspaceSlug]);

  // Derive settings sub-nav from URL.
  const settingsSubNav = useMemo(() => {
    if (activeView === "auditlog") return "auditlog";
    if (activeView === "bin") return "bin";
    if (activeView !== "settings") return undefined;
    const settingsPath = `/${workspaceSlug}/settings`;
    const rest = pathname.slice(settingsPath.length).replace(/^\//, "");
    if (rest === "languages") return "languages";
    if (rest === "members") return "members";
    if (rest === "roles") return "roles";
    if (rest === "governance") return "governance";
    if (rest === "providers") return "providers";
    if (rest === "tokens") return "tokens";
    if (rest === "system") return "system";
    if (rest === "bravo") return "bravo";
    if (rest === "billing") return "billing";
    return "general";
  }, [activeView, pathname, workspaceSlug]);

  const handleSubNavChange = useCallback(
    (id: string) => {
      const to = subNavTarget(id);
      // An unknown id is a sidebar item with no destination. Returning rather
      // than navigating keeps it inert instead of throwing, and
      // sub-nav-targets.test.ts is what stops one existing in the first place.
      if (!to) return;
      void navigate({ to, params: { workspace: workspaceSlug ?? "" } });
    },
    [navigate, workspaceSlug],
  );
  // -----------------------------------------------------------------------
  // Sidebar context: determine from URL + query cache
  // -----------------------------------------------------------------------

  const ws = activeWorkspace.slug;

  // Subscribe to the workspace's unified change-event stream so every view
  // stays fresh when content changes from outside it (another user's edit, a
  // kapi push, a connector sync, an automation/flow completion, a
  // stream/member/brand/term change). Scope to the active project when the URL
  // is on a project route to reduce noise; otherwise stream the whole
  // workspace. Yjs collab WS keeps handling per-cursor presence.
  const activeProjectId = useMemo(
    () => parseProjectParams(pathname, workspaceSlug ?? "")?.projectId,
    [pathname, workspaceSlug],
  );
  // The change-event stream is a raw same-origin EventSource with cookie auth —
  // that only works in the browser. On the desktop the server is reached over
  // Wails with a keychain token, and freshness arrives via the backend-event
  // bridge instead, so pass no slug to keep the stream off (the hook early-
  // returns on an empty slug).
  useWorkspaceEvents(platform.kind === "web" ? ws : undefined, activeProjectId);
  // Desktop freshness: the Go SSE→Wails-events watcher, invalidating the same
  // caches. No-op on web (the hook gates on kind === "desktop").
  useDesktopFreshness(ws, activeProjectId);

  const sidebarContext = useMemo<SidebarContext | undefined>(() => {
    const projectParams = parseProjectParams(pathname, workspaceSlug ?? "");
    if (!projectParams) {
      // Workspace-level: use default flat nav. The cast covers web-only views
      // ("locale-demand") that the shared SidebarContext type doesn't know yet.
      return { level: "workspace", activeView: effectiveView as View };
    }

    // Try to read project from React Query cache (populated by child route loaders)
    const project = queryClient.getQueryData<ProjectInfo>([
      "project",
      ws,
      projectParams.projectId,
      projectParams.stream,
    ]);

    if (!project) {
      // Project data not yet in cache — fall back to workspace nav.
      // This can happen briefly during navigation; child loader will populate it.
      return { level: "workspace", activeView: effectiveView as View };
    }

    // Determine which project sub-page is active.
    const activeProjectView = projectParams.isSource
      ? ("source" as const)
      : projectParams.isAutomations
        ? ("automations" as const)
        : projectParams.isRuns
          ? ("runs" as const)
          : projectParams.isConnectors
            ? ("connectors" as const)
            : ("dashboard" as const);

    return {
      level: "project",
      project,
      activeStream: projectParams.stream,
      activeProjectView,
      onBack:
        projectParams.itemName ||
        projectParams.isSource ||
        projectParams.isAutomations ||
        projectParams.isRuns ||
        projectParams.isConnectors
          ? () => {
              // A project sub-surface goes up to the project's overview.
              void navigate({
                to: "/$workspace/p/$projectId/s/$stream",
                params: {
                  workspace: workspaceSlug ?? ws,
                  projectId: project.id,
                  stream: projectParams.stream,
                },
              });
            }
          : () => {
              // The overview is the project's top level: up is the workspace.
              void navigate({
                to: "/$workspace",
                params: { workspace: workspaceSlug ?? ws },
              });
            },
      onOpenDashboard: () => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
          },
        });
      },
      onOpenSource: () => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/source",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
          },
        });
      },
      onOpenFile: (itemName: string) => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/translate/$",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
            _splat: itemName,
          },
        });
      },
      onStreamChange: handleStreamChange,
      onOpenAutomations: () => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/automations",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
          },
        });
      },
      onOpenRuns: () => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/runs",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
          },
        });
      },
      onOpenConnectors: () => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/connectors",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
          },
        });
      },
    };
  }, [pathname, workspaceSlug, stream, activeView, ws, queryClient, navigate, handleStreamChange]);

  // -----------------------------------------------------------------------
  // Handlers
  // -----------------------------------------------------------------------

  const handleSignOut = useCallback(async () => {
    // Desktop signs out at the host: the OIDC cookie/redirect round-trip below
    // is a browser flow with no meaning in the webview (Bearer-token auth). The
    // host clears the keychain token and disconnects; the connection gate then
    // returns to ServerConnect.
    if (platform.signOut) {
      queryClient.clear();
      await platform.signOut();
      return;
    }
    try {
      const resp = await fetch("/api/v1/auth/logout", {
        method: "POST",
        credentials: "same-origin",
        // Cookie-authenticated mutation: send the CSRF header the server
        // requires on the cookie auth path (this route bypasses RestApiAdapter).
        headers: { "X-Bowrain-Csrf": "1" },
      });
      if (resp.ok) {
        const data = await resp.json();
        // The server returns a fully-formed, provider-appropriate end_session_url
        // (Cognito or Keycloak); redirect to it verbatim to terminate the upstream
        // SSO session. The IdP returns the browser to our origin afterward.
        if (data.end_session_url) {
          queryClient.clear();
          window.location.href = data.end_session_url;
          return;
        }
      }
    } catch {
      // Best-effort
    }
    queryClient.clear();
    setSignedOut(true);
  }, [queryClient, platform]);

  const handleViewChange = useCallback(
    (view: WorkspaceView) => {
      const wsSlug = workspaceSlug ?? "";
      switch (view) {
        case "translate":
          void navigate({ to: "/$workspace", params: { workspace: wsSlug } });
          break;
        case "locale-demand":
          void navigate({
            to: "/$workspace/locale-demand",
            params: { workspace: wsSlug },
          });
          break;
        case "context":
          void navigate({
            to: "/$workspace/context",
            params: { workspace: wsSlug },
          });
          break;
        case "insights":
          void navigate({
            to: "/$workspace/context/dashboard",
            params: { workspace: wsSlug },
          });
          break;
        case "memory":
          void navigate({
            to: "/$workspace/context/memory",
            params: { workspace: wsSlug },
          });
          break;
        case "auditlog":
          void navigate({
            to: "/$workspace/auditlog",
            params: { workspace: wsSlug },
          });
          break;
        case "bin":
          void navigate({
            to: "/$workspace/bin",
            params: { workspace: wsSlug },
          });
          break;
        case "settings":
          void navigate({
            to: "/$workspace/settings",
            params: { workspace: wsSlug },
          });
          break;
      }
    },
    [navigate, workspaceSlug],
  );

  const handleSelectWorkspace = useCallback(
    (selectedWs: Workspace) => {
      setLastWorkspaceSlug(selectedWs.slug);
      void navigate({
        to: "/$workspace",
        params: { workspace: selectedWs.slug },
      });
    },
    [navigate, setLastWorkspaceSlug],
  );

  const handleWorkspaceCreated = useCallback(
    async (createdWs: Workspace) => {
      setLastWorkspaceSlug(createdWs.slug);
      setShowCreateWs(false);
      await queryClient.refetchQueries({ queryKey: ["workspaces"] });
      void navigate({
        to: "/$workspace",
        params: { workspace: createdWs.slug },
      });
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

  // Which routes manage their own scrolling, and therefore want the shell's
  // scroll container switched off.
  //
  // Only the full-height editor surfaces qualify — they lay out fixed panes and
  // scroll inside them. activeView === "translate" is the whole Projects
  // *section*, which also covers the dashboard and the project overview: those
  // are ordinary documents that grow past the viewport, and handing them
  // overflow-hidden left their lower content unreachable (no scrollbar, no
  // wheel, nothing) on any window short enough to matter. Match the route.
  const isSelfScrollingRoute = /\/(translate|review|pre-process)$/.test(pathname);

  // @bravo is dark by default (epic 015): only surface it when the server reports
  // the workspace is entitled (plan matrix + per-workspace override). It is also
  // web-only: @bravo streams over an EventSource-style SSE response, which the
  // desktop's request/response Wails proxy transport cannot carry (it buffers the
  // whole body and times out), so we withhold the chrome rather than render a
  // chat that never streams.
  const bravoEnabled = platform.kind === "web" && Boolean(activeWorkspace.features?.bravo);

  return (
    <AuthProvider initialUser={user}>
      <WorkspaceProvider
        key={activeWorkspace.slug}
        initialWorkspace={activeWorkspace}
        initialWorkspaces={workspaces}
      >
        <MaybeBravoProvider enabled={bravoEnabled}>
          <StreamActionsProvider>
            <AppShell
              workspaces={workspaces}
              activeWorkspace={activeWorkspace}
              onSelectWorkspace={handleSelectWorkspace}
              onCreateWorkspace={serverMode === "server" ? () => setShowCreateWs(true) : undefined}
              activeView={effectiveView}
              onViewChange={handleViewChange}
              hiddenSubNavIds={bravoEnabled ? undefined : ["bravo"]}
              subNavCounts={{ changes: pendingChangesets?.length ?? 0 }}
              user={user}
              onSignOut={serverMode === "server" ? handleSignOut : undefined}
              collapsed={sidebarCollapsed}
              onCollapsedChange={setSidebarCollapsed}
              showThemeToggle={false}
              sidebarContext={sidebarContext}
              activeSubNav={contextSubNav ?? insightsSubNav ?? settingsSubNav}
              onSubNavChange={handleSubNavChange}
              headerSlot={
                <ConnectedTopBar
                  user={user}
                  onSignOut={serverMode === "server" ? handleSignOut : undefined}
                  onSettings={
                    serverMode === "server"
                      ? () =>
                          void navigate({
                            to: "/$workspace/user-settings",
                            params: { workspace: ws },
                          })
                      : undefined
                  }
                  workspaceSlug={ws}
                  onViewAllActivities={() =>
                    void navigate({
                      to: "/$workspace/activities",
                      params: { workspace: ws },
                    })
                  }
                  onViewAllTasks={() =>
                    void navigate({
                      to: "/$workspace/tasks",
                      params: { workspace: ws },
                    })
                  }
                  onTaskClick={(task) => {
                    if (task.project_id) {
                      const taskStream = task.stream || "main";
                      void navigate({
                        to: "/$workspace/p/$projectId/s/$stream",
                        params: {
                          workspace: ws,
                          projectId: task.project_id,
                          stream: taskStream,
                        },
                      });
                    }
                  }}
                  onCompleteTask={async (taskId) => {
                    await adapter.completeTask(ws, taskId);
                    void queryClient.invalidateQueries({
                      queryKey: ["myTasks", ws],
                    });
                    void queryClient.invalidateQueries({
                      queryKey: ["tasks", ws],
                    });
                    void queryClient.invalidateQueries({
                      queryKey: ["myTaskCounts", ws],
                    });
                    void queryClient.invalidateQueries({
                      queryKey: ["taskCounts", ws],
                    });
                  }}
                  leftSlot={
                    sidebarContext?.level === "project" &&
                    sidebarContext.project.streams &&
                    sidebarContext.project.streams.length > 0 ? (
                      <TopBarStreamSelector
                        sidebarContext={sidebarContext}
                        onStreamChange={handleStreamChange}
                      />
                    ) : undefined
                  }
                  beforeAvatarSlot={bravoEnabled ? <ConnectedBravoTrigger /> : undefined}
                />
              }
              rightPanelSlot={bravoEnabled ? <ConnectedBravoPanel /> : undefined}
              contentClassName={isSelfScrollingRoute ? "overflow-hidden" : "overflow-auto"}
            >
              <StreamProvider initialStream={currentStream} onStreamChange={handleStreamChange}>
                <Outlet />
              </StreamProvider>
            </AppShell>
          </StreamActionsProvider>
        </MaybeBravoProvider>

        <CreateWorkspaceDialog
          open={showCreateWs}
          onOpenChange={setShowCreateWs}
          onCreate={handleWorkspaceCreated}
        />
      </WorkspaceProvider>
    </AuthProvider>
  );
}
