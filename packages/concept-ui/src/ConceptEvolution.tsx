// ConceptEvolution — the responsive concept evolution view (Apache-2.0). It
// measures its OWN width and renders the horizontal `EvolutionRoadmap` when
// there is room, folding to the vertical `EvolutionGraph` on narrow widths. The
// timeline panel sits in a narrow dashboard column on large screens and
// full-width when promoted, and the same view is embedded in tighter desktop
// panes — so the fold keys off the container, not the viewport. Both renderers
// consume the same `EvolutionModel`, so switching is purely visual.

import { cn } from "@neokapi/ui-primitives";
import { useContainerWidth } from "./evolution-atoms";
import { EvolutionGraph } from "./EvolutionGraph";
import { EvolutionRoadmap } from "./EvolutionRoadmap";
import type { EvolutionViewProps } from "./evolution-view";

/** At or above this container width the horizontal roadmap is used. */
export const ROADMAP_MIN_WIDTH = 600;

export function ConceptEvolution({ className, ...props }: EvolutionViewProps) {
  const [ref, width] = useContainerWidth<HTMLDivElement>();
  // `useContainerWidth` measures in a layout effect (before paint), so a wide
  // container never flashes the fold; an unmeasured width (0, e.g. jsdom) takes
  // the graph, which fits any width.
  const roadmap = width >= ROADMAP_MIN_WIDTH;
  return (
    <div ref={ref} className={cn("min-w-0", className)}>
      {roadmap ? <EvolutionRoadmap {...props} /> : <EvolutionGraph {...props} />}
    </div>
  );
}
