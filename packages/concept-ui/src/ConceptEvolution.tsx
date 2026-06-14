// ConceptEvolution — the concept evolution view (Apache-2.0). The DEFAULT is the
// vertical `EvolutionGraph` timeline (a spine with icon nodes and event cards) —
// the pattern that reads instantly as a timeline and works at any width. When
// there is more than one language and the container is wide enough, a "Compare
// languages" toggle reveals the horizontal `EvolutionRoadmap` (a Gantt-style lane
// per language across one time axis). The view choice is the user's; nothing is
// forced by width except hiding the lanes option when there is no room for it.

import { useState } from "react";
import { ToggleGroup, ToggleGroupItem, cn } from "@neokapi/ui-primitives";
import { Columns3, GitCommitVertical } from "lucide-react";
import { useContainerWidth } from "./evolution-atoms";
import { EvolutionGraph } from "./EvolutionGraph";
import { EvolutionRoadmap } from "./EvolutionRoadmap";
import type { EvolutionViewProps } from "./evolution-view";

/** The horizontal lane view needs at least this much room to be legible. */
export const ROADMAP_MIN_WIDTH = 640;

type View = "timeline" | "lanes";

export function ConceptEvolution({ className, ...props }: EvolutionViewProps) {
  const [ref, width] = useContainerWidth<HTMLDivElement>();
  const [view, setView] = useState<View>("timeline");
  // The lanes view only makes sense with ≥2 languages and room to draw them.
  const canCompare = props.model.lanes.length >= 2 && width >= ROADMAP_MIN_WIDTH;
  const lanes = view === "lanes" && canCompare;

  return (
    <div ref={ref} className={cn("min-w-0", className)}>
      {canCompare && (
        <div className="mb-3 flex justify-end">
          <ToggleGroup
            type="single"
            size="sm"
            value={lanes ? "lanes" : "timeline"}
            onValueChange={(v) => v && setView(v as View)}
            aria-label="Evolution view"
          >
            <ToggleGroupItem value="timeline" className="gap-1.5 px-2.5 text-xs">
              <GitCommitVertical aria-hidden className="size-3.5" />
              Timeline
            </ToggleGroupItem>
            <ToggleGroupItem value="lanes" className="gap-1.5 px-2.5 text-xs">
              <Columns3 aria-hidden className="size-3.5" />
              Compare languages
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
      )}
      {lanes ? <EvolutionRoadmap {...props} /> : <EvolutionGraph {...props} />}
    </div>
  );
}
