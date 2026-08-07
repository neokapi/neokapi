// Cookieless PostHog analytics for the landing page.
//
// The capture contract lives in @neokapi/docs-shared (memory-only persistence,
// no cookies or localStorage, DNT respected, autocapture off, EU ingestion by
// default, key-gated so keyless builds load zero analytics code). The shared
// module queues events fired before the async posthog-js import resolves, so a
// signup click in the first few hundred milliseconds is captured rather than
// dropped. Imported from the module path (not the package index) so the landing
// bundle never pulls the docs diagram components or their CSS.
//
// Events: one $pageview (single-page site) and signup/app CTA clicks.
//
// Separately from analytics — and NOT key-gated — every anchor pointing at
// app.bowrain.cloud forwards inbound utm_* parameters (and stamps a default
// utm_source) via a delegated document listener, so campaign attribution
// survives the hop to the app even when analytics never initializes.

import {
  initDocsAnalytics,
  captureDocsEvent,
  captureDocsPageview,
} from "@neokapi/docs-shared/analytics.ts";

const KEY = import.meta.env.VITE_POSTHOG_KEY;
const HOST = import.meta.env.VITE_POSTHOG_HOST || "https://eu.i.posthog.com";
const APP_HOST = "app.bowrain.cloud";

export function initAnalytics(): void {
  installSignupLinkForwarding();
  const environment = import.meta.env.BASE_URL.includes("/prs/") ? "preview" : "prod";
  initDocsAnalytics({ key: KEY, host: HOST, surface: "landing", environment });
  captureDocsPageview();
}

// Rewrites app.bowrain.cloud anchors on click to carry the visitor's inbound
// utm_* query parameters (UTM survival). Always on; the analytics capture at
// the end is a no-op unless PostHog initialized, and is queued if the click
// lands before posthog-js has loaded.
function installSignupLinkForwarding(): void {
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
      const inbound = new URLSearchParams(window.location.search);
      for (const [key, value] of inbound) {
        if (key.startsWith("utm_") && !url.searchParams.has(key)) {
          url.searchParams.set(key, value);
        }
      }
      if (!url.searchParams.has("utm_source")) {
        url.searchParams.set("utm_source", "bowrain-landing");
      }
      if (anchor.href !== url.toString()) anchor.href = url.toString();
      captureDocsEvent("signup_cta_clicked", {
        href: url.toString(),
        section: anchor.closest("section[id]")?.id ?? null,
      });
    },
    { capture: true },
  );
}
