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
import { SlugReservationsRoute } from "./slug-reservations";

// ---------------------------------------------------------------------------
// Router context
// ---------------------------------------------------------------------------

export interface RouterContext {
  queryClient: QueryClient;
}

// ---------------------------------------------------------------------------
// Search param types (shared across routes and page components)
// ---------------------------------------------------------------------------

export interface WorkspacesSearch {
  q?: string;
  plan?: string;
  status?: string;
}

export interface UsersSearch {
  q?: string;
}

export interface EventsSearch {
  type?: string;
}

export interface OverridesSearch {
  q?: string;
  status?: string;
  feature?: string;
}

export interface UpsellsSearch {
  q?: string;
  plan?: string;
  signal?: string;
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
  validateSearch: (search: Record<string, unknown>): WorkspacesSearch => ({
    q: (search.q as string) || undefined,
    plan: (search.plan as string) || undefined,
    status: (search.status as string) || undefined,
  }),
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
  validateSearch: (search: Record<string, unknown>): UsersSearch => ({
    q: (search.q as string) || undefined,
  }),
});

const eventsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "events",
  beforeLoad: requireAuth,
  component: EventsRoute,
  validateSearch: (search: Record<string, unknown>): EventsSearch => ({
    type: (search.type as string) || undefined,
  }),
});

const overridesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "overrides",
  beforeLoad: requireAuth,
  component: OverridesRoute,
  validateSearch: (search: Record<string, unknown>): OverridesSearch => ({
    q: (search.q as string) || undefined,
    status: (search.status as string) || undefined,
    feature: (search.feature as string) || undefined,
  }),
});

const upsellsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "upsells",
  beforeLoad: requireAuth,
  component: UpsellsRoute,
  validateSearch: (search: Record<string, unknown>): UpsellsSearch => ({
    q: (search.q as string) || undefined,
    plan: (search.plan as string) || undefined,
    signal: (search.signal as string) || undefined,
  }),
});

const slugReservationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "slug-reservations",
  beforeLoad: requireAuth,
  component: SlugReservationsRoute,
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
  slugReservationsRoute,
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
