// The governance inbox (AD-021): the change-sets a steward must act on — in
// review, or approved and awaiting merge — ordered so the longest-waiting
// proposal leads. Each row carries its blast radius (computed before approval)
// and an inline approve action. This is the half of the dashboard a steward
// works, so it leads the page.
import { Button, Card, CardContent, Skeleton, cn } from "@neokapi/ui-primitives";
import { ErrorNotice } from "../../errors";
import { Shield, Check, ArrowRight, Loader2 } from "../../components/icons";
import type { ChangeSet, ImpactSummary } from "../../types/brand-graph";
import { useApproveChangeset } from "../../hooks/useChangesetsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { ChangeSetStatusBadge, formatRelative } from "../shell/atoms";
import { pendingDecisions, waitingSince } from "./metrics";

export interface PendingDecisionsProps {
  changesets: ChangeSet[];
  loading?: boolean;
  onOpen?: (changesetId: string) => void;
}

export function PendingDecisions({ changesets, loading, onOpen }: PendingDecisionsProps) {
  const queue = pendingDecisions(changesets);

  return (
    <Card className="overflow-hidden">
      <div className="flex items-center gap-2.5 border-b bg-muted/30 px-4 py-3">
        <span className="flex size-7 items-center justify-center rounded-md bg-primary/10 text-primary [&_svg]:size-4">
          <Shield />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-medium">Awaiting your decision</h2>
          <p className="text-xs text-muted-foreground">
            Governed proposals in review, with their blast radius over published content.
          </p>
        </div>
        {!loading && queue.length > 0 && (
          <span className="shrink-0 rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium tabular-nums text-primary">
            {queue.length}
          </span>
        )}
      </div>
      <CardContent className="p-0">
        {loading ? (
          <div className="space-y-px">
            {Array.from({ length: 2 }).map((_, i) => (
              <Skeleton key={i} className="h-20 w-full rounded-none" />
            ))}
          </div>
        ) : queue.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 px-6 py-12 text-center">
            <span className="flex size-9 items-center justify-center rounded-full bg-success/10 text-success [&_svg]:size-5">
              <Check />
            </span>
            <p className="text-sm font-medium">Inbox clear</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              No proposals are waiting on a review or a merge. Governed edits land here the moment
              they are submitted.
            </p>
          </div>
        ) : (
          <ul className="divide-y">
            {queue.map((cs) => (
              <PendingDecisionRow
                key={cs.id}
                changeset={cs}
                onOpen={onOpen ? () => onOpen(cs.id) : undefined}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function PendingDecisionRow({ changeset, onOpen }: { changeset: ChangeSet; onOpen?: () => void }) {
  const { nameOf } = useUserDisplayNames();
  const approve = useApproveChangeset(changeset.id);
  const inReview = changeset.status === "in_review";

  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onOpen}
              className="truncate text-left text-sm font-medium text-foreground hover:underline"
            >
              {changeset.name}
            </button>
            <ChangeSetStatusBadge status={changeset.status} className="text-[10px]" />
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {nameOf(changeset.created_by)} · waiting {formatRelative(waitingSince(changeset))}
          </p>
          <BlastRadiusStrip impact={changeset.impact_summary} />
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {inReview && (
            <Button
              size="sm"
              className="h-8"
              disabled={approve.isPending}
              onClick={() => approve.mutate(undefined)}
            >
              {approve.isPending ? <Loader2 className="animate-spin" /> : <Check />}
              {approve.isPending ? "Approving" : "Approve"}
            </Button>
          )}
          <Button size="sm" variant="outline" className="h-8" onClick={onOpen} disabled={!onOpen}>
            {inReview ? "Open" : "Review & merge"}
            <ArrowRight />
          </Button>
        </div>
      </div>
      {approve.isError && (
        <ErrorNotice
          error={approve.error}
          title="Couldn't approve"
          hint="A different reviewer may be required."
          variant="inline"
          className="mt-2"
        />
      )}
    </li>
  );
}

/**
 * The totals the server computed and stored when the change-set was submitted,
 * carried on the list row. A change-set that has not been submitted has none,
 * and a walk that stopped early reports a floor rather than a total.
 */
function BlastRadiusStrip({ impact }: { impact?: ImpactSummary }) {
  if (!impact) {
    return (
      <p className="mt-2 text-xs text-muted-foreground">
        Blast radius is measured when a proposal is submitted.
      </p>
    );
  }
  if (impact.affected_blocks === 0) {
    return (
      <p className="mt-2 text-xs text-muted-foreground">
        No published content is affected, so this is safe to merge.
      </p>
    );
  }
  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      <Metric value={impact.affected_blocks} label="blocks" prefix={impact.partial ? "≥" : ""} />
      <span className="text-border">·</span>
      <Metric value={impact.words} label="words" prefix={impact.partial ? "≥" : ""} />
      {impact.new_violations > 0 && (
        <span className="font-medium text-warning tabular-nums">
          +{impact.new_violations.toLocaleString()} new
        </span>
      )}
      {impact.resolved > 0 && (
        <span className="font-medium text-success tabular-nums">
          −{impact.resolved.toLocaleString()} resolved
        </span>
      )}
      {impact.partial && (
        <span className="text-muted-foreground" title={impact.partial_reason}>
          partial
        </span>
      )}
    </div>
  );
}

function Metric({ value, label, prefix = "" }: { value: number; label: string; prefix?: string }) {
  return (
    <span className={cn("text-muted-foreground")}>
      <span className="font-medium tabular-nums text-foreground">
        {prefix}
        {value.toLocaleString()}
      </span>{" "}
      {label}
    </span>
  );
}
