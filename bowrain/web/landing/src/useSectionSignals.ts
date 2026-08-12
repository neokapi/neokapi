import { useEffect, useRef } from "react";
import { markEntered, markLeft, markViewed, type SectionSpec } from "./sectionSignals";

/** How much of a section counts as on screen. */
const VISIBLE_FRACTION = 0.5;
/** How long it must stay there before it counts as read rather than scrolled past. */
const ARRIVAL_MS = 400;

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

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            markEntered(section.id);
            arrival = setTimeout(() => markViewed(section), ARRIVAL_MS);
          } else {
            clearTimeout(arrival);
            markLeft(section.id);
          }
        }
      },
      { threshold: VISIBLE_FRACTION },
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
