// PostHog analytics for the pulse dashboard (roadmap epic 018, workstream C).
//
// Key-gated init (keyless builds are silent no-ops), EU ingestion by default.
// Autocapture stays off — pulse emits route-pattern pageviews only, and events
// never carry PII beyond ids. The {surface, environment} taxonomy is registered
// by the shared initPostHogSurface helper.
//
// Pulse is a public surface: its dashboards are readable without a session, so
// there is no authentication to start analytics behind. It therefore takes the
// other posture the parent domain publishes — the cookieless one the landing
// and documentation sites carry: memory-only persistence, so a visit writes no
// identifier to `.bowrain.cloud` (the domain those sites share), and Do Not
// Track is honoured (#1940).
import posthog from "posthog-js";
import { initPostHogSurface } from "@neokapi/ui";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

let initialized = false;

export function initAnalytics() {
  if (initialized) return;
  initialized = initPostHogSurface(posthog, {
    surface: "pulse",
    environment: import.meta.env.MODE,
    key: POSTHOG_KEY,
    host: POSTHOG_HOST,
    init: {
      // Cookieless: state lives in memory for the tab's lifetime only, so
      // nothing is written to cookies or localStorage.
      persistence: "memory",
      respect_dnt: true,
      disable_session_recording: true,
      disable_surveys: true,
      // Pageviews are fired explicitly on router resolution with the matched
      // route pattern (see main.tsx), not on document load.
      capture_pageview: false,
      capture_pageleave: true,
      autocapture: false,
    },
  });
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
