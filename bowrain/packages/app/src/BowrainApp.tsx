import { useState, useEffect } from "react";
import { RouterProvider, type RouterHistory } from "@tanstack/react-router";
import { QueryClient } from "@tanstack/react-query";
import type { ApiAdapter } from "@neokapi/ui";
import { createBowrainRouter } from "./routes";
import { PlatformProvider, webPlatform, type PlatformAdapter } from "./platform";

export interface BowrainAppProps {
  /** Data seam — RestApiAdapter on the web, WailsApiAdapter on the desktop. */
  api: ApiAdapter;
  /** Host-capability seam. Defaults to a bare web platform. */
  platform?: PlatformAdapter;
  /** Optional shared QueryClient; one is created with app defaults otherwise. */
  queryClient?: QueryClient;
  /**
   * Optional router history override. The desktop passes a hash history so the
   * Wails webview never 404s on refresh/deep navigation; the web omits it and
   * gets browser history.
   */
  history?: RouterHistory;
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: 1,
      },
    },
  });
}

/**
 * The shared Bowrain app: owns the QueryClient and router, threads the platform
 * seam through React context, and renders the route tree. The remaining
 * providers (Theme/Api/Auth/Workspace/Tooltip) live in the router's RootLayout,
 * which reads `api` and `queryClient` from the router context installed here.
 */
export function BowrainApp({ api, platform, queryClient, history }: BowrainAppProps) {
  const [qc] = useState(() => queryClient ?? makeQueryClient());
  const [plat] = useState(() => platform ?? webPlatform());
  const [router] = useState(() => createBowrainRouter({ queryClient: qc, api }, { history }));

  // Deep links (bowrain://…) are normalized by the desktop shell to an in-app
  // path; navigate the router to it. No-op on the web (webPlatform has no
  // onDeepLink), so this leaves web behavior untouched.
  useEffect(() => {
    if (!plat.onDeepLink) return;
    return plat.onDeepLink((path) => {
      // A resolved absolute path; push it through the router's history so it
      // works identically under browser and hash history.
      router.history.push(path);
    });
  }, [plat, router]);

  return (
    <PlatformProvider platform={plat}>
      <RouterProvider router={router} context={{ queryClient: qc, api }} />
    </PlatformProvider>
  );
}
