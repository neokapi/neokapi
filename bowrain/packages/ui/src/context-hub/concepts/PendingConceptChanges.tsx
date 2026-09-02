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
import { Button, Card, Skeleton } from "@neokapi/ui-primitives";
import { One, Other, Plural } from "@neokapi/i18n-react/runtime";
import { Shield, ArrowRight } from "../../components/icons";
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

  if (loading) return <Skeleton className="h-20 w-full rounded-xl" />;
  if (pending.length === 0) return null;

  const shown = pending.slice(0, MAX_ROWS);
  const rest = pending.length - shown.length;

  return (
    <Card className="gap-0 overflow-hidden p-0">
      <div className="flex items-center gap-2.5 border-b bg-muted/30 px-4 py-3">
        <span className="flex size-7 items-center justify-center rounded-md bg-primary/10 text-primary [&_svg]:size-4">
          <Shield />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-medium">
            <Plural count={pending.length}>
              <One>A change to these concepts is waiting for review</One>
              <Other>{pending.length} changes to these concepts are waiting for review</Other>
            </Plural>
          </h2>
          <p className="text-xs text-muted-foreground">
            Proposed concepts and terms stay out of this list until a reviewer approves them.
          </p>
        </div>
      </div>
      <ul className="divide-y">
        {shown.map((cs) => (
          <PendingRow
            key={cs.id}
            changeset={cs}
            onOpen={onOpenChangeSet ? () => onOpenChangeSet(cs.id) : undefined}
          />
        ))}
      </ul>
      {rest > 0 && (
        <p className="border-t px-4 py-2 text-xs text-muted-foreground">and {rest} more waiting.</p>
      )}
    </Card>
  );
}

function PendingRow({ changeset, onOpen }: { changeset: ChangeSet; onOpen?: () => void }) {
  const { nameOf } = useUserDisplayNames();
  // The list route counts each change-set's ops, so the count that makes this
  // concrete ("57 changes" rather than "a change") costs no extra request.
  const count = changeset.ops_count;

  return (
    <li className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <button
          type="button"
          onClick={onOpen}
          disabled={!onOpen}
          className="truncate text-left text-sm font-medium text-foreground hover:underline disabled:cursor-default disabled:no-underline"
        >
          {changeset.name}
        </button>
        <p className="mt-0.5 text-xs text-muted-foreground">
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
        className="h-8 shrink-0"
        onClick={onOpen}
        disabled={!onOpen}
      >
        Review
        <ArrowRight />
      </Button>
    </li>
  );
}
