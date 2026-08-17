// PostHog analytics for the ctrl admin panel (roadmap epic 018, workstream C).
//
// Key-gated init (keyless builds are silent no-ops), EU ingestion by default.
// Unlike the web app, autocapture stays off — ctrl emits explicit admin-action
// events plus route-pattern pageviews only, and events never carry PII beyond
// workspace/user ids. The {surface, environment} taxonomy is registered by the
// shared initPostHogSurface helper.
//
// Analytics start at authentication, never on load: `posthog.init` writes a
// year-long identifier on the registrable domain — the one the landing and
// documentation sites share and publish as cookieless — so an anonymous visit
// to ctrl, including the bounce to the admin identity provider, must leave
// nothing behind (#1940).
import posthog from "posthog-js";
import { initPostHogSurface } from "@neokapi/ui";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

let initialized = false;

/**
 * Start analytics for an authenticated admin session. Idempotent, and called
 * from the router's `requireAuth` gate — the one place that knows a session
 * exists — before the first pageview of that navigation is captured.
 */
export function startAnalytics() {
  if (initialized) return;
  initialized = initPostHogSurface(posthog, {
    surface: "ctrl",
    environment: import.meta.env.MODE,
    key: POSTHOG_KEY,
    host: POSTHOG_HOST,
    init: {
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

/** Capture a pageview carrying the matched route pattern (e.g. "/workspaces/$workspaceId"). */
export function capturePageview(route: string) {
  capture("$pageview", { route });
}
