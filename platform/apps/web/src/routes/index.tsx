import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import type { ApiAdapter, User, Workspace } from "@gokapi/ui";
import { RootLayout } from "./root-layout";
import { AuthLayout } from "./auth-layout";
import { WorkspaceLayout } from "./workspace-layout";
import { ProjectDashboardRoute } from "./workspace/dashboard";
import { ProjectDetailRoute } from "./workspace/project-detail";
import { JoinRoute } from "./auth/join";
import { ClaimRoute } from "./auth/claim";
import { DeviceVerifyRoute } from "./auth/device-verify";
import { DeviceAuthorizedRoute } from "./auth/device-authorized";
import { useUIStore } from "../stores/ui-store";
import {
  configQueryOptions,
  currentUserQueryOptions,
  workspacesQueryOptions,
  projectsQueryOptions,
  projectQueryOptions,
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
      workspaces = [{
        id: "local", name: "Local", slug: "local",
        description: "", logo_url: "", type: "personal", role: "owner",
      }];
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

    return { serverMode, user, workspaces, activeWorkspace } satisfies WorkspaceRouteContext;
  },
  component: WorkspaceLayout,
});

const dashboardRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/",
  loader: async ({ context: { queryClient, api, activeWorkspace } }) => {
    await queryClient.ensureQueryData(projectsQueryOptions(api, activeWorkspace.slug));
  },
  component: ProjectDashboardRoute,
});

// Stream-scoped project routes.
const projectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "project/$projectId/stream/$stream",
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream));
  },
  component: ProjectDetailRoute,
});

const translateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "project/$projectId/stream/$stream/translate/$fileName",
  component: lazyRouteComponent(
    () => import("./workspace/translate"),
    "TranslateRoute",
  ),
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream));
  },
  validateSearch: (
    search: Record<string, unknown>,
  ): { locale?: string; block?: string; layout?: string } => ({
    locale: typeof search.locale === "string" ? search.locale : undefined,
    block: typeof search.block === "string" ? search.block : undefined,
    layout: typeof search.layout === "string" ? search.layout : undefined,
  }),
});

const automationsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "project/$projectId/stream/$stream/automations",
  component: lazyRouteComponent(
    () => import("./workspace/automations"),
    "AutomationsRoute",
  ),
  loader: async ({ context: { queryClient, api, activeWorkspace }, params }) => {
    await queryClient.ensureQueryData(projectQueryOptions(api, activeWorkspace.slug, params.projectId, params.stream));
  },
});

const termbaseRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "termbase",
  component: lazyRouteComponent(
    () => import("./workspace/termbase"),
    "TermbaseRoute",
  ),
});

const memoryRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "memory",
  component: lazyRouteComponent(
    () => import("./workspace/memory"),
    "MemoryRoute",
  ),
});

const settingsRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "settings",
  component: Outlet,
});

const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  component: lazyRouteComponent(
    () => import("./workspace/settings"),
    "SettingsIndexRoute",
  ),
});

const settingsMembersRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "members",
  component: lazyRouteComponent(
    () => import("./workspace/settings-members"),
    "SettingsMembersRoute",
  ),
});

const settingsProvidersRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "providers",
  component: lazyRouteComponent(
    () => import("./workspace/settings-providers"),
    "SettingsProvidersRoute",
  ),
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
  ]),
  workspaceRoute.addChildren([
    dashboardRoute,
    projectRoute,
    translateRoute,
    automationsRoute,
    termbaseRoute,
    memoryRoute,
    settingsRoute.addChildren([
      settingsIndexRoute,
      settingsMembersRoute,
      settingsProvidersRoute,
    ]),
  ]),
]);

// ---------------------------------------------------------------------------
// Router instance
// ---------------------------------------------------------------------------

export const router = createRouter({
  routeTree,
  context: { queryClient: undefined!, api: undefined! },
  defaultPendingMinMs: 200,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
