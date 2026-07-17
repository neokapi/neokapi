// Docusaurus client module: cookieless pageview analytics for this docs site.
//
// The shared implementation (@neokapi/docs-shared/src/analytics.ts) is
// key-gated and memory-only: with no POSTHOG_KEY at build time (local dev,
// fork builds, PR previews) nothing initializes and posthog-js is never
// loaded. This site captures pageviews only — no click instrumentation.
//
// Client modules are evaluated during SSR too; init guards on `window` and
// onRouteDidUpdate only runs in the browser.
import type { ClientModule } from "@docusaurus/types";
import siteConfig from "@generated/docusaurus.config";
import { initDocsAnalytics, captureDocsPageview } from "@neokapi/docs-shared";

const fields = (siteConfig.customFields ?? {}) as Record<string, unknown>;

initDocsAnalytics({
  key: (fields.posthogKey as string) || undefined,
  host: (fields.posthogHost as string) || undefined,
  surface: "kapi-docs",
  environment: (fields.analyticsEnvironment as string) || "prod",
});

const clientModule: ClientModule = {
  // Fires on the initial route and on every SPA route change.
  onRouteDidUpdate({ location, previousLocation }) {
    if (
      previousLocation &&
      location.pathname === previousLocation.pathname &&
      location.search === previousLocation.search
    ) {
      return; // hash-only change — same page
    }
    captureDocsPageview();
  },
};

export default clientModule;
