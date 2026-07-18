import * as Sentry from "@sentry/react";

const SENTRY_DSN = import.meta.env.VITE_SENTRY_DSN as string | undefined;

let initialized = false;

/**
 * initSentry wires browser error + performance monitoring for the web app.
 *
 * Gated on VITE_SENTRY_DSN: keyless builds (local dev, forks) are silent
 * no-ops, mirroring the PostHog seam. The DSN is publish-safe — it ships in the
 * bundle by design.
 *
 * Correlation: server responses carry X-Request-ID, surfaced to the user as an
 * error "reference"; backend exceptions are tagged with that same id in Sentry.
 * Frontend events are tagged surface: "web-app" so they are separable from the
 * server project in the same Sentry org.
 */
export function initSentry() {
  if (initialized || !SENTRY_DSN) return;
  Sentry.init({
    dsn: SENTRY_DSN,
    environment: import.meta.env.MODE,
    // Modest tracing by default; tune via the Sentry project if needed.
    integrations: [Sentry.browserTracingIntegration()],
    tracesSampleRate: 0.1,
    // Never ship request bodies / headers that could carry tokens.
    sendDefaultPii: false,
  });
  Sentry.setTag("surface", "web-app");
  initialized = true;
}

/** True when Sentry is configured (used to choose the ErrorBoundary variant). */
export function sentryEnabled(): boolean {
  return initialized;
}

export { Sentry };
