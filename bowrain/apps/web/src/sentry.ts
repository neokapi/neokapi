import * as Sentry from "@sentry/react";
import posthog from "posthog-js";

const SENTRY_DSN = import.meta.env.VITE_SENTRY_DSN as string | undefined;

// posthogSessionTags attaches the active PostHog session id + replay URL to a
// Sentry event, so an operator triaging an error can jump straight to the
// recording of what the user did. Guarded so it is safe when replay is off or
// the posthog-js API differs across versions.
function posthogSessionTags(event: Sentry.ErrorEvent): Sentry.ErrorEvent {
  try {
    const ph = posthog as unknown as {
      get_session_id?: () => string | undefined;
      get_session_replay_url?: (opts?: { withTimestamp?: boolean }) => string | undefined;
    };
    const sessionId = ph.get_session_id?.();
    const replayUrl = ph.get_session_replay_url?.({ withTimestamp: true });
    if (sessionId || replayUrl) {
      event.tags = {
        ...event.tags,
        ...(sessionId ? { posthog_session_id: sessionId } : {}),
        ...(replayUrl ? { posthog_replay_url: replayUrl } : {}),
      };
    }
  } catch {
    // Never let telemetry-linking break error reporting.
  }
  return event;
}

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
    // Link each error to the PostHog session replay (error → watch the session).
    beforeSend: posthogSessionTags,
  });
  Sentry.setTag("surface", "web-app");
  initialized = true;
}

/** True when Sentry is configured (used to choose the ErrorBoundary variant). */
export function sentryEnabled(): boolean {
  return initialized;
}

export { Sentry };
