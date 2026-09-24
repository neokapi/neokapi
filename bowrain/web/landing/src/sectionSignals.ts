// Landing-page visibility and interaction signals.
//
// section_viewed fires once per section after a minimum visibility period;
// position orders the analytics funnel. landing_engagement summarizes section
// dwell and widget interactions once per visit when the page is hidden.
// These events measure exposure and interaction, not comprehension.
//
// captureLandingEvent disables collection and analytics loading in keyless builds.

import { captureLandingEvent } from "./analytics";

/** A named beat of the narrative, in the order the page tells it. */
export interface SectionSpec {
  id: string;
  position: number;
}

const dwellMs = new Map<string, number>();
const viewed = new Set<string>();
const enteredAt = new Map<string, number>();
const engaged = new Set<string>();
let deepest: SectionSpec | null = null;
let flushed = false;
let flushInstalled = false;

/**
 * Record that a visitor engaged with a named proof (a coordinate example, the
 * check widget, the ship-state selector) rather than only scrolling past it.
 * Each name is reported once per visit on the summary event; the caller also
 * fires its own event with the detail.
 */
export function markEngaged(name: string): void {
  engaged.add(name);
}

/** Note a section as arrived-at. Idempotent: the funnel counts visits, not scrolls. */
export function markViewed(section: SectionSpec): void {
  if (viewed.has(section.id)) return;
  viewed.add(section.id);
  if (!deepest || section.position > deepest.position) deepest = section;
  captureLandingEvent("section_viewed", { section: section.id, position: section.position });
}

/** Start accumulating dwell for a section that just became visible. */
export function markEntered(id: string): void {
  if (!enteredAt.has(id)) enteredAt.set(id, Date.now());
  installFlush();
}

/** Stop accumulating dwell for a section that just left the viewport. */
export function markLeft(id: string): void {
  const since = enteredAt.get(id);
  if (since === undefined) return;
  enteredAt.delete(id);
  dwellMs.set(id, (dwellMs.get(id) ?? 0) + (Date.now() - since));
}

// The visit summary. `pagehide` rather than `beforeunload`: the latter is
// unreliable on mobile Safari, which is where a bounce is most likely to be the
// whole visit. `visibilitychange` covers a tab switched away and never returned
// to. Both funnel through a once-only flush.
function installFlush(): void {
  if (flushInstalled) return;
  flushInstalled = true;
  const flush = () => {
    if (flushed) return;
    flushed = true;
    // markLeft deletes the entry it just read; Map iteration is defined to
    // tolerate deletion of the current key, so this closes the open sections
    // without a snapshot.
    for (const id of enteredAt.keys()) markLeft(id);
    const props: Record<string, unknown> = {
      sections_viewed: viewed.size,
      deepest_section: deepest?.id ?? null,
      deepest_position: deepest?.position ?? 0,
      engaged_with: [...engaged].sort(),
      total_dwell_ms: [...dwellMs.values()].reduce((a, b) => a + b, 0),
    };
    for (const [id, ms] of dwellMs) props[`dwell_ms_${id.replace(/-/g, "_")}`] = ms;
    captureLandingEvent("landing_engagement", props);
  };
  window.addEventListener("pagehide", flush);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") flush();
  });
}
