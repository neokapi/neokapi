/**
 * Shared @tanstack/react-query client for Kapi Desktop.
 *
 * Server state in the desktop app is the local `.kapi` project reached over
 * Wails bindings (the `api` object in `hooks/useApi.ts`). react-query gives us
 * the same caching / dedup / invalidation model the web apps use: Wails
 * bindings are the query fns, and Wails runtime events become
 * `queryClient.invalidateQueries` triggers (see `hooks/useInvalidateOnEvent`).
 */

import { QueryClient } from "@tanstack/react-query";

/**
 * Create a QueryClient with desktop-appropriate defaults.
 *
 * - `staleTime` is short: desktop data (formats, plugins, project files) is
 *   cheap to re-read over Wails and changes via explicit events, so we keep it
 *   fresh-ish rather than aggressively cached.
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
