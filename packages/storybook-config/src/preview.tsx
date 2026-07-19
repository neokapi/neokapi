import type { Decorator, Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import {
  neokapiDecorator,
  neokapiGlobalType,
  type NeokapiStorybookOptions,
} from "@neokapi/i18n-react/storybook";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from "@tanstack/react-router";
import { themes } from "storybook/theming";
import React from "react";

/**
 * Provide a react-query client to every story.
 *
 * Desktop pages (and any component that reads server state via react-query)
 * need a QueryClientProvider in the tree. Stories typically pre-load their data
 * through props, so queries stay disabled — but the provider must exist for the
 * hooks to mount. A fresh client per preview keeps stories isolated; retries are
 * off so the empty (no-Wails) state renders immediately.
 */
export function withQueryClient(Story: React.ComponentType) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
}

/** A single query-cache seed: pre-load `data` at `queryKey`. */
export interface QuerySeedEntry {
  queryKey: readonly unknown[];
  data: unknown;
}

export interface MakeQueryClientDecoratorOptions {
  /**
   * Query-cache entries to pre-load. A seeded query resolves from the cache
   * without ever running its `queryFn`, so components that read server state
   * through react-query render their loaded state with no live backend — the
   * seam that lets Wails/desktop container pages story without a runtime.
   */
  seed?: QuerySeedEntry[];
}

/**
 * Build a QueryClient decorator that can pre-seed the react-query cache.
 *
 * This is the reusable primitive behind mocking a data-fetching backend in
 * Storybook. Container pages read server state through react-query where the
 * `queryFn` is the real backend call (a Wails binding, a REST client). Seeding
 * the cache — with `staleTime: Infinity` so seeded queries never re-fetch —
 * lets those queries resolve to fixture data, so the page renders its loaded
 * state and the real backend is never invoked. Unseeded queries stay disabled
 * or error out harmlessly (retries off), matching the empty state.
 *
 * `withQueryClient` is the no-seed case (fresh client, all queries empty). Apps
 * compose this with their own provider decorators (workspace, adapter, router)
 * to assemble a full container-page decorator; see the bowrain-desktop
 * `withBackend` story decorator for an example.
 */
export function makeQueryClientDecorator(
  options: MakeQueryClientDecoratorOptions = {},
): (Story: React.ComponentType) => React.ReactElement {
  const { seed = [] } = options;
  return function SeededQueryClient(Story: React.ComponentType) {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity },
      },
    });
    for (const { queryKey, data } of seed) {
      client.setQueryData(queryKey as unknown[], data);
    }
    return (
      <QueryClientProvider client={client}>
        <Story />
      </QueryClientProvider>
    );
  };
}

/**
 * Provide a TanStack Router context to every story.
 *
 * Components that call router hooks (`useNavigate`, `useLocation`,
 * `useSearch`, `Link`) need a RouterProvider in the tree — otherwise they
 * throw on mount. This decorator mounts the story as the component of a
 * throwaway in-memory router so those hooks resolve. Navigation targets that
 * don't exist in the single-route tree are inert (stories don't navigate), so
 * a bare root route is enough to render admin sidebars, tables, and pages that
 * merely *read* router state.
 *
 * Use it per-story (`decorators: [withRouter]`) rather than globally so plain
 * component stories keep the simpler tree.
 */
export function withRouter(Story: React.ComponentType) {
  const rootRoute = createRootRoute({ component: () => <Story /> });
  const router = createRouter({
    routeTree: rootRoute,
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  // The story router is intentionally minimal; the generic RouterProvider
  // accepts any router instance when no app has augmented the Register type.
  return <RouterProvider router={router as never} />;
}

/**
 * Wraps each story in a themed container so the correct theme surface
 * shows through — especially in the Docs tab where stories are inlined
 * in an otherwise-white documentation page.
 */
export function ThemeDecorator(Story: React.ComponentType) {
  return (
    <div className="bg-background text-foreground" style={{ minHeight: "100%" }}>
      <Story />
    </div>
  );
}

/** Detect system dark mode preference. */
export const prefersDark =
  typeof window !== "undefined" && window.matchMedia("(prefers-color-scheme: dark)").matches;

export type I18nOptions = NeokapiStorybookOptions;

export interface CreatePreviewOptions {
  /** Default Storybook layout: "centered" | "fullscreen" | "padded". */
  layout?: "centered" | "fullscreen" | "padded";
  /** Default theme: "light" | "dark" | "system". */
  defaultTheme?: "light" | "dark" | "system";
  /** Sidebar sort order (array of top-level category names). */
  sortOrder?: string[];
  /** Additional decorators inserted before theme decorators. */
  decorators?: Decorator[];
  /**
   * Enable a locale toolbar driven by @neokapi/i18n-react. Pair with `i18n: true`
   * in createMainConfig() so stories receive the runtime transform.
   */
  i18n?: I18nOptions;
}

/**
 * Creates a Storybook preview config with shared defaults.
 * Product-specific Storybooks call this with their own overrides.
 */
export function createPreview(options: CreatePreviewOptions = {}): Preview {
  const {
    layout = "centered",
    defaultTheme = "system",
    sortOrder,
    decorators: extraDecorators = [],
    i18n,
  } = options;

  const resolvedDefault =
    defaultTheme === "system" ? (prefersDark ? "dark" : "light") : defaultTheme;

  const preview: Preview = {
    parameters: {
      controls: {
        matchers: {
          color: /(background|color)$/i,
          date: /Date$/i,
        },
      },
      layout,
      backgrounds: { disabled: true },
      docs: {
        theme: resolvedDefault === "dark" ? themes.dark : themes.light,
      },
      ...(sortOrder && {
        options: {
          storySort: {
            order: sortOrder,
          },
        },
      }),
    },
    decorators: [
      withQueryClient,
      ...extraDecorators,
      ...(i18n ? [neokapiDecorator(i18n)] : []),
      ThemeDecorator,
      withThemeByClassName<ReactRenderer>({
        themes: {
          light: "",
          dark: "dark",
        },
        defaultTheme: resolvedDefault,
      }),
    ],
    ...(i18n && {
      globalTypes: {
        locale: neokapiGlobalType(i18n),
      },
    }),
  };

  return preview;
}
