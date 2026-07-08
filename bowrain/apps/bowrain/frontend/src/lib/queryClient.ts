/**
 * Shared @tanstack/react-query client for the Bowrain desktop app.
 *
 * Server state in the desktop app is the local working copy reached over Wails
 * bindings (the `WailsApiAdapter`, plus the `Backend` binding for a few
 * desktop-only reads). react-query gives us the same caching / dedup /
 * invalidation model the web / ctrl / pulse apps already use: the Wails calls
 * are the query fns, and the backend freshness events (`useBackendEvents`)
 * become `queryClient.invalidateQueries` triggers (see
 * `hooks/useInvalidateOnEvent`). This matches kapi-desktop (#1142).
 */

import { QueryClient } from "@tanstack/react-query";

/**
 * Create a QueryClient with desktop-appropriate defaults.
 *
 * - `staleTime` is short: desktop data (formats, plugins, connectors, members)
 *   is cheap to re-read over Wails and changes via explicit backend events, so
 *   we keep it fresh-ish rather than aggressively cached.
 * - `retry` is disabled: a Wails binding either resolves or the runtime is
 *   absent (Storybook / vitest), where retrying just delays the empty state.
 * - `refetchOnWindowFocus` is off: the native window regains focus constantly;
 *   invalidation is driven by backend events instead.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: false,
        refetchOnWindowFocus: false,
      },
    },
  });
}

/** App-wide singleton client. Stories/tests create their own via the decorator. */
export const queryClient = createQueryClient();
