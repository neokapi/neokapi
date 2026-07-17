// Opt-out desktop analytics (roadmap epic 018, workstream F; decision D1).
//
// posthog-js keyed by VITE_POSTHOG_KEY at build time — dev and self-builds are
// keyless and silent (no analytics code runs). Capture is gated on the
// persisted telemetry setting (default ON, npm-style first-run notice in
// App.tsx, toggle in the settings page) and on Do Not Track. The desktop never
// reports concrete URLs, file paths, project names, or content: pageviews
// carry the matched route *pattern* only, and every URL-ish default property
// is scrubbed from every event before it leaves the app.
import posthog from "posthog-js";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

// Persisted alongside the app's other local UI state (the shared app keeps its
// zustand-persist state under "bowrain-ui" in the same webview localStorage).
const STORAGE_KEY = "bowrain-telemetry";

interface TelemetryState {
  /** Opt-out flag; absent/anything-but-false means enabled (default ON). */
  enabled: boolean;
  /** Whether the one-time first-run notice has been shown. */
  noticeShown: boolean;
}

function readState(): TelemetryState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<TelemetryState>;
      return { enabled: parsed.enabled !== false, noticeShown: parsed.noticeShown === true };
    }
  } catch {
    // Unreadable state falls back to the defaults below.
  }
  return { enabled: true, noticeShown: false };
}

function writeState(state: TelemetryState): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    // Persistence is best-effort; the in-session behavior still applies.
  }
}

// The OS-level DO_NOT_TRACK convention surfaces in the webview as
// navigator.doNotTrack (with legacy spellings); honor any of them.
function doNotTrack(): boolean {
  const nav = navigator as Navigator & { doNotTrack?: string; msDoNotTrack?: string };
  const win = window as Window & { doNotTrack?: string };
  const value = nav.doNotTrack ?? win.doNotTrack ?? nav.msDoNotTrack;
  return value === "1" || value === "yes";
}

// Default posthog-js properties that carry the webview URL (whose hash route
// contains workspace slugs and project ids) — dropped from every event.
const URL_PROPERTIES = [
  "$current_url",
  "$pathname",
  "$host",
  "$referrer",
  "$referring_domain",
  "$initial_current_url",
  "$initial_pathname",
  "$initial_host",
  "$initial_referrer",
  "$initial_referring_domain",
];

let initialized = false;

/** True when a key is baked into this build and Do Not Track is not set. */
export function analyticsAvailable(): boolean {
  return Boolean(POSTHOG_KEY) && !doNotTrack();
}

export function isTelemetryEnabled(): boolean {
  return readState().enabled;
}

export function isNoticeShown(): boolean {
  return readState().noticeShown;
}

export function markNoticeShown(): void {
  writeState({ ...readState(), noticeShown: true });
}

/** Initialize analytics if the build has a key, DNT is unset, and the setting is on. */
export function initAnalytics(): void {
  if (initialized || !analyticsAvailable() || !isTelemetryEnabled()) return;
  posthog.init(POSTHOG_KEY!, {
    api_host: POSTHOG_HOST,
    // localStorage keeps a random anonymous distinct id stable across
    // launches (machine-id semantics); no cross-site cookies.
    persistence: "localStorage",
    respect_dnt: true,
    autocapture: false,
    capture_pageview: false,
    capture_pageleave: false,
    disable_session_recording: true,
    disable_surveys: true,
    sanitize_properties: (properties) => {
      for (const key of URL_PROPERTIES) {
        if (key in properties) delete properties[key];
      }
      return properties;
    },
  });
  // A previous in-session disable persists posthog's own opt-out flag; clear
  // it now that the setting is on again.
  if (posthog.has_opted_out_capturing()) posthog.opt_in_capturing();
  // Mandatory taxonomy property discriminating this surface on every event.
  posthog.register({ surface: "bowrain-desktop" });
  initialized = true;
}

/** Persist the telemetry setting and apply it to the live client. */
export function setTelemetryEnabled(enabled: boolean): void {
  writeState({ ...readState(), enabled });
  if (!analyticsAvailable()) return;
  if (enabled) {
    if (initialized) {
      posthog.opt_in_capturing();
    } else {
      initAnalytics();
    }
  } else if (initialized) {
    posthog.opt_out_capturing();
  }
}

/** Capture an explicit event. No-op unless analytics initialized. */
export function captureEvent(event: string, properties?: Record<string, unknown>): void {
  if (!initialized) return;
  posthog.capture(event, properties);
}

/** Associate the session with a PostHog group (e.g. the server workspace). */
export function groupAnalytics(type: string, key: string, props?: Record<string, unknown>): void {
  if (!initialized) return;
  posthog.group(type, key, props);
}

/** Identify by user id only — a local client never sends email or name. */
export function identifyUser(id: string): void {
  if (!initialized) return;
  posthog.identify(id);
}

export function resetAnalytics(): void {
  if (!initialized) return;
  posthog.reset();
}
