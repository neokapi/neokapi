// TODO(flow-editor): The Bowrain web app has no flow list/CRUD route because the
// server exposes no flow-definition REST API yet (only the @bravo MCP run_flow /
// list_flows tools — see bowrain/server/mcp/tools_flow.go). When a flow REST API
// lands, add a `/workspace/$slug/flows` route that renders the shared
// `@neokapi/flow-editor` <FlowEditor>, bridging the node/edge definitions via the
// shared `defToSpec` / `specToDef` adapter (the same component + adapter the
// desktop FlowBuilder uses). Do NOT fork the editor for the web.
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import type { ApiAdapter, User, Workspace } from "@neokapi/ui";
import {
  DashboardSkeleton,
  ProjectDetailSkeleton,
  EditorSkeleton,
  TablePageSkeleton,
  BrandProfilesSkeleton,
  SettingsSkeleton,
  ExplorerSkeleton,
  TranslationDashboardSkeleton,
  ActivityFeedSkeleton,
  TaskBoardSkeleton,
} from "@neokapi/ui";
import { RootLayout } from "./root-layout";
import { AuthLayout } from "./auth-layout";
import { WorkspaceLayout } from "./workspace-layout";
import { ProjectDashboardRoute } from "./workspace/dashboard";
import { ProjectDetailRoute } from "./workspace/project-detail";
import { ProjectSettingsRoute } from "./workspace/project-settings";
import { JoinRoute } from "./auth/join";
import { ClaimRoute } from "./auth/claim";
import { DeviceVerifyRoute } from "./auth/device-verify";
import { DeviceAuthorizedRoute } from "./auth/device-authorized";
import { WelcomeRoute } from "./auth/welcome";
import { ConfirmEmailRoute } from "./auth/confirm-email";
import { useUIStore } from "../stores/ui-store";
import {
  configQueryOptions,
  currentUserQueryOptions,
  workspacesQueryOptions,
  projectsQueryOptions,
  projectQueryOptions,
  translationDashboardQueryOptions,
} from "../queries";

// ---------------------------------------------------------------------------
// Router context types
// ---------------------------------------------------------------------------

export interface RouterContext {
  queryClient: QueryClient;
  api: ApiAdapter;
}

export interface WorkspaceRouteContext {
  serverMode: "standalone" | "server";
  user: User;
  workspaces: Workspace[];
  activeWorkspace: Workspace;
}

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

export const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: async ({ context: { queryClient, api } }) => {
    const config = await queryClient.ensureQueryData(configQueryOptions(api));

    if (config.mode === "standalone") {
      throw redirect({
        to: "/$workspace",
        params: { workspace: "local" },
        replace: true,
      });
    }

    // Server mode — fetch user and workspaces in parallel.
    const [user, workspaces] = await Promise.all([
      queryClient.ensureQueryData(currentUserQueryOptions(api)),
      queryClient.ensureQueryData(workspacesQueryOptions(api)),
    ]);

    if (!user) {
      window.location.href = "/api/v1/auth/login";
      await new Promise(() => {}); // Prevent render while redirecting
      throw new Error("unreachable");
    }

    // First-run users have no personal workspace yet — route them through
    // /welcome to pick a handle. We bias to the user's onboarded_at flag
    // (set by CompleteOnboarding) and fall back to "no workspaces" so older
    // accounts that predate the flag still resolve.
    if (!user.onboarded_at && (!workspaces || workspaces.length === 0)) {
      throw redirect({ to: "/welcome", replace: true });
    }

    if (!workspaces || workspaces.length === 0) {
      return; // Renders the "no workspaces" component below
    }

    // Prefer the last-used workspace if it still exists.
    const lastSlug = useUIStore.getState().lastWorkspaceSlug;
    const target = (lastSlug && workspaces.find((w) => w.slug === lastSlug)) || workspaces[0];

    throw redirect({
      to: "/$workspace",
      params: { workspace: target.slug },
      replace: true,
    });
  },
  component: () => (
    <div className="flex items-center justify-center h-screen bg-background text-muted-foreground text-sm">
      No workspaces available. Please contact your administrator.
    </div>
  ),
});

// ---------------------------------------------------------------------------
// Auth routes (no workspace prefix)
// ---------------------------------------------------------------------------

const authLayout = createRoute({
  getParentRoute: () => rootRoute,
  id: "auth",
  component: AuthLayout,
});

const joinRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "join/$code",
  component: JoinRoute,
});

const claimRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "claim/$token",
  component: ClaimRoute,
});

const deviceVerifyRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "device/verify",
  component: DeviceVerifyRoute,
});

const deviceAuthorizedRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "device/authorized",
  component: DeviceAuthorizedRoute,
});

const welcomeRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "welcome",
  component: WelcomeRoute,
});

const confirmEmailRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "account/confirm-email",
  validateSearch: (search: Record<string, unknown>): { token?: string } => ({
    token: typeof search.token === "string" ? search.token : undefined,
  }),
  component: ConfirmEmailRoute,
});

// ---------------------------------------------------------------------------
// Workspace routes
// ---------------------------------------------------------------------------

const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace",
  beforeLoad: async ({ context: { queryClient, api }, params }) => {
    const config = await queryClient.ensureQueryData(configQueryOptions(api));

    let user: User;
    let workspaces: Workspace[];
    let serverMode: "standalone" | "server";

    if (config.mode === "standalone") {
      serverMode = "standalone";
      user = { id: "local", email: "", name: "Local User", avatar_url: "" };
      workspaces = [
        {
          id: "local",
          name: "Local",
          slug: "local",
          description: "",
          logo_url: "",
          type: "personal",
          role: "owner",
        },
      ];
    } else {
      serverMode = "server";
      const [fetchedUser, fetchedWorkspaces] = await Promise.all([
        queryClient.ensureQueryData(currentUserQueryOptions(api)),
        queryClient.ensureQueryData(workspacesQueryOptions(api)),
      ]);

      if (!fetchedUser) {
        window.location.href = "/api/v1/auth/login";
        await new Promise(() => {});
        throw new Error("unreachable");
      }

      // Bounce un-onboarded users to /welcome before they can access any
      // workspace URL. This handles direct navigation/bookmarks.
      if (!fetchedUser.onboarded_at && (!fetchedWorkspaces || fetchedWorkspaces.length === 0)) {
        throw redirect({ to: "/welcome", replace: true });
      }

      user = fetchedUser;
      workspaces = fetchedWorkspaces;
    }

    const match = workspaces.find((w) => w.slug === params.workspace);
    if (!match && workspaces.length > 0) {
      throw redirect({
        to: "/$workspace",
        params: { workspace: workspaces[0].slug },
        replace: true,
      });
    }

    const activeWorkspace = match ?? workspaces[0];
    useUIStore.getState().setLastWorkspaceSlug(activeWorkspace.slug);

    return {
      serverMode,
      user,
      workspaces,
      activeWorkspace,
    } satisfies WorkspaceRouteContext;
  },
  component: WorkspaceLayout,
});

const dashboardRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/",
  loader: async ({ context: { queryClient, api, activeWorkspace } }) => {
    await queryClient.ensureQueryData(projectsQueryOptions(api, activeWorkspace.slug));
  },
  pendingComponent: DashboardSkeleton,
  component: ProjectDashboardRoute,
});

// Stream-scoped project routes.
const projectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream",
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
  pendingComponent: ProjectDetailSkeleton,
  component: ProjectDetailRoute,
});

const projectSettingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/settings",
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
  pendingComponent: SettingsSkeleton,
  component: ProjectSettingsRoute,
});

const translateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/$itemId/translate",
  component: lazyRouteComponent(() => import("./workspace/translate"), "TranslateRoute"),
  pendingComponent: EditorSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
  validateSearch: (
    search: Record<string, unknown>,
  ): { locale?: string; block?: string; layout?: string } => ({
    locale: typeof search.locale === "string" ? search.locale : undefined,
    block: typeof search.block === "string" ? search.block : undefined,
    layout: typeof search.layout === "string" ? search.layout : undefined,
  }),
});

const reviewRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/$itemId/review",
  component: lazyRouteComponent(() => import("./workspace/review"), "ReviewRoute"),
  pendingComponent: EditorSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

const preProcessRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/$itemId/pre-process",
  component: lazyRouteComponent(() => import("./workspace/pre-process"), "PreProcessRoute"),
  pendingComponent: EditorSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

const automationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/automations",
  component: lazyRouteComponent(() => import("./workspace/automations"), "AutomationsRoute"),
  pendingComponent: TablePageSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

const translationDashboardRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/dashboard",
  pendingComponent: TranslationDashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/translation-dashboard"),
    "TranslationDashboardRoute",
  ),
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await Promise.all([
      queryClient.ensureQueryData(
        projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
      ),
      queryClient.ensureQueryData(
        translationDashboardQueryOptions(
          api,
          activeWorkspace.slug,
          params.projectId,
          params.stream,
        ),
      ),
    ]);
  },
});

// ── Brand hub (AD-021) ───────────────────────────────────────────────────────
// One workspace surface with five sections: Concepts (graph + list + per-concept
// story), Voice (profiles + correction loop), Experiments (change-sets), Activity,
// and Dashboard. The old standalone Termbase is absorbed into Concepts.

const brandRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "brand",
  component: Outlet,
});

// /brand → /brand/concepts (Concepts is the hub's landing section).
const brandIndexRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "/",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/$workspace/brand/concepts",
      params: { workspace: params.workspace },
      replace: true,
    });
  },
});

const brandConceptsRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "concepts",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-concepts"), "ConceptsRoute"),
});

const brandConceptStoryRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "concepts/$cid",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/brand-concept-story"),
    "ConceptStoryRoute",
  ),
});

const brandExperimentsRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "experiments",
  pendingComponent: TablePageSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-experiments"), "ExperimentsRoute"),
});

const brandExperimentDetailRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "experiments/$id",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/brand-experiment-detail"),
    "ExperimentDetailRoute",
  ),
});

const brandActivityRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "activity",
  pendingComponent: ActivityFeedSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-activity"), "BrandActivityRoute"),
});

const brandDashboardRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "dashboard",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-dashboard"), "BrandDashboardRoute"),
});

// Voice — the brand-voice profiles + correction loop, re-homed under the hub.
const brandVoiceRoute = createRoute({
  getParentRoute: () => brandRoute,
  path: "voice",
  component: Outlet,
});

const brandVoiceIndexRoute = createRoute({
  getParentRoute: () => brandVoiceRoute,
  path: "/",
  pendingComponent: BrandProfilesSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-profiles"), "BrandProfilesRoute"),
});

const brandVoiceEditorRoute = createRoute({
  getParentRoute: () => brandVoiceRoute,
  path: "$profileId",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-editor"), "BrandEditorRoute"),
});

const brandVoiceReviewRoute = createRoute({
  getParentRoute: () => brandVoiceRoute,
  path: "review/$profileId",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-review"), "BrandReviewRoute"),
});

const brandVoiceMCPGuideRoute = createRoute({
  getParentRoute: () => brandVoiceRoute,
  path: "mcp-guide",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/brand-mcp-guide"), "BrandMCPGuideRoute"),
});

// Legacy /termbase → Brand · Concepts. Terminology now lives inside the graph.
const termbaseRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "termbase",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/$workspace/brand/concepts",
      params: { workspace: params.workspace },
      replace: true,
    });
  },
});

const memoryRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "memory",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(() => import("./workspace/memory"), "MemoryRoute"),
});

const binRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "bin",
  pendingComponent: TablePageSkeleton,
  component: lazyRouteComponent(() => import("./workspace/bin"), "BinRoute"),
});

const auditlogRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "auditlog",
  pendingComponent: TablePageSkeleton,
  component: lazyRouteComponent(() => import("./workspace/auditlog"), "AuditLogRoute"),
});

const activitiesRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "activities",
  pendingComponent: ActivityFeedSkeleton,
  component: lazyRouteComponent(() => import("./workspace/activities"), "ActivitiesRoute"),
});

const tasksRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "tasks",
  pendingComponent: TaskBoardSkeleton,
  component: lazyRouteComponent(() => import("./workspace/tasks"), "TasksRoute"),
});

const userSettingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "user-settings",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/user-settings"), "UserSettingsRoute"),
});

const settingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "settings",
  component: Outlet,
});

const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/settings"), "SettingsIndexRoute"),
});

const settingsLanguagesRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "languages",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/settings-languages"),
    "SettingsLanguagesRoute",
  ),
});

const settingsMembersRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "members",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/settings-members"),
    "SettingsMembersRoute",
  ),
});

const settingsRolesRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "roles",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/settings-roles"), "SettingsRolesRoute"),
});

const settingsGovernanceRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "governance",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/settings-governance"),
    "SettingsGovernanceRoute",
  ),
});

const settingsProvidersRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "providers",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/settings-providers"),
    "SettingsProvidersRoute",
  ),
});

const settingsTokensRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "tokens",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/settings-tokens"), "SettingsTokensRoute"),
});

const settingsSystemRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "system",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/settings-system"), "SettingsSystemRoute"),
});

const settingsBravoRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "bravo",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/settings-bravo"), "SettingsBravoRoute"),
});

const settingsBillingRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "billing",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/settings-billing"),
    "SettingsBillingRoute",
  ),
});

const pricingRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "pricing",
  component: lazyRouteComponent(() => import("./pricing"), "PricingRoute"),
});

// ---------------------------------------------------------------------------
// Route tree
// ---------------------------------------------------------------------------

const routeTree = rootRoute.addChildren([
  indexRoute,
  authLayout.addChildren([
    joinRoute,
    claimRoute,
    deviceVerifyRoute,
    deviceAuthorizedRoute,
    welcomeRoute,
    confirmEmailRoute,
    pricingRoute,
  ]),
  workspaceRoute.addChildren([
    dashboardRoute,
    projectRoute,
    projectSettingsRoute,
    translateRoute,
    reviewRoute,
    preProcessRoute,
    automationsRoute,
    translationDashboardRoute,
    brandRoute.addChildren([
      brandIndexRoute,
      brandConceptsRoute,
      brandConceptStoryRoute,
      brandExperimentsRoute,
      brandExperimentDetailRoute,
      brandActivityRoute,
      brandDashboardRoute,
      brandVoiceRoute.addChildren([
        brandVoiceIndexRoute,
        brandVoiceEditorRoute,
        brandVoiceReviewRoute,
        brandVoiceMCPGuideRoute,
      ]),
    ]),
    termbaseRoute,
    memoryRoute,
    auditlogRoute,
    activitiesRoute,
    tasksRoute,
    userSettingsRoute,
    binRoute,
    settingsRoute.addChildren([
      settingsIndexRoute,
      settingsLanguagesRoute,
      settingsMembersRoute,
      settingsRolesRoute,
      settingsGovernanceRoute,
      settingsProvidersRoute,
      settingsTokensRoute,
      settingsSystemRoute,
      settingsBravoRoute,
      settingsBillingRoute,
    ]),
  ]),
]);

// ---------------------------------------------------------------------------
// Router instance
// ---------------------------------------------------------------------------

export const router = createRouter({
  routeTree,
  context: { queryClient: undefined!, api: undefined! },
  defaultPendingMinMs: 0,
  defaultPendingMs: 100,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
