import type { Decorator, Preview, ReactRenderer } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import {
  neokapiDecorator,
  neokapiGlobalType,
  type NeokapiStorybookOptions,
} from "@neokapi/kapi-react/storybook";
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
   * Enable a locale toolbar driven by @neokapi/kapi-react. Pair with `i18n: true`
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
