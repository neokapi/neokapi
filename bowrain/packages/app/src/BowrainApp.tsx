import { useState } from "react";
import { RouterProvider } from "@tanstack/react-router";
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
export function BowrainApp({ api, platform, queryClient }: BowrainAppProps) {
  const [qc] = useState(() => queryClient ?? makeQueryClient());
  const [router] = useState(() => createBowrainRouter({ queryClient: qc, api }));
  const [plat] = useState(() => platform ?? webPlatform());

  return (
    <PlatformProvider platform={plat}>
      <RouterProvider router={router} context={{ queryClient: qc, api }} />
    </PlatformProvider>
  );
}
