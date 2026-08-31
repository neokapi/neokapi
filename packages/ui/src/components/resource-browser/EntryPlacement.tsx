// Where an approved answer came from: the unit it was approved for, and the
// coordinate it sits at.
//
// An entry the whole project agrees on carries a unit and no point — a point is
// stored only when one source has been answered differently at two of them. So
// the coordinate shown is either the stored one (a contested entry: this is
// where THIS answer was decided) or the one a host resolves for the unit (an
// uncontested entry: this is where that unit sits).
//
// The component decides its own absence. A host that cannot resolve a point
// supplies none and no coordinate is drawn, which is the honest reading: a
// browser that guessed would report a place nothing recorded.

import { useEffect, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { SimpleTooltip } from "../ui/tooltip";
import type { MemoryEntryDTO, MemoryPointDTO } from "./types";

export interface EntryPlacementProps {
  entry: MemoryEntryDTO;
  /** Resolve where an uncontested entry's unit sits. Absent = no coordinate. */
  resolvePoint?: (entry: MemoryEntryDTO) => Promise<MemoryPointDTO | null>;
  /** Open the context surface at this entry's unit. Absent = the unit is a label. */
  onOpenUnit?: (entry: MemoryEntryDTO) => void;
}

/** A point as the rungs a reader reads, coarsest first. */
export function formatPoint(point?: MemoryPointDTO | null): string {
  if (!point) return "";
  return [point.profile, point.channel, point.collection].filter(Boolean).join("/");
}

/** The last segment of a unit id, which is the part that identifies it. */
function shortUnit(unit: string): string {
  const parts = unit.split(/[:/]/);
  return parts[parts.length - 1] || unit;
}

export function EntryPlacement({ entry, resolvePoint, onOpenUnit }: EntryPlacementProps) {
  const stored = entry.point;
  const [resolved, setResolved] = useState<MemoryPointDTO | null>(null);

  useEffect(() => {
    // A contested entry already records its own point; resolving would answer a
    // question it has already answered, and could answer it differently.
    if (stored || !entry.unit || !resolvePoint) {
      setResolved(null);
      return;
    }
    let live = true;
    void resolvePoint(entry)
      .then((p) => {
        if (live) setResolved(p);
      })
      .catch(() => {
        // A point that will not resolve is a point not shown. The entry is
        // still a real answer, and saying nothing about where it sits beats
        // saying something unverified.
        if (live) setResolved(null);
      });
    return () => {
      live = false;
    };
  }, [entry, stored, resolvePoint]);

  const point = stored ?? resolved;
  const label = formatPoint(point);

  if (!entry.unit && !label) return null;

  return (
    <span className="inline-flex shrink-0 items-center gap-1.5" data-testid="entry-placement">
      {entry.unit &&
        (onOpenUnit ? (
          <SimpleTooltip content={entry.unit}>
            <button
              type="button"
              onClick={() => onOpenUnit(entry)}
              className="font-mono text-[10px] underline-offset-2 hover:text-foreground hover:underline"
              data-testid="entry-unit"
            >
              {shortUnit(entry.unit)}
            </button>
          </SimpleTooltip>
        ) : (
          <SimpleTooltip content={entry.unit}>
            <span className="font-mono text-[10px]" data-testid="entry-unit">
              {shortUnit(entry.unit)}
            </span>
          </SimpleTooltip>
        ))}
      {label && (
        <SimpleTooltip
          content={stored ? t("This answer was approved at this point") : t("Where this unit sits")}
        >
          <span
            className="inline-flex shrink-0 items-center rounded bg-muted px-1.5 py-px font-mono text-[10px]"
            data-testid="entry-point"
            data-contested={stored ? "true" : "false"}
          >
            {label}
          </span>
        </SimpleTooltip>
      )}
    </span>
  );
}
