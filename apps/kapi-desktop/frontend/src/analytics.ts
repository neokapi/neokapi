// Opt-out desktop analytics for Kapi Desktop (roadmap epic 018, workstream F;
// decision D1).
//
// posthog-js keyed by VITE_POSTHOG_KEY at build time — dev and self-builds are
// keyless and silent (no analytics code runs). Capture is gated on the
// persisted app setting `telemetry_disabled` (default ON — opt-out with the
// one-time first-run notice, plus a toggle in Settings) and on Do Not Track.
// Events are limited to app_opened, panel pageviews (static view ids), and
// flow run started/completed — never content, file paths, or project names,
// and every URL-ish default property is scrubbed before an event leaves the
// app.
import posthog from "posthog-js";
import { api } from "./hooks/useApi";

const POSTHOG_KEY = import.meta.env.VITE_POSTHOG_KEY as string | undefined;
const POSTHOG_HOST = (import.meta.env.VITE_POSTHOG_HOST as string) || "https://eu.i.posthog.com";

// The OS-level DO_NOT_TRACK convention surfaces in the webview as
// navigator.doNotTrack (with legacy spellings); honor any of them.
function doNotTrack(): boolean {
  const nav = navigator as Navigator & { doNotTrack?: string; msDoNotTrack?: string };
  const win = window as Window & { doNotTrack?: string };
  const value = nav.doNotTrack ?? win.doNotTrack ?? nav.msDoNotTrack;
  return value === "1" || value === "yes";
}

// Default posthog-js properties that carry the webview URL — dropped from
// every event; pageviews carry a static view id in `route` instead.
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

function initClient(): void {
  if (initialized) return;
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
  posthog.register({ surface: "kapi-desktop" });
  initialized = true;
}

/**
 * Read the persisted setting and start analytics if permitted. Returns
 * whether the one-time first-run notice should be shown. Outside Wails
 * (Storybook, vitest) settings resolve to null and analytics stay off.
 */
export async function initAnalytics(): Promise<{ showNotice: boolean }> {
  if (!analyticsAvailable()) return { showNotice: false };
  const settings = await api.getSettings();
  if (!settings) return { showNotice: false };
  if (!settings.telemetry_disabled) {
    initClient();
    captureEvent("app_opened");
  }
  return { showNotice: !settings.telemetry_notice_shown };
}

/** Persist the telemetry setting and apply it to the live client. */
export async function setTelemetryEnabled(enabled: boolean): Promise<void> {
  const settings = (await api.getSettings()) ?? {};
  await api.saveSettings({ ...settings, telemetry_disabled: !enabled });
  if (!analyticsAvailable()) return;
  if (enabled) {
    if (initialized) {
      posthog.opt_in_capturing();
    } else {
      initClient();
    }
  } else if (initialized) {
    posthog.opt_out_capturing();
  }
}

/** Persist that the one-time first-run notice has been shown. */
export async function markTelemetryNoticeShown(): Promise<void> {
  const settings = (await api.getSettings()) ?? {};
  await api.saveSettings({ ...settings, telemetry_notice_shown: true });
}

/** Capture an explicit event. No-op unless analytics initialized. */
export function captureEvent(event: string, properties?: Record<string, unknown>): void {
  if (!initialized) return;
  posthog.capture(event, properties);
}

/** Capture a panel pageview. `route` is a static view id (e.g. "projects/flows"). */
export function capturePageview(route: string): void {
  captureEvent("$pageview", { route });
}

/** Coarsen a run duration into buckets (mirrors host/telemetry.DurationBucket). */
export function durationBucket(ms: number | undefined): string {
  if (ms === undefined || ms < 0) return "unknown";
  if (ms < 1_000) return "<1s";
  if (ms < 10_000) return "<10s";
  if (ms < 60_000) return "<60s";
  return ">=60s";
}
