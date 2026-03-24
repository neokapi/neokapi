import { createRootRouteWithContext, createRoute, createRouter } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import { RootLayout } from "./root-layout";
import { WorkspaceOverviewPage } from "./workspace-overview";
import { ProjectDetailPage } from "./project-detail";
import { LocaleDetailPage } from "./locale-detail";
import { LeaderboardPage } from "./leaderboard";
import { TerminologyPage } from "./terminology";

export interface RouterContext {
  queryClient: QueryClient;
}

export interface PulseSearch {
  project?: string;
  language?: string;
  time?: string;
  type?: string;
  q?: string;
  tab?: string;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
});

const workspaceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace",
  component: WorkspaceOverviewPage,
  validateSearch: (search: Record<string, unknown>): PulseSearch => ({
    project: (search.project as string) || undefined,
    language: (search.language as string) || undefined,
    time: (search.time as string) || undefined,
    type: (search.type as string) || undefined,
    q: (search.q as string) || undefined,
  }),
});

const projectDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace/projects/$pid",
  component: ProjectDetailPage,
  validateSearch: (search: Record<string, unknown>): PulseSearch => ({
    language: (search.language as string) || undefined,
    time: (search.time as string) || undefined,
  }),
});

const localeDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace/projects/$pid/lang/$locale",
  component: LocaleDetailPage,
});

const leaderboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace/leaderboard",
  component: LeaderboardPage,
  validateSearch: (search: Record<string, unknown>): PulseSearch => ({
    time: (search.time as string) || undefined,
    tab: (search.tab as string) || undefined,
  }),
});

const terminologyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$workspace/terms",
  component: TerminologyPage,
  validateSearch: (search: Record<string, unknown>): PulseSearch => ({
    q: (search.q as string) || undefined,
  }),
});

const routeTree = rootRoute.addChildren([
  workspaceRoute,
  projectDetailRoute,
  localeDetailRoute,
  leaderboardRoute,
  terminologyRoute,
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
