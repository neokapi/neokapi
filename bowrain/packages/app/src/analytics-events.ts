/**
 * App-level analytics helpers (epic 018, workstream B).
 *
 * Event names are defined once in `@neokapi/ui`'s `analytics-events.ts` (the
 * lowest shared package, so the instrumented components there use the same
 * constants) and re-exported here for app call sites. This module adds the
 * route-pattern → feature derivation used by the router-driven `$pageview` /
 * `feature_entered` events in BowrainApp.
 */
export { AnalyticsEvents, type AnalyticsEventName } from "@neokapi/ui";

/** PostHog's reserved pageview event name (not part of our taxonomy). */
export const PAGEVIEW_EVENT = "$pageview";

/** Collapse a route path into snake_case, dropping `$param` segments. */
function segmentsToFeature(path: string): string {
  return path
    .split("/")
    .filter((s) => s && !s.startsWith("$"))
    .join("_")
    .replace(/-/g, "_");
}

/**
 * Derive a stable feature name from a TanStack Router route PATTERN (e.g.
 * "/$workspace/p/$projectId/s/$stream/translate/$" → "translate").
 * Patterns — not resolved paths — go in, so no workspace slugs, project ids,
 * or file names ever reach analytics. Returns null when the pattern carries
 * no feature (the bare root redirect).
 */
export function featureFromRoutePattern(pattern: string): string | null {
  const path = pattern.replace(/\/+$/, "") || "/";
  if (path === "/") return null;
  if (path === "/$workspace") return "dashboard";

  if (!path.startsWith("/$workspace/")) {
    // Auth/entry surfaces: /welcome, /join/$code, /pricing, /device/verify, …
    return segmentsToFeature(path) || null;
  }

  const inWorkspace = path.slice("/$workspace/".length);
  const project = inWorkspace.match(/^p\/\$projectId\/s\/\$stream(?:\/(.*))?$/);
  if (project) {
    const rest = project[1] ?? "";
    if (rest === "") return "project_overview";
    if (rest === "dashboard") return "translation_dashboard";
    if (rest === "settings") return "project_settings";
    // Per-file editor surfaces: {translate,review,pre-process}/$, whose
    // trailing splat is dropped with every other param segment.
    return segmentsToFeature(rest) || "project_overview";
  }
  return segmentsToFeature(inWorkspace) || null;
}
