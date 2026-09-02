import type { Run } from "@neokapi/contract-types";

import { cn } from "../../lib/utils";
import { directionAttrs } from "../../lib/text-direction";
import { RunText } from "./run-text";

/**
 * The unit in its document: the blocks before it, the unit itself, and the
 * blocks after it, in the order the file holds them.
 *
 * This is one of the four things a translate prompt carries about a block, so a
 * reviewer reading it reads what the model read. Kapi Desktop and the Bowrain
 * platform draw the same table, because a reviewer moving between the two
 * surfaces should be reading one document, not two renderings of it.
 *
 * Each row is a key, the source beneath it, and the target beneath that where
 * the neighbour has one. The neighbours travel as run sequences and are drawn
 * through the declared run projection, chips and all.
 */

/** One block beside the unit under decision. */
export interface NeighbourhoodEntry {
  /** The block's key, as the document names it. */
  key?: string;
  source?: Run[];
  /** What the locale under review says here, absent when nothing is translated. */
  target?: Run[];
}

export interface NeighbourhoodTableProps {
  /** The blocks before the unit, nearest last, so reading top to bottom reads the document. */
  before?: NeighbourhoodEntry[];
  /** The blocks after the unit, nearest first. */
  after?: NeighbourhoodEntry[];
  /** The unit under decision, rendered in place between its neighbours. */
  unitKey?: string;
  unitSource?: string;
  unitTarget?: string;
  /** The language the source is written in. */
  sourceLocale?: string;
  /** The language under review. */
  targetLocale?: string;
  className?: string;
}

function NeighbourRow({
  neighbour,
  sourceLocale,
  targetLocale,
}: {
  neighbour: NeighbourhoodEntry;
  sourceLocale?: string;
  targetLocale?: string;
}) {
  return (
    <li className="flex gap-2 px-2 py-1" data-slot="review-neighbour">
      <span
        className="w-24 shrink-0 truncate font-mono text-[10px] text-muted-foreground"
        translate="no"
      >
        {neighbour.key}
      </span>
      <span className="min-w-0 flex-1 space-y-0.5">
        <RunText
          runs={neighbour.source}
          className="block text-muted-foreground"
          dirAttrs={directionAttrs(sourceLocale)}
        />
        {neighbour.target && neighbour.target.length > 0 && (
          <RunText
            runs={neighbour.target}
            className="block"
            dirAttrs={directionAttrs(targetLocale)}
          />
        )}
      </span>
    </li>
  );
}

/** The blocks around a unit, with the unit itself in place among them. */
export function NeighbourhoodTable({
  before = [],
  after = [],
  unitKey,
  unitSource,
  unitTarget,
  sourceLocale,
  targetLocale,
  className,
}: NeighbourhoodTableProps) {
  return (
    <ul
      className={cn("divide-y divide-border/60 rounded-md border text-xs", className)}
      data-slot="review-neighbourhood-table"
    >
      {before.map((n, i) => (
        <NeighbourRow
          key={`before-${n.key ?? i}`}
          neighbour={n}
          sourceLocale={sourceLocale}
          targetLocale={targetLocale}
        />
      ))}
      <li className="flex gap-2 bg-primary/5 px-2 py-1.5" data-slot="review-neighbour-unit">
        <span className="w-24 shrink-0 break-all font-mono text-[10px] font-medium" translate="no">
          {unitKey}
        </span>
        <span className="min-w-0 flex-1 space-y-0.5">
          <span
            className="block whitespace-pre-wrap text-muted-foreground"
            translate="no"
            {...directionAttrs(sourceLocale)}
          >
            {unitSource}
          </span>
          {unitTarget ? (
            <span
              className="block whitespace-pre-wrap font-medium"
              translate="no"
              {...directionAttrs(targetLocale)}
            >
              {unitTarget}
            </span>
          ) : null}
        </span>
      </li>
      {after.map((n, i) => (
        <NeighbourRow
          key={`after-${n.key ?? i}`}
          neighbour={n}
          sourceLocale={sourceLocale}
          targetLocale={targetLocale}
        />
      ))}
    </ul>
  );
}
