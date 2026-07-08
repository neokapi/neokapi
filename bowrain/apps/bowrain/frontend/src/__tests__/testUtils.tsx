/**
 * Test utilities: a drop-in replacement for `@testing-library/react` whose
 * `render` / `renderHook` wrap the tree in a fresh QueryClientProvider.
 *
 * Desktop pages now read server state through react-query, so every rendered
 * tree needs a client. Importing `render` from here (instead of directly from
 * `@testing-library/react`) provides one automatically, with retries off and a
 * throwaway cache per render for isolation. Mirrors kapi-desktop's testUtils.
 */

import {
  render as rtlRender,
  renderHook as rtlRenderHook,
  type RenderOptions,
  type RenderHookOptions,
} from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Re-export everything else (screen, waitFor, fireEvent, within, act, …). The
// explicit `render`/`renderHook` below shadow the star-exported originals.
export * from "@testing-library/react";

function makeWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0, gcTime: 0 } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

export function render(ui: ReactElement, options?: RenderOptions) {
  return rtlRender(ui, { wrapper: makeWrapper(), ...options });
}

export function renderHook<Result, Props>(
  render: (props: Props) => Result,
  options?: RenderHookOptions<Props>,
) {
  return rtlRenderHook(render, { wrapper: makeWrapper(), ...options });
}
