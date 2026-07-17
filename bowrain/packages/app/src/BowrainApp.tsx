import { useState, useEffect } from "react";
import { RouterProvider, type RouterHistory } from "@tanstack/react-router";
import { QueryClient } from "@tanstack/react-query";
import { AnalyticsProvider, type ApiAdapter } from "@neokapi/ui";
import { createBowrainRouter } from "./routes";
import { PlatformProvider, webPlatform, type PlatformAdapter } from "./platform";
import { AnalyticsEvents, PAGEVIEW_EVENT, featureFromRoutePattern } from "./analytics-events";

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

  // Route-pattern pageviews through the platform seam: one $pageview per
  // navigation carrying the matched route pattern (e.g. "/$workspace/p/$pid"),
  // never the concrete URL — no project names, slugs stay in params. No-op for
  // shells whose analytics member has no capture.
  useEffect(() => {
    const capture = plat.analytics?.capture?.bind(plat.analytics);
    if (!capture) return;
    let lastPath: string | undefined;
    const fire = () => {
      const { matches, location } = router.state;
      const leaf = matches[matches.length - 1];
      if (!leaf || location.pathname === lastPath) return;
      lastPath = location.pathname;
      capture("$pageview", { route: leaf.routeId });
    };
    // The initial navigation may resolve before this effect subscribes.
    fire();
    return router.subscribe("onResolved", fire);
  }, [plat, router]);

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

  // SPA navigation analytics (epic 018, workstream B): posthog-js only fires
  // $pageview on full document loads, so router navigations are captured here
  // through the platform seam. Events carry the matched route PATTERN
  // ("/$workspace/p/$projectId/s/$stream/$itemId/translate"), never resolved
  // params, so slugs/ids stay out of the properties. A feature_entered event
  // is derived from the same pattern, deduped against the previous feature.
  // No-op when the host wires no analytics (desktop).
  useEffect(() => {
    const analytics = plat.analytics;
    if (!analytics) return;
    let lastPathname: string | undefined;
    let lastFeature: string | undefined;
    const fire = () => {
      const matches = router.state.matches;
      const leaf = matches[matches.length - 1];
      if (!leaf) return;
      const pathname = router.state.location.pathname;
      if (pathname === lastPathname) return; // search-only change or duplicate resolve
      lastPathname = pathname;
      const pattern = leaf.fullPath;
      analytics.capture?.(PAGEVIEW_EVENT, { path_pattern: pattern });
      const feature = featureFromRoutePattern(pattern);
      if (feature && feature !== lastFeature) {
        lastFeature = feature;
        analytics.capture?.(AnalyticsEvents.featureEntered, { feature });
      }
    };
    fire(); // the initial load may resolve before this effect subscribes
    return router.subscribe("onResolved", fire);
  }, [plat, router]);

  return (
    <PlatformProvider platform={plat}>
      {/* Bridge the platform seam into the shared components' capture seam:
          @neokapi/ui call sites use useAnalytics(), which no-ops when the
          host (desktop) wires no analytics. */}
      <AnalyticsProvider
        capture={
          plat.analytics ? (event, props) => plat.analytics?.capture?.(event, props) : undefined
        }
      >
        <RouterProvider router={router} context={{ queryClient: qc, api }} />
      </AnalyticsProvider>
    </PlatformProvider>
  );
}
