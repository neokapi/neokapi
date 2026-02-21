import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
} from "@tanstack/react-router";
import { RootLayout } from "./root-layout";
import { AuthLayout } from "./auth-layout";
import { WorkspaceLayout } from "./workspace-layout";
import { ProjectDashboardRoute } from "./workspace/dashboard";
import { ProjectDetailRoute } from "./workspace/project-detail";
import { JoinRoute } from "./auth/join";
import { ClaimRoute } from "./auth/claim";
import { DeviceVerifyRoute } from "./auth/device-verify";
import { DeviceAuthorizedRoute } from "./auth/device-authorized";
import { IndexRedirect } from "./index-redirect";

// ---------------------------------------------------------------------------
// Root
// ---------------------------------------------------------------------------

export const rootRoute = createRootRoute({
  component: RootLayout,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: IndexRedirect,
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
  component: WorkspaceLayout,
});

const dashboardRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "/",
  component: ProjectDashboardRoute,
});

const projectRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "project/$projectId",
  component: ProjectDetailRoute,
});

const translateRoute = createRoute({
  getParentRoute: () => workspaceRoute,
  path: "project/$projectId/translate/$fileName",
  component: lazyRouteComponent(
    () => import("./workspace/translate"),
    "TranslateRoute",
  ),
  validateSearch: (
    search: Record<string, unknown>,
  ): { locale?: string; block?: string; layout?: string } => ({
    locale: typeof search.locale === "string" ? search.locale : undefined,
    block: typeof search.block === "string" ? search.block : undefined,
    layout: typeof search.layout === "string" ? search.layout : undefined,
  }),
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

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
