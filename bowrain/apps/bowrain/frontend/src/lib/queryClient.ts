/**
 * Shared @tanstack/react-query client for the Bowrain desktop app.
 *
 * This singleton is handed to the mounted @neokapi/bowrain-app so the connection
 * gate and the shared route tree share one cache. Server state reaches the app
 * through the composite ApiAdapter (REST proxy + Wails local-first bindings);
 * freshness is driven by the platform seam's `watchProject` (the Go SSE→Wails
 * watcher), which invalidates the same caches the web SSE layer does.
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
