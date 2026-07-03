import { cn } from "@neokapi/ui-primitives";

export interface ListCapRowProps {
  /** Number of rows actually rendered. */
  shown: number;
  /** Total number of rows available. */
  total: number;
  /** Noun for the rows, e.g. "blocks", "entries". */
  noun?: string;
  /** Guidance for narrowing the list. */
  hint?: string;
  className?: string;
}

/**
 * Honest render-cap footer for capped lists: "Showing first N of M — refine
 * filters". Render it whenever a list truncates instead of cutting silently.
 * Returns null when nothing was cut.
 */
export function ListCapRow({
  shown,
  total,
  noun = "items",
  hint = "Refine the filters to narrow the list.",
  className,
}: ListCapRowProps) {
  if (total <= shown) return null;
  return (
    <div
      data-testid="list-cap-row"
      className={cn(
        "px-3 py-2 text-xs text-muted-foreground border-t border-border bg-muted/30 text-center",
        className,
      )}
    >
      Showing first {shown.toLocaleString()} of {total.toLocaleString()} {noun}. {hint}
    </div>
  );
}
