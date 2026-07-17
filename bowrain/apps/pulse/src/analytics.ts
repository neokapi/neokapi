// PostHog analytics for the pulse dashboard (roadmap epic 018, workstream C).
//
// Mirrors bowrain/apps/web/src/posthog.ts: key-gated init (keyless builds are
// silent no-ops), EU ingestion by default. Autocapture stays off — pulse emits
// route-pattern pageviews only, and events never carry PII beyond ids.
import posthog from "posthog-js";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

let initialized = false;

export function initAnalytics() {
  if (initialized || !POSTHOG_KEY) return;
  posthog.init(POSTHOG_KEY, {
    api_host: POSTHOG_HOST,
    // Pageviews are fired explicitly on router resolution with the matched
    // route pattern (see main.tsx), not on document load.
    capture_pageview: false,
    capture_pageleave: true,
    autocapture: false,
  });
  // Mandatory taxonomy property discriminating this surface on every event.
  posthog.register({ surface: "pulse" });
  initialized = true;
}

/** Capture an explicit event. No-op when no key is configured. */
export function capture(event: string, properties?: Record<string, unknown>) {
  if (!initialized) return;
  posthog.capture(event, properties);
}

/** Capture a pageview carrying the matched route pattern (e.g. "/$workspace/projects/$pid"). */
export function capturePageview(route: string) {
  capture("$pageview", { route });
}
