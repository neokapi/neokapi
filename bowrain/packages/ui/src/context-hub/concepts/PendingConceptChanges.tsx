// PendingConceptChanges: the Concepts page's summons.
//
// A governed change to the workspace terms travels through a change-set, and
// until it merges the Concepts list shows nothing of it: the concepts are
// proposed, not live, so the list is correctly empty and the page reads as
// though nothing happened. A `kapi push` that proposed 57 concepts left a
// reviewer looking at a page with no indication there was anything to approve.
//
// This is the indication. It sits above the list, names what is waiting, who
// proposed it and how long ago, and goes one click to the change-set. It shows
// only what is genuinely pending review, because a draft is its author's business.
import { Button, Skeleton, SimpleTooltip } from "@neokapi/ui-primitives";
import { One, Other, Plural, t } from "@neokapi/i18n-react/runtime";
import { Shield, ArrowRight, Info } from "../../components/icons";
import type { ChangeSet } from "../../types/brand-graph";
import { useChangesets } from "../../hooks/useChangesetsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { formatRelative } from "../shell/atoms";
import { sortByWaiting, waitingSince } from "../dashboard/metrics";

/** How many waiting change-sets the banner names before it summarises the rest. */
const MAX_ROWS = 3;

export interface PendingConceptChangesProps {
  /** Open a change-set's review page. */
  onOpenChangeSet?: (changesetId: string) => void;
}

/**
 * usePendingConceptChanges returns the change-sets waiting on a review, oldest
 * first. Exported so the Concepts section can also tell an empty list apart from
 * an empty vocabulary.
 */
export function usePendingConceptChanges() {
  const { data, isLoading } = useChangesets("in_review");
  return { pending: sortByWaiting(data ?? []), loading: isLoading };
}

export function PendingConceptChanges({ onOpenChangeSet }: PendingConceptChangesProps) {
  const { pending, loading } = usePendingConceptChanges();

  if (loading) return <Skeleton className="h-10 w-full rounded-lg" />;
  if (pending.length === 0) return null;

  const shown = pending.slice(0, MAX_ROWS);
  const rest = pending.length - shown.length;

  return (
    <section
      data-slot="concepts-pending"
      className="overflow-hidden rounded-lg border border-border bg-muted/30"
    >
      <div className="flex items-center gap-2 px-3 pt-2 pb-1">
        <Shield className="size-3.5 shrink-0 text-primary" aria-hidden />
        <h2 className="text-[11px] font-semibold text-muted-foreground">
          <Plural count={pending.length}>
            <One>A change to these concepts is waiting for review</One>
            <Other>{pending.length} changes to these concepts are waiting for review</Other>
          </Plural>
        </h2>
        <SimpleTooltip
          content={t(
            "Proposed concepts and terms stay out of this list until a reviewer approves them.",
          )}
        >
          <span
            tabIndex={0}
            aria-label={t("Why these concepts are not in the list")}
            className="inline-flex rounded text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
          >
            <Info className="size-3.5" aria-hidden />
          </span>
        </SimpleTooltip>
      </div>
      <ul className="divide-y divide-border">
        {shown.map((cs) => (
          <PendingRow
            key={cs.id}
            changeset={cs}
            onOpen={onOpenChangeSet ? () => onOpenChangeSet(cs.id) : undefined}
          />
        ))}
      </ul>
      {rest > 0 && (
        <p className="border-t border-border px-3 py-1.5 text-[11px] text-muted-foreground">
          and {rest} more waiting.
        </p>
      )}
    </section>
  );
}

function PendingRow({ changeset, onOpen }: { changeset: ChangeSet; onOpen?: () => void }) {
  const { nameOf } = useUserDisplayNames();
  // The list route counts each change-set's ops, so the count that makes this
  // concrete ("57 changes" rather than "a change") costs no extra request.
  const count = changeset.ops_count;

  return (
    <li className="flex flex-wrap items-center justify-between gap-3 px-3 py-1.5">
      <div className="min-w-0 flex-1">
        <button
          type="button"
          onClick={onOpen}
          disabled={!onOpen}
          className="truncate text-left text-sm font-medium text-foreground hover:underline disabled:cursor-default disabled:no-underline"
        >
          {changeset.name}
        </button>
        <p className="text-[11px] text-muted-foreground">
          {count === undefined ? null : (
            <>
              <span className="tabular-nums">{count.toLocaleString()}</span>{" "}
              <Plural count={count}>
                <One>change</One>
                <Other>changes</Other>
              </Plural>{" "}
              ·{" "}
            </>
          )}
          {nameOf(changeset.created_by)} · waiting {formatRelative(waitingSince(changeset))}
        </p>
      </div>
      <Button
        size="sm"
        variant="outline"
        className="h-7 shrink-0"
        onClick={onOpen}
        disabled={!onOpen}
      >
        Review
        <ArrowRight />
      </Button>
    </li>
  );
}
