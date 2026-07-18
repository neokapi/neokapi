import posthog from "posthog-js";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

let initialized = false;

export function initPostHog() {
  if (initialized || !POSTHOG_KEY) return;
  posthog.init(POSTHOG_KEY, {
    api_host: POSTHOG_HOST,
    // SPA route changes never trigger a full document load, so automatic
    // pageviews would only cover the first hit. All $pageview events —
    // including the initial one — are fired from the TanStack Router
    // subscription in BowrainApp (via the platform seam), which also attaches
    // the params-scrubbed route pattern.
    capture_pageview: false,
    capture_pageleave: true,
    autocapture: true,
    // Session replay — the best tool for reproducing a user-reported error:
    // watch exactly what they did. Privacy-first (GDPR / EU PostHog): every
    // text input is masked by default; mark extra-sensitive DOM with
    // [data-sensitive] to blackout, or [data-ph-no-capture] to drop entirely.
    disable_session_recording: false,
    session_recording: {
      maskAllInputs: true,
      maskTextSelector: "[data-sensitive]",
    },
  });
  // Mandatory taxonomy super-properties (epic 018): every event from this
  // surface — captures, pageviews, autocapture — is discriminated in the
  // shared PostHog project by surface + environment.
  posthog.register({
    surface: "web-app",
    environment: import.meta.env.MODE,
  });
  initialized = true;
}

export function identifyUser(id: string, properties?: Record<string, unknown>) {
  if (!POSTHOG_KEY) return;
  posthog.identify(id, properties);
}

/** Fire-and-forget product event (platform seam `analytics.capture`). */
export function captureEvent(event: string, properties?: Record<string, unknown>) {
  if (!POSTHOG_KEY) return;
  posthog.capture(event, properties);
}

/** Associate the session with a PostHog group (platform seam `analytics.group`). */
export function groupIdentify(type: string, key: string, properties?: Record<string, unknown>) {
  if (!POSTHOG_KEY) return;
  posthog.group(type, key, properties);
}

export function resetPostHog() {
  if (!POSTHOG_KEY) return;
  posthog.reset();
}

export { posthog };
