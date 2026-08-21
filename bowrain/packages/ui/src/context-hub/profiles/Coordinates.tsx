import { cn } from "@neokapi/ui-primitives";

/**
 * A point's coordinates, read as a row of axis/value columns.
 *
 * The axis sits above its value rather than beside it, so a point with three
 * axes stays scannable and the axis names line up down a list of profiles.
 */
export function CoordinateReadout({
  coordinates,
  className,
}: {
  coordinates?: Record<string, string>;
  className?: string;
}) {
  const axes = Object.keys(coordinates ?? {}).sort();
  if (axes.length === 0) return null;

  return (
    <dl className={cn("flex flex-wrap items-stretch gap-x-4 gap-y-2", className)}>
      {axes.map((axis) => (
        <div key={axis} className="min-w-0 border-l border-border pl-3 first:border-l-0 first:pl-0">
          <dt className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {axis}
          </dt>
          <dd className="truncate font-mono text-sm text-foreground">{coordinates?.[axis]}</dd>
        </div>
      ))}
    </dl>
  );
}

/** The same point on one line, for a dense row. */
export function CoordinateLine({
  coordinates,
  className,
}: {
  coordinates?: Record<string, string>;
  className?: string;
}) {
  const axes = Object.keys(coordinates ?? {}).sort();
  if (axes.length === 0) return null;

  return (
    <span className={cn("font-mono text-xs text-muted-foreground", className)}>
      {axes.map((axis, i) => (
        <span key={axis}>
          {i > 0 && <span className="px-1 opacity-50">·</span>}
          {axis}
          <span className="opacity-50">=</span>
          <span className="text-foreground">{coordinates?.[axis]}</span>
        </span>
      ))}
    </span>
  );
}
