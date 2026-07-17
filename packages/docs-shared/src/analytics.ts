// Cookieless PostHog analytics shared by the documentation sites.
//
// Contract (docs/marketing consent class): memory-only persistence — nothing
// is ever written to cookies or localStorage — Do Not Track respected,
// autocapture off (explicit events only), EU ingestion by default. Because no
// identifier is stored client-side, no consent banner is needed.
//
// Init is key-gated: without a key (local dev, fork builds, PR previews) the
// module is inert and `posthog-js` is never even loaded — the dynamic import
// below only runs when a key is present, so keyless builds execute zero
// analytics code.

export interface DocsAnalyticsOptions {
  /** PostHog project API key (public, client-side). Empty/undefined disables analytics. */
  key: string | undefined | null;
  /** Ingestion host. Defaults to the PostHog EU endpoint. */
  host?: string;
  /** Mandatory event property discriminating the site (e.g. "kapi-docs"). */
  surface: string;
  /** Mandatory event property: "prod" or "preview". */
  environment: string;
}

type CaptureProperties = Record<string, unknown>;

interface CaptureClient {
  capture(event: string, properties?: CaptureProperties): unknown;
}

let started = false;
let client: CaptureClient | null = null;
// Events captured between init and the async posthog-js load are queued so
// the first pageview (fired synchronously on route mount) is not dropped.
const queue: Array<[string, CaptureProperties | undefined]> = [];

/**
 * Initialize cookieless analytics. Safe to call in any environment: it is a
 * no-op during SSR, when no key is configured, and on repeat calls.
 */
export function initDocsAnalytics(options: DocsAnalyticsOptions): void {
  if (typeof window === "undefined" || started || !options.key) return;
  started = true;
  const key = options.key;
  void import("posthog-js").then(({ default: posthog }) => {
    posthog.init(key, {
      api_host: options.host || "https://eu.i.posthog.com",
      // Cookieless mode: state lives in memory for the tab's lifetime only.
      persistence: "memory",
      respect_dnt: true,
      autocapture: false,
      // Pageviews are captured explicitly (SPA route changes), not on load.
      capture_pageview: false,
      capture_pageleave: false,
      disable_session_recording: true,
      disable_surveys: true,
    });
    // Mandatory taxonomy properties on every event. register() writes to the
    // in-memory store under `persistence: "memory"` — nothing touches disk.
    posthog.register({ surface: options.surface, environment: options.environment });
    client = posthog;
    for (const [event, properties] of queue.splice(0)) {
      posthog.capture(event, properties);
    }
  });
}

/** Capture an explicit event. No-op unless analytics were initialized with a key. */
export function captureDocsEvent(event: string, properties?: CaptureProperties): void {
  if (!started) return;
  if (client) {
    client.capture(event, properties);
  } else {
    queue.push([event, properties]);
  }
}

/** Capture a pageview (standard PostHog `$pageview`; the current URL is attached). */
export function captureDocsPageview(): void {
  captureDocsEvent("$pageview");
}
