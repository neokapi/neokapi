import { useEffect, useRef } from "react";
import { markEntered, markLeft, markViewed, type SectionSpec } from "./sectionSignals";

/** How much of a section — or of the viewport — counts as on screen. */
const VISIBLE_FRACTION = 0.5;
/** How long it must stay there before it counts as read rather than scrolled past. */
const ARRIVAL_MS = 400;

// IntersectionObserver ratios are fractions of the *target*, so a section taller
// than the viewport can never reach 0.5 however it is scrolled: at 390×844 the
// coordinates, loop and proof beats top out at 0.44, 0.45 and 0.42. Observing a
// single 0.5 threshold therefore drops exactly the sections whose argument the
// page is measuring, on exactly the devices where a bounce is most likely.
//
// A section counts as on screen when half of it is visible OR it covers half the
// viewport — the second clause is what a tall section satisfies. Ratios are
// sampled on the way up so the predicate is re-evaluated as the section fills
// the screen; a single threshold only fires at its own crossing.
const THRESHOLDS = [0, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5];

/**
 * Whether a section is far enough on screen to count as arrived at.
 *
 * `visible` is the height of the intersection, `sectionHeight` the section's own
 * height and `viewportHeight` the visible height of the page.
 */
export function isOnScreen(
  visible: number,
  sectionHeight: number,
  viewportHeight: number,
): boolean {
  if (visible <= 0) return false;
  return visible >= VISIBLE_FRACTION * Math.min(sectionHeight, viewportHeight);
}

/**
 * Attach comprehension signals to one narrative section.
 *
 * Returns a ref for the `<section>` element itself. Separate from useReveal on
 * purpose: reveal animates an inner block on first sight and then stops
 * observing, while this has to keep watching to accumulate dwell. One ref per
 * job keeps either from quietly changing the other's threshold.
 */
export function useSectionSignals<T extends HTMLElement = HTMLElement>(section: SectionSpec) {
  const ref = useRef<T>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let arrival: ReturnType<typeof setTimeout> | undefined;

    let onScreen = false;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          const next = isOnScreen(
            entry.intersectionRect.height,
            entry.boundingClientRect.height,
            entry.rootBounds?.height ?? window.innerHeight,
          );
          if (next === onScreen) continue;
          onScreen = next;
          if (next) {
            markEntered(section.id);
            arrival = setTimeout(() => markViewed(section), ARRIVAL_MS);
          } else {
            clearTimeout(arrival);
            markLeft(section.id);
          }
        }
      },
      { threshold: THRESHOLDS },
    );
    observer.observe(el);
    return () => {
      clearTimeout(arrival);
      markLeft(section.id);
      observer.disconnect();
    };
  }, [section]);

  return ref;
}
