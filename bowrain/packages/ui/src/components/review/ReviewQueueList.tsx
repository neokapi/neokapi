import { useEffect, useMemo, useRef } from "react";
import { cn, DirectionalText, LocaleLabel } from "@neokapi/ui-primitives";
import { getTargetText } from "../editor/blockStatus";
import { AlertTriangle, CircleCheck } from "../icons";
import {
  BLOCKER_LABELS,
  entryBlockers,
  entryVerdict,
  groupEntries,
  type ReviewEntry,
  type ReviewGroupBy,
  type ReviewQueueVerdict,
} from "./reviewQueue";

const GROUP_OPTIONS: { value: ReviewGroupBy; label: string }[] = [
  { value: "item", label: "File" },
  { value: "locale", label: "Language" },
  { value: "verdict", label: "Verdict" },
];

const verdictDot: Record<ReviewQueueVerdict, { className: string; icon: typeof AlertTriangle }> = {
  failing: { className: "text-destructive", icon: AlertTriangle },
  passing: { className: "text-success", icon: CircleCheck },
};

export interface ReviewQueueListProps {
  /** The visible (already filtered) queue, in review order. */
  entries: ReviewEntry[];
  /** The project's source language, so each row's source snippet reads in its own direction. */
  sourceLocale: string;
  groupBy: ReviewGroupBy;
  onGroupByChange: (groupBy: ReviewGroupBy) => void;
  /** The id of the entry currently open in the reviewer. */
  currentId: string | null;
  onSelect: (id: string) => void;
  /** The workspace's own name for a language, when it has one. */
  localeName?: (locale: string) => string;
}

/**
 * ReviewQueueList is the review session's left rail: the pending blocks across
 * all of a project's items and locales, grouped (by file, locale, or
 * verdict) with a live count per group, and the current block highlighted.
 * Selection is keyboard-driven from the session; this panel reflects and scrolls
 * to `currentId` and lets the reviewer jump with a click.
 */
export function ReviewQueueList({
  entries,
  sourceLocale,
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
                  {groupBy === "locale" ? (
                    <LocaleLabel
                      locale={group.label}
                      displayName={localeName?.(group.label)}
                      className="normal-case"
                    />
                  ) : (
                    group.label
                  )}
                </span>
                <span className="tabular-nums">{group.entries.length}</span>
              </div>
              {group.entries.map((entry) => {
                const verdict = entryVerdict(entry);
                const Dot = verdictDot[verdict].icon;
                const blockers = entryBlockers(entry);
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
                      className={cn("mt-0.5 h-3.5 w-3.5 shrink-0", verdictDot[verdict].className)}
                      // The dot says pass or fail; the title says which bar,
                      // so a row is never red for a reason nobody can read.
                      aria-label={blockers.map((b) => BLOCKER_LABELS[b]).join(", ") || "Passing"}
                    />
                    <span className="min-w-0 flex-1">
                      <DirectionalText locale={sourceLocale} className="block truncate text-sm">
                        {entry.block.source}
                      </DirectionalText>
                      <DirectionalText
                        locale={entry.locale}
                        className="mt-0.5 block truncate text-xs text-muted-foreground"
                      >
                        {groupBy !== "locale" && (
                          // A locale code is an identifier, not prose: always LTR
                          // and isolated so an RTL row's direction can't
                          // reposition it. The name is in the label's title, so a
                          // dense row keeps the tag and loses nothing.
                          <bdi dir="ltr" className="mr-1 rounded bg-muted px-1 py-px">
                            <LocaleLabel
                              locale={entry.locale}
                              displayName={localeName?.(entry.locale)}
                              compact
                            />
                          </bdi>
                        )}
                        {getTargetText(entry.block, entry.locale) || "—"}
                      </DirectionalText>
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
