import { useState, useCallback, useMemo, useEffect } from "react";
import { identifyUser } from "../posthog";
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
import { viewFromPath } from "./view-from-path";
import { activitiesQueryOptions, myTasksQueryOptions } from "../queries";
import { useWorkspaceEvents } from "../hooks/useWorkspaceEvents";
import type { WorkspaceRouteContext } from ".";

// ---------------------------------------------------------------------------
// Helpers to extract project context from URL
// ---------------------------------------------------------------------------

/** Parse project-level params from the current URL path. */
function parseProjectParams(pathname: string, workspaceSlug: string) {
  // Pattern: /$workspace/p/$projectId/s/$stream[/$itemId/translate]
  const prefix = `/${workspaceSlug}/p/`;
  if (!pathname.startsWith(prefix)) return null;

  const rest = pathname.slice(prefix.length);
  const parts = rest.split("/");
  // parts: [projectId, "s", streamName, ...]
  if (parts.length < 3 || parts[1] !== "s") return null;

  const projectId = decodeURIComponent(parts[0]);
  const stream = decodeURIComponent(parts[2]);
  let itemId: string | undefined;

  // Check for the per-file editor surfaces (/$itemId/{translate,review,pre-process}).
  if (parts.length >= 5 && ["translate", "review", "pre-process"].includes(parts[4])) {
    itemId = decodeURIComponent(parts[3]);
  }

  const isAutomations = parts.length >= 4 && parts[3] === "automations";

  return { projectId, stream, itemId, isAutomations };
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

  const { data: activitiesData } = useQuery(activitiesQueryOptions(api, workspaceSlug));
  const { data: myTasksData } = useQuery(myTasksQueryOptions(api, workspaceSlug));

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
      leftSlot={leftSlot}
      beforeAvatarSlot={beforeAvatarSlot}
      activities={activitiesData?.activities}
      newActivityCount={activitiesData?.new_count}
      myTasks={myTasksData?.tasks}
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

  // Data from route beforeLoad — already fetched, no loading state needed.
  const { serverMode, user, workspaces, activeWorkspace } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  useEffect(() => {
    if (user && user.id !== "local") {
      identifyUser(user.id, { email: user.email, name: user.name });
    }
  }, [user]);

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
      const wsSlug = workspaceSlug ?? "";
      switch (id) {
        case "general":
          void navigate({
            to: "/$workspace/settings",
            params: { workspace: wsSlug },
          });
          break;
        case "languages":
          void navigate({
            to: "/$workspace/settings/languages",
            params: { workspace: wsSlug },
          });
          break;
        case "members":
          void navigate({
            to: "/$workspace/settings/members",
            params: { workspace: wsSlug },
          });
          break;
        case "roles":
          void navigate({
            to: "/$workspace/settings/roles",
            params: { workspace: wsSlug },
          });
          break;
        case "governance":
          void navigate({
            to: "/$workspace/settings/governance",
            params: { workspace: wsSlug },
          });
          break;
        case "providers":
          void navigate({
            to: "/$workspace/settings/providers",
            params: { workspace: wsSlug },
          });
          break;
        case "tokens":
          void navigate({
            to: "/$workspace/settings/tokens",
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
        case "system":
          void navigate({
            to: "/$workspace/settings/system",
            params: { workspace: wsSlug },
          });
          break;
        case "bravo":
          void navigate({
            to: "/$workspace/settings/bravo",
            params: { workspace: wsSlug },
          });
          break;
        case "billing":
          void navigate({
            to: "/$workspace/settings/billing",
            params: { workspace: wsSlug },
          });
          break;
      }
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
  useWorkspaceEvents(ws, activeProjectId);

  const sidebarContext = useMemo<SidebarContext | undefined>(() => {
    const projectParams = parseProjectParams(pathname, workspaceSlug ?? "");
    if (!projectParams) {
      // Workspace-level: use default flat nav
      return { level: "workspace", activeView: effectiveView };
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
      return { level: "workspace", activeView: effectiveView };
    }

    // Determine which project sub-page is active.
    const activeProjectView = projectParams.isAutomations
      ? ("automations" as const)
      : ("dashboard" as const);

    return {
      level: "project",
      project,
      activeStream: projectParams.stream,
      activeProjectView,
      onBack:
        projectParams.itemId || projectParams.isAutomations
          ? () => {
              // Editor/automations → project detail (up one level)
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
              // Project detail → workspace dashboard (up one level)
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
      onOpenFile: (itemId: string) => {
        void navigate({
          to: "/$workspace/p/$projectId/s/$stream/$itemId/translate",
          params: {
            workspace: workspaceSlug ?? ws,
            projectId: project.id,
            stream: projectParams.stream,
            itemId,
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
    };
  }, [pathname, workspaceSlug, stream, activeView, ws, queryClient, navigate, handleStreamChange]);

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
          if (data.id_token_hint) {
            endSessionUrl.searchParams.set("id_token_hint", data.id_token_hint);
            endSessionUrl.searchParams.set(
              "post_logout_redirect_uri",
              window.location.origin + "/",
            );
          }
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
      const wsSlug = workspaceSlug ?? "";
      switch (view) {
        case "translate":
          void navigate({ to: "/$workspace", params: { workspace: wsSlug } });
          break;
        case "brand":
          void navigate({
            to: "/$workspace/brand",
            params: { workspace: wsSlug },
          });
          break;
        case "termbase":
          void navigate({
            to: "/$workspace/termbase",
            params: { workspace: wsSlug },
          });
          break;
        case "memory":
          void navigate({
            to: "/$workspace/memory",
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

  const isEditor = activeView === "translate";

  return (
    <AuthProvider initialUser={user}>
      <WorkspaceProvider
        key={activeWorkspace.slug}
        initialWorkspace={activeWorkspace}
        initialWorkspaces={workspaces}
      >
        <BravoProvider>
          <StreamActionsProvider>
            <AppShell
              workspaces={workspaces}
              activeWorkspace={activeWorkspace}
              onSelectWorkspace={handleSelectWorkspace}
              onCreateWorkspace={serverMode === "server" ? () => setShowCreateWs(true) : undefined}
              activeView={effectiveView}
              onViewChange={handleViewChange}
              user={user}
              onSignOut={serverMode === "server" ? handleSignOut : undefined}
              collapsed={sidebarCollapsed}
              onCollapsedChange={setSidebarCollapsed}
              showThemeToggle={false}
              sidebarContext={sidebarContext}
              activeSubNav={settingsSubNav}
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
                  beforeAvatarSlot={<ConnectedBravoTrigger />}
                />
              }
              rightPanelSlot={<ConnectedBravoPanel />}
              contentClassName={isEditor ? "overflow-hidden" : "overflow-auto"}
            >
              <StreamProvider initialStream={currentStream} onStreamChange={handleStreamChange}>
                <Outlet />
              </StreamProvider>
            </AppShell>
          </StreamActionsProvider>
        </BravoProvider>

        <CreateWorkspaceDialog
          open={showCreateWs}
          onOpenChange={setShowCreateWs}
          onCreate={handleWorkspaceCreated}
        />
      </WorkspaceProvider>
    </AuthProvider>
  );
}
