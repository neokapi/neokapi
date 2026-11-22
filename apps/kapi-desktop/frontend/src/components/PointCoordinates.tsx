// Coordinates drawn once, and the differences drawn per row.
//
// A project's points usually share every coordinate but one (most often the
// channel). A list that repeats the full set on every row hides the one value
// that tells the rows apart, so a list shows the shared set once, above it,
// and each row shows only the coordinates where it differs.
//
// Each chip carries its axis name as a muted prefix. A recipe may mint any
// axis, and a chip for an axis the design system has no icon for would
// otherwise show a bare value such as "13-".

import { CoordinateChip, axisMeta, cn } from "@neokapi/ui-primitives";

/** A coordinate map, or nothing. */
export type Coordinates = Record<string, string> | undefined;

/**
 * The coordinates every map in the list holds with the same value: the line a
 * list draws once.
 */
export function sharedCoordinates(maps: Coordinates[]): Record<string, string> {
  const present = maps.filter((m): m is Record<string, string> => !!m);
  if (present.length === 0) return {};
  const [first, ...rest] = present;
  const shared: Record<string, string> = {};
  for (const [axis, value] of Object.entries(first)) {
    if (rest.every((m) => m[axis] === value)) shared[axis] = value;
  }
  return shared;
}

/** The entries of `coordinates` that `base` does not hold with the same value. */
export function coordinateDifferences(
  coordinates: Coordinates,
  base: Coordinates,
): [string, string][] {
  return Object.entries(coordinates ?? {}).filter(([axis, value]) => base?.[axis] !== value);
}

/**
 * A coordinate value as a reader sees it. An open numeric range written with a
 * hyphen ("13-", "-12", "13-17") reads as a range with an en dash, so "13-" no
 * longer reads as truncated text. Every other value is shown exactly as given.
 */
export function displayCoordinateValue(value: string): string {
  const range = /^(\d*)-(\d*)$/.exec(value);
  if (range && (range[1] || range[2])) return `${range[1]}–${range[2]}`;
  return value;
}

/** The axis name a chip's prefix shows, lowercase to sit quietly before it. */
function axisPrefix(axis: string): string {
  return axisMeta(axis).label.toLowerCase();
}

/** One coordinate: a muted axis name, then the chip. */
export function LabelledCoordinate({ axis, value }: { axis: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1" data-testid="labelled-coordinate">
      <span className="text-[11px] text-muted-foreground">{axisPrefix(axis)}</span>
      <CoordinateChip axis={axis} value={displayCoordinateValue(value)} />
    </span>
  );
}

/** A row of labelled coordinates; renders nothing for an empty set. */
export function LabelledCoordinates({
  entries,
  className,
  testId,
}: {
  entries: [string, string][];
  className?: string;
  testId?: string;
}) {
  if (entries.length === 0) return null;
  return (
    <span
      className={cn("flex flex-wrap items-center gap-x-2.5 gap-y-1", className)}
      data-testid={testId}
    >
      {entries.map(([axis, value]) => (
        <LabelledCoordinate key={axis} axis={axis} value={value} />
      ))}
    </span>
  );
}
