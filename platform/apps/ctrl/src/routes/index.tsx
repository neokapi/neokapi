import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import { isAuthenticated } from "../auth";
import { RootLayout } from "./root-layout";
import { AuthCallbackRoute } from "./auth-callback";
import { DashboardRoute } from "./dashboard";
import { WorkspacesRoute } from "./workspaces";
import { WorkspaceDetailRoute } from "./workspace-detail";
import { UsersRoute } from "./users";
import { EventsRoute } from "./events";
import { OverridesRoute } from "./overrides";
import { UpsellsRoute } from "./upsells";

// ---------------------------------------------------------------------------
// Router context
// ---------------------------------------------------------------------------

export interface RouterContext {
  queryClient: QueryClient;
}

// ---------------------------------------------------------------------------
// Root route
// ---------------------------------------------------------------------------

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

// ---------------------------------------------------------------------------
// Auth callback (no auth guard)
// ---------------------------------------------------------------------------

const authCallbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "auth/callback",
  component: AuthCallbackRoute,
});

// ---------------------------------------------------------------------------
// Protected routes — redirect to login if not authenticated
// ---------------------------------------------------------------------------

function requireAuth() {
  if (!isAuthenticated()) {
    throw redirect({ to: "/auth/callback", search: { action: "login" } });
  }
}

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: requireAuth,
  component: DashboardRoute,
});

const workspacesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "workspaces",
  beforeLoad: requireAuth,
  component: WorkspacesRoute,
});

const workspaceDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "workspaces/$workspaceId",
  beforeLoad: requireAuth,
  component: WorkspaceDetailRoute,
});

const usersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "users",
  beforeLoad: requireAuth,
  component: UsersRoute,
});

const eventsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "events",
  beforeLoad: requireAuth,
  component: EventsRoute,
});

const overridesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "overrides",
  beforeLoad: requireAuth,
  component: OverridesRoute,
});

const upsellsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "upsells",
  beforeLoad: requireAuth,
  component: UpsellsRoute,
});

// ---------------------------------------------------------------------------
// Route tree & router
// ---------------------------------------------------------------------------

const routeTree = rootRoute.addChildren([
  authCallbackRoute,
  dashboardRoute,
  workspacesRoute,
  workspaceDetailRoute,
  usersRoute,
  eventsRoute,
  overridesRoute,
  upsellsRoute,
]);

export const router = createRouter({
  routeTree,
  context: { queryClient: undefined! },
  defaultPendingMinMs: 0,
  defaultPendingMs: 100,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
