// Cookieless PostHog analytics for the landing page.
//
// Same contract as the docs sites' shared module (packages/docs-shared/src/
// analytics.ts — duplicated here because the landing is a Vite app outside the
// Docusaurus toolchain): memory-only persistence, no cookies or localStorage,
// DNT respected, autocapture off, EU ingestion by default. Key-gated: without
// VITE_POSTHOG_KEY (local dev, fork builds, PR previews) nothing initializes
// and posthog-js is never even loaded.
//
// Events: one $pageview (single-page site) and signup/app CTA clicks — every
// anchor pointing at app.bowrain.cloud, instrumented via a delegated document
// listener that also forwards inbound utm_* parameters (and stamps a default
// utm_source) so campaign attribution survives the hop to the app.

const KEY = import.meta.env.VITE_POSTHOG_KEY;
const HOST = import.meta.env.VITE_POSTHOG_HOST || "https://eu.i.posthog.com";
const APP_HOST = "app.bowrain.cloud";

export function initAnalytics(): void {
  if (!KEY) return;
  const environment = import.meta.env.BASE_URL.includes("/prs/") ? "preview" : "prod";
  void import("posthog-js").then(({ default: posthog }) => {
    posthog.init(KEY, {
      api_host: HOST,
      // Cookieless mode: state lives in memory for the tab's lifetime only.
      persistence: "memory",
      respect_dnt: true,
      autocapture: false,
      capture_pageview: false,
      capture_pageleave: false,
      disable_session_recording: true,
      disable_surveys: true,
    });
    // Mandatory taxonomy properties on every event (in-memory only).
    posthog.register({ surface: "landing", environment });
    posthog.capture("$pageview");

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
        posthog.capture("signup_cta_clicked", {
          href: url.toString(),
          section: anchor.closest("section[id]")?.id ?? null,
        });
      },
      { capture: true },
    );
  });
}
