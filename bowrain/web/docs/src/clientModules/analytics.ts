// Docusaurus client module: cookieless analytics for the Bowrain docs site.
//
// The shared implementation (@neokapi/docs-shared/src/analytics.ts) is
// key-gated and memory-only: with no POSTHOG_KEY at build time (local dev,
// fork builds, PR previews) nothing initializes and posthog-js is never
// loaded. Beyond pageviews, this site instruments clicks on links to the app
// (app.bowrain.cloud) — the docs → signup leg of the funnel. Those links are
// plain markdown anchors spread across the docs, so a delegated document
// listener instruments them centrally; it also forwards any inbound utm_*
// parameters (and stamps a default utm_source) so campaign attribution
// survives the hop to the app.
//
// Client modules are evaluated during SSR too; init guards on `window`, and
// the listener is only attached in the browser (canUseDOM).
import type { ClientModule } from "@docusaurus/types";
import ExecutionEnvironment from "@docusaurus/ExecutionEnvironment";
import siteConfig from "@generated/docusaurus.config";
import { initDocsAnalytics, captureDocsEvent, captureDocsPageview } from "@neokapi/docs-shared";

const APP_HOST = "app.bowrain.cloud";

const fields = (siteConfig.customFields ?? {}) as Record<string, unknown>;

initDocsAnalytics({
  key: (fields.posthogKey as string) || undefined,
  host: (fields.posthogHost as string) || undefined,
  surface: "bowrain-docs",
  environment: (fields.analyticsEnvironment as string) || "prod",
});

if (ExecutionEnvironment.canUseDOM) {
  document.addEventListener(
    "click",
    (event) => {
      const anchor = (event.target as Element | null)?.closest?.("a[href]");
      if (!(anchor instanceof HTMLAnchorElement)) return;
      let url: URL;
      try {
        url = new URL(anchor.href);
      } catch {
        return;
      }
      if (url.hostname !== APP_HOST) return;
      // UTM-preserving href: forward inbound campaign parameters, then stamp
      // the docs site as the source when none is present.
      const inbound = new URLSearchParams(window.location.search);
      for (const [key, value] of inbound) {
        if (key.startsWith("utm_") && !url.searchParams.has(key)) {
          url.searchParams.set(key, value);
        }
      }
      if (!url.searchParams.has("utm_source")) {
        url.searchParams.set("utm_source", "bowrain-docs");
      }
      if (anchor.href !== url.toString()) anchor.href = url.toString();
      captureDocsEvent("signup_cta_clicked", {
        href: url.toString(),
        page: window.location.pathname,
      });
    },
    { capture: true },
  );
}

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
