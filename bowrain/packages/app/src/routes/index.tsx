// Flow editing lives project-scoped under Automations › Flows
// (ProjectFlowsEditor over the flow-definition REST API); there is deliberately
// no workspace-level flows route because flow definitions are per-project.
// If one is ever added, render the shared `@neokapi/flow-editor` via the same
// `defToSpec` / `specToDef` adapter the desktop FlowBuilder uses — do NOT fork
// the editor for the web.
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
  type RouterHistory,
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
  ErrorNotice,
} from "@neokapi/ui";
import { searchInstallationId, searchSetupAction, searchSetupState } from "./installation-id";
import {
  searchPlan,
  searchSeats,
  stashIntendedPlan,
  readIntendedPlan,
  clearIntendedPlan,
  type IntendedPlan,
} from "./intended-plan";
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
  projectDetailQueryOptions,
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
  /**
   * Whether this deployment runs the brand-scan job system (the server's
   * `features.brand_scan` capability bit). Gates the hosted-scan entry points
   * so servers without it (SQLite/standalone) do not advertise a flow that
   * would fail with "brand scan system not configured".
   */
  brandScanAvailable: boolean;
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
  // A landing CTA appends `?plan=<id>` (Team also `&seats=`) so a self-serve
  // paid tier can be pre-selected after signup. Validate both against the known
  // self-serve plans; ignore anything else (the server re-validates on checkout).
  validateSearch: (search: Record<string, unknown>): { plan?: IntendedPlan; seats?: number } => ({
    plan: searchPlan(search.plan),
    seats: searchSeats(search.seats),
  }),
  beforeLoad: async ({ context: { queryClient, api }, search }) => {
    // Fire all three bootstrap fetches at once — config only decides how the
    // OTHER two are consumed, so waiting for it before starting them just
    // serializes the waterfall. In standalone mode the user/workspaces
    // results are ignored (rejections swallowed below).
    const configPromise = queryClient.ensureQueryData(configQueryOptions(api));
    const userPromise = queryClient.ensureQueryData(currentUserQueryOptions(api));
    const workspacesPromise = queryClient.ensureQueryData(workspacesQueryOptions(api));
    userPromise.catch(() => {});
    workspacesPromise.catch(() => {});

    const config = await configPromise;

    if (config.mode === "standalone") {
      throw redirect({
        to: "/$workspace",
        params: { workspace: "local" },
        replace: true,
      });
    }

    // Server mode — the user/workspaces fetches are already in flight.
    const [user, workspaces] = await Promise.all([userPromise, workspacesPromise]);

    if (!user) {
      // Carry the intended plan across the OIDC round-trip: stash it before we
      // leave for the identity provider, restore it when the user returns.
      if (search.plan) stashIntendedPlan({ plan: search.plan, seats: search.seats });
      window.location.href = "/api/v1/auth/login";
      await new Promise(() => {}); // Prevent render while redirecting
      throw new Error("unreachable");
    }

    // First-run users have no personal workspace yet — route them through
    // /welcome to pick a handle. We bias to the user's onboarded_at flag
    // (set by CompleteOnboarding) and fall back to "no workspaces" so older
    // accounts that predate the flag still resolve.
    if (!user.onboarded_at && (!workspaces || workspaces.length === 0)) {
      throw redirect({ to: "/welcome", search: { return_to: undefined }, replace: true });
    }

    if (!workspaces || workspaces.length === 0) {
      return; // Renders the "no workspaces" component below
    }

    // Prefer the last-used workspace if it still exists.
    const lastSlug = useUIStore.getState().lastWorkspaceSlug;
    const target = (lastSlug && workspaces.find((w) => w.slug === lastSlug)) || workspaces[0];

    // Plan passthrough: a fresh `?plan` (a returning, already-onboarded user who
    // clicked a landing CTA) or a stashed one (returned from OIDC without the
    // query string) routes to billing with the plan pre-selected instead of the
    // dashboard. Clear the stash so a later plain visit isn't hijacked.
    const intended = search.plan ? { plan: search.plan, seats: search.seats } : readIntendedPlan();
    if (intended) {
      clearIntendedPlan();
      throw redirect({
        to: "/$workspace/settings/billing",
        params: { workspace: target.slug },
        search: { plan: intended.plan, seats: intended.seats },
        replace: true,
      });
    }

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

const githubSetupRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "github/setup",
  validateSearch: (search: Record<string, unknown>) => ({
    installation_id: searchInstallationId(search.installation_id),
    // GitHub appends setup_action=install|update; the page treats both the
    // same, keeping it only to tell "arrived from GitHub" apart from a
    // hand-typed URL when the installation id is missing.
    setup_action: searchSetupAction(search.setup_action),
    // The signed state Bowrain sent to GitHub and GitHub echoed back. It names
    // the workspace that started the install, which is what lets the returning
    // request claim the installation; undeclared search params are stripped by
    // the router, so it has to be validated here to reach the page at all.
    state: searchSetupState(search.state),
  }),
  component: lazyRouteComponent(() => import("./github-setup"), "GithubSetupRoute"),
});

const welcomeRoute = createRoute({
  getParentRoute: () => authLayout,
  path: "welcome",
  validateSearch: (search: Record<string, unknown>) => ({
    return_to: typeof search.return_to === "string" ? search.return_to : undefined,
  }),
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
    // Start every bootstrap fetch in parallel — config is only consumed after
    // the others are needed, so awaiting it first would serialize three
    // round-trips into a waterfall. Standalone mode ignores the user/workspace
    // results (rejections swallowed below).
    const configPromise = queryClient.ensureQueryData(configQueryOptions(api));
    const userPromise = queryClient.ensureQueryData(currentUserQueryOptions(api));
    const workspacesPromise = queryClient.ensureQueryData(workspacesQueryOptions(api));
    userPromise.catch(() => {});
    workspacesPromise.catch(() => {});

    // Warm the child dashboard loader's projects query with the URL slug —
    // it does not depend on the fetches above, so it rides along instead of
    // becoming a third serial hop. prefetchQuery ignores errors (a stale/bad
    // slug simply leaves the cache cold and the loader refetches correctly
    // after the redirect below).
    void queryClient.prefetchQuery(projectsQueryOptions(api, params.workspace));

    const config = await configPromise;

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
      const [fetchedUser, fetchedWorkspaces] = await Promise.all([userPromise, workspacesPromise]);

      if (!fetchedUser) {
        window.location.href = "/api/v1/auth/login";
        await new Promise(() => {});
        throw new Error("unreachable");
      }

      // Bounce un-onboarded users to /welcome before they can access any
      // workspace URL. This handles direct navigation/bookmarks.
      if (!fetchedUser.onboarded_at && (!fetchedWorkspaces || fetchedWorkspaces.length === 0)) {
        throw redirect({ to: "/welcome", search: { return_to: undefined }, replace: true });
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
      brandScanAvailable: config.features?.brand_scan === true,
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

/**
 * An editor surface always names a file, so its trailing splat must not be
 * empty. A splat matches the empty remainder too and, being one node deeper,
 * outranks the exact sibling path — without this a bare `…/s/main/review`
 * opens the per-file editor on no file instead of the project-level review
 * session. Returning `false` from a param parse declines the match, handing
 * the path to that sibling.
 *
 * The router types `params.parse` away for a route whose own params are the
 * splat, but the matcher honours it, hence the cast. The behaviour is pinned
 * in routes/index.test.ts, so a router upgrade that drops it fails there
 * rather than in production.
 */
const requireSplat = {
  parse: (raw: Record<string, string>) => (raw._splat ? raw : false),
} as unknown as { parse?: undefined };

/**
 * The overview's URL state. The collection a reader is inside is a search
 * param, so the overview and the item list it drills into are one route and a
 * drill-down is linkable. Shared with the route's former `/dashboard` address
 * so a redirect from there carries the reader's scope across.
 */
/**
 * The item whose preview is open, by name. Held in the URL so the reading
 * deep-links and Back closes it rather than leaving the list behind.
 */
const previewSearch = (search: Record<string, unknown>): string | undefined =>
  typeof search.preview === "string" && search.preview !== "" ? search.preview : undefined;

const overviewSearch = (
  search: Record<string, unknown>,
): {
  collection?: string;
  ungrouped?: boolean;
  items?: "all";
  axis?: string;
  preview?: string;
} => ({
  collection: typeof search.collection === "string" ? search.collection : undefined,
  ungrouped: search.ungrouped === true || search.ungrouped === "true" ? true : undefined,
  items: search.items === "all" ? "all" : undefined,
  axis: typeof search.axis === "string" ? search.axis : undefined,
  preview: previewSearch(search),
});

/**
 * The overview needs the project's name and the dashboard's default page of
 * stats. Both are primed here so the component's first render is a cache hit;
 * neither carries the project's item array.
 */
const overviewLoader = async ({
  context: { queryClient, api, activeWorkspace },
  params,
}: {
  context: WorkspaceRouteContext & { queryClient: QueryClient; api: ApiAdapter };
  params: { projectId: string; stream: string };
}) => {
  await Promise.all([
    queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    ),
    queryClient.ensureQueryData(
      translationDashboardQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    ),
  ]);
};

// Stream-scoped project routes.
//
// The stream root is the project's overview: opening a project lands on its
// collections and their ship readiness, and the item list is one drill-down
// below (`?collection=` / `?items=all`). The file-centric source view — where
// collections, uploads and streams are managed — is a sibling at
// `…/s/$stream/source`.
const projectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream",
  pendingComponent: TranslationDashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/translation-dashboard"),
    "TranslationDashboardRoute",
  ),
  validateSearch: overviewSearch,
  loader: overviewLoader,
});

// The overview's former address. Links and bookmarks shipped pointing here, so
// it stays reachable and lands on the same surface at its new address, carrying
// the drill-down the reader was on.
const translationDashboardRedirectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/dashboard",
  validateSearch: overviewSearch,
  beforeLoad: ({ params, search }) => {
    throw redirect({
      to: "/$workspace/p/$projectId/s/$stream",
      params,
      search,
      replace: true,
    });
  },
});

// The project's source content: its collections, the files inside them, its
// streams, and the project-level actions (members, archive, settings). Reached
// from the sidebar beside the overview.
const projectSourceRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/source",
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectDetailQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
  pendingComponent: ProjectDetailSkeleton,
  component: ProjectDetailRoute,
  validateSearch: (search: Record<string, unknown>): { preview?: string } => ({
    preview: previewSearch(search),
  }),
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
  path: "p/$projectId/s/$stream/translate/$",
  params: requireSplat,
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
  path: "p/$projectId/s/$stream/review/$",
  params: requireSplat,
  component: lazyRouteComponent(() => import("./workspace/review"), "ReviewRoute"),
  pendingComponent: EditorSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

// The dedicated project-level governed review session (walks all pending
// blocks across items + locales). Distinct from the per-item `review/$`
// surface above, which the exact path outranks; every project-level "review"
// entry point routes here.
const reviewSessionRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/review",
  component: lazyRouteComponent(() => import("./workspace/review-session"), "ReviewSessionRoute"),
  pendingComponent: EditorSkeleton,
  // The collection a reviewer entered from, pre-applied to the queue. Same
  // spelling as the overview's own scope, so the two surfaces hand off without
  // translating between vocabularies.
  validateSearch: (
    search: Record<string, unknown>,
  ): { collection?: string; ungrouped?: boolean } => ({
    collection: typeof search.collection === "string" ? search.collection : undefined,
    ungrouped: search.ungrouped === true || search.ungrouped === "true" ? true : undefined,
  }),
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

const preProcessRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/pre-process/$",
  params: requireSplat,
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

const runsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/runs",
  component: lazyRouteComponent(() => import("./workspace/runs"), "RunsRoute"),
  pendingComponent: TablePageSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

const connectorsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "p/$projectId/s/$stream/connectors",
  component: lazyRouteComponent(() => import("./workspace/connectors"), "ConnectorsRoute"),
  pendingComponent: TablePageSkeleton,
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(
      projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream),
    );
  },
});

// ── Context hub (AD-021) ─────────────────────────────────────────────────────
// One workspace surface entered through its governance profiles — the points
// content sits at — with Concepts (graph + list + per-concept story), Voice
// (profiles + correction loop), Content memory, Changes (change-sets) and
// Activity beneath them. The old standalone Terms is absorbed into Concepts, and
// Content memory is re-homed here from its own top-level route. Dashboard also
// hangs off this section, but the sidebar files it under Insights.

const contextRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "context",
  component: Outlet,
});

// /context → /context/profiles. Profiles is the hub's landing section: a
// workspace's context is a set of points before it is a set of concepts.
const contextIndexRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "/",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/$workspace/context/profiles",
      params: { workspace: params.workspace },
      replace: true,
    });
  },
});

const contextProfilesRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "profiles",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-governance-profiles"),
    "ContextGovernanceProfilesRoute",
  ),
});

const contextProfileDetailRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "profiles/$slug",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-profile-detail"),
    "ContextProfileDetailRoute",
  ),
});

const contextConceptsRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "concepts",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(() => import("./workspace/context-concepts"), "ConceptsRoute"),
});

const contextConceptStoryRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "concepts/$cid",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-concept-story"),
    "ConceptStoryRoute",
  ),
});

// "Changes", not "experiments": a change-set with a reach preview and an
// optional pilot is the only route a governed change takes, so the URL names
// the destination the sub-nav names.
const contextChangesRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "changes",
  pendingComponent: TablePageSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-experiments"),
    "ExperimentsRoute",
  ),
});

const contextChangeDetailRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "changes/$id",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-experiment-detail"),
    "ExperimentDetailRoute",
  ),
});

const contextActivityRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "activity",
  pendingComponent: ActivityFeedSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-activity"),
    "ContextActivityRoute",
  ),
});

const contextMemoryRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "memory",
  pendingComponent: ExplorerSkeleton,
  component: lazyRouteComponent(() => import("./workspace/memory"), "MemoryRoute"),
});

const contextDashboardRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "dashboard",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-dashboard"),
    "ContextDashboardRoute",
  ),
});

// Brand scan (AI brand onboarding — epic 016): paste/link/upload/repo intake,
// then a polled job page that flips into the confidence/attribution review.
const contextScanRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "scan",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/context-scan"), "ContextScanRoute"),
});

const contextScanJobRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "scan/$jobId",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-scan-job"),
    "ContextScanJobRoute",
  ),
});

// Voice — the brand-voice profiles + correction loop, re-homed under the hub.
const contextVoiceRoute = createRoute({
  getParentRoute: () => contextRoute,
  path: "voice",
  component: Outlet,
});

const contextVoiceIndexRoute = createRoute({
  getParentRoute: () => contextVoiceRoute,
  path: "/",
  pendingComponent: BrandProfilesSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-voice-profiles"),
    "ContextVoiceProfilesRoute",
  ),
});

const contextVoiceEditorRoute = createRoute({
  getParentRoute: () => contextVoiceRoute,
  path: "$profileId",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/context-editor"), "ContextEditorRoute"),
});

const contextVoiceReviewRoute = createRoute({
  getParentRoute: () => contextVoiceRoute,
  path: "review/$profileId",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(() => import("./workspace/context-review"), "ContextReviewRoute"),
});

const contextVoiceMCPGuideRoute = createRoute({
  getParentRoute: () => contextVoiceRoute,
  path: "mcp-guide",
  pendingComponent: SettingsSkeleton,
  component: lazyRouteComponent(
    () => import("./workspace/context-mcp-guide"),
    "ContextMCPGuideRoute",
  ),
});

// Legacy /terms → Context · Concepts. Terminology now lives inside the graph.
const termsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "terms",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/$workspace/context/concepts",
      params: { workspace: params.workspace },
      replace: true,
    });
  },
});

// Legacy /memory → Context · Content memory. The page has one home now
// (contextMemoryRoute); this only keeps older links and bookmarks resolving.
const memoryRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "memory",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/$workspace/context/memory",
      params: { workspace: params.workspace },
      replace: true,
    });
  },
});

// Locale demand — live when a PostHog connector is configured; sample data
// otherwise. No plan-tier gating yet.
const localeDemandRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "locale-demand",
  pendingComponent: DashboardSkeleton,
  component: lazyRouteComponent(() => import("./workspace/locale-demand"), "LocaleDemandRoute"),
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

// Workspace-level review inbox: projects with pending review, linking into
// each project's focused review session.
const reviewInboxRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "review-inbox",
  pendingComponent: TaskBoardSkeleton,
  component: lazyRouteComponent(() => import("./workspace/review-inbox"), "ReviewInboxRoute"),
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
  // `?plan`/`?seats` pre-select a plan when the user arrives from a landing CTA
  // (via the index route's plan passthrough) so the page can offer a one-click
  // "complete your upgrade".
  validateSearch: (search: Record<string, unknown>): { plan?: IntendedPlan; seats?: number } => ({
    plan: searchPlan(search.plan),
    seats: searchSeats(search.seats),
  }),
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
    githubSetupRoute,
    deviceVerifyRoute,
    deviceAuthorizedRoute,
    welcomeRoute,
    confirmEmailRoute,
    pricingRoute,
  ]),
  workspaceRoute.addChildren([
    dashboardRoute,
    projectRoute,
    translationDashboardRedirectRoute,
    projectSourceRoute,
    projectSettingsRoute,
    translateRoute,
    reviewRoute,
    reviewSessionRoute,
    preProcessRoute,
    automationsRoute,
    runsRoute,
    connectorsRoute,
    contextRoute.addChildren([
      contextIndexRoute,
      contextProfilesRoute,
      contextProfileDetailRoute,
      contextConceptsRoute,
      contextConceptStoryRoute,
      contextChangesRoute,
      contextChangeDetailRoute,
      contextActivityRoute,
      contextMemoryRoute,
      contextDashboardRoute,
      contextScanRoute,
      contextScanJobRoute,
      contextVoiceRoute.addChildren([
        contextVoiceIndexRoute,
        contextVoiceEditorRoute,
        contextVoiceReviewRoute,
        contextVoiceMCPGuideRoute,
      ]),
    ]),
    termsRoute,
    memoryRoute,
    localeDemandRoute,
    auditlogRoute,
    activitiesRoute,
    tasksRoute,
    reviewInboxRoute,
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
// Router factory
// ---------------------------------------------------------------------------

// Each shell (web, desktop) builds its own router with its ApiAdapter +
// QueryClient baked into the context. The route tree is shared.
//
// `history` lets a shell override the default browser history: the desktop
// (Wails webview) passes a hash history so route changes and refreshes never
// hit the asset server (which would 404 on a deep path). Omitted on the web,
// where createRouter defaults to browser history.
// Branded fallback for any route that throws in a loader or on render and does
// not set its own errorComponent — matching the loading polish already wired
// via the skeleton pending components. `reset` re-attempts the failed boundary.
function RouteErrorComponent({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div className="flex min-h-[60vh] items-center justify-center p-6">
      <ErrorNotice error={error} onRetry={reset} className="max-w-lg" />
    </div>
  );
}

export function createBowrainRouter(context: RouterContext, opts?: { history?: RouterHistory }) {
  return createRouter({
    routeTree,
    context,
    defaultPendingMinMs: 0,
    defaultPendingMs: 100,
    defaultErrorComponent: RouteErrorComponent,
    ...(opts?.history ? { history: opts.history } : {}),
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createBowrainRouter>;
  }
}
