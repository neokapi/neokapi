import { useEffect, useMemo, useRef } from "react";
import { cn } from "@neokapi/ui-primitives";
import { getTargetText } from "../editor/blockStatus";
import { AlertTriangle, ShieldCheck, CircleCheck } from "../icons";
import {
  entryCheckStatus,
  groupEntries,
  type ReviewEntry,
  type ReviewGroupBy,
  type ReviewCheckStatus,
} from "./reviewQueue";

const GROUP_OPTIONS: { value: ReviewGroupBy; label: string }[] = [
  { value: "item", label: "File" },
  { value: "locale", label: "Locale" },
  { value: "check", label: "Checks" },
];

const checkDot: Record<ReviewCheckStatus, { className: string; icon: typeof AlertTriangle }> = {
  failing: { className: "text-destructive", icon: AlertTriangle },
  off_brand: { className: "text-warning", icon: ShieldCheck },
  clean: { className: "text-success", icon: CircleCheck },
};

export interface ReviewQueueListProps {
  /** The visible (already filtered) queue, in review order. */
  entries: ReviewEntry[];
  groupBy: ReviewGroupBy;
  onGroupByChange: (groupBy: ReviewGroupBy) => void;
  /** The id of the entry currently open in the reviewer. */
  currentId: string | null;
  onSelect: (id: string) => void;
  /** Locale display-name resolver for row/group labels. */
  localeName?: (locale: string) => string;
}

/**
 * ReviewQueueList is the review session's left rail: the pending blocks across
 * all of a project's items and locales, grouped (by file, locale, or
 * check-status) with a live count per group, and the current block highlighted.
 * Selection is keyboard-driven from the session; this panel reflects and scrolls
 * to `currentId` and lets the reviewer jump with a click.
 */
export function ReviewQueueList({
  entries,
  groupBy,
  onGroupByChange,
  currentId,
  onSelect,
  localeName,
}: ReviewQueueListProps) {
  const groups = useMemo(() => groupEntries(entries, groupBy), [entries, groupBy]);
  const rowRefs = useRef(new Map<string, HTMLButtonElement>());

  // Keep the current row in view as the reviewer advances (keyboard or bulk).
  useEffect(() => {
    if (!currentId) return;
    rowRefs.current.get(currentId)?.scrollIntoView({ block: "nearest" });
  }, [currentId]);

  return (
    <div
      className="flex min-h-0 w-72 shrink-0 flex-col border-r border-border"
      data-testid="review-queue"
    >
      <div className="flex items-center gap-1 border-b border-border p-2">
        {GROUP_OPTIONS.map((o) => (
          <button
            key={o.value}
            type="button"
            onClick={() => onGroupByChange(o.value)}
            data-testid={`group-by-${o.value}`}
            className={cn(
              "rounded-md px-2 py-1 text-xs transition-colors",
              groupBy === o.value
                ? "bg-primary font-semibold text-primary-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {o.label}
          </button>
        ))}
        <span
          className="ml-auto text-xs tabular-nums text-muted-foreground"
          data-testid="queue-total"
        >
          {entries.length}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {entries.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            No blocks match this filter.
          </div>
        ) : (
          groups.map((group) => (
            <div key={group.key} data-testid={`queue-group-${group.key}`}>
              <div className="sticky top-0 z-10 flex items-center justify-between bg-muted/60 px-3 py-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground backdrop-blur">
                <span className="truncate" title={group.label}>
                  {groupBy === "locale" && localeName ? localeName(group.label) : group.label}
                </span>
                <span className="tabular-nums">{group.entries.length}</span>
              </div>
              {group.entries.map((entry) => {
                const status = entryCheckStatus(entry);
                const Dot = checkDot[status].icon;
                const active = entry.id === currentId;
                return (
                  <button
                    key={entry.id}
                    type="button"
                    ref={(el) => {
                      if (el) rowRefs.current.set(entry.id, el);
                      else rowRefs.current.delete(entry.id);
                    }}
                    onClick={() => onSelect(entry.id)}
                    data-testid={`queue-row-${entry.id}`}
                    data-active={active}
                    className={cn(
                      "flex w-full items-start gap-2 border-b border-border/50 px-3 py-2 text-left transition-colors",
                      active ? "bg-primary/10" : "hover:bg-muted/40",
                    )}
                  >
                    <Dot
                      className={cn("mt-0.5 h-3.5 w-3.5 shrink-0", checkDot[status].className)}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm">{entry.block.source}</span>
                      <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                        {groupBy !== "locale" && (
                          <span className="mr-1 rounded bg-muted px-1 py-px">{entry.locale}</span>
                        )}
                        {getTargetText(entry.block, entry.locale) || "—"}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
