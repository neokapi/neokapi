// The detail page for a single change-set / experiment (AD-021): a PR-like
// header with its lifecycle, the ordered operations as a human-readable diff,
// the measured blast radius as the hero, reviews (separation-of-duties aware),
// pilots, and the merge / abandon controls. Governed changes can only merge with
// a second person's approval; the merge confirmation re-shows the blast radius
// and surfaces stale-draft conflicts clearly.
import { useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Skeleton,
} from "@neokapi/ui-primitives";
import { ArrowLeft, Lock, GitMerge, GitPullRequest, Plus, Network } from "../../components/icons";
import type { ChangeSetDetail } from "../../types/brand-graph";
import {
  useChangeset,
  useChangesetBlastRadius,
  useSubmitChangeset,
  useAbandonChangeset,
  useRemoveChangesetOp,
} from "../../hooks/useChangesetsApi";
import { useUserDisplayNames } from "../../hooks/useMembersApi";
import { BrandHub } from "../shell/BrandHub";
import { ChangeSetStatusBadge, EmptyState, formatRelative } from "../shell/atoms";
import { OpsDiff } from "./OpsDiff";
import { OpBuilder } from "./OpBuilder";
import { BlastRadiusPanel } from "./BlastRadiusPanel";
import { ReviewsPanel } from "./ReviewsPanel";
import { PilotsPanel } from "./PilotsPanel";
import { MergeConfirmDialog } from "./MergeConfirmDialog";
import { governedOpCount } from "./ops";

export interface ExperimentDetailViewProps {
  changesetId: string;
  onBack: () => void;
}

export function ExperimentDetailView({ changesetId, onBack }: ExperimentDetailViewProps) {
  const { data: cs, isLoading } = useChangeset(changesetId);
  const { nameOf } = useUserDisplayNames();

  if (isLoading) {
    return (
      <BrandHub title="Experiment" width="wide">
        <Skeleton className="h-64 w-full" />
      </BrandHub>
    );
  }
  if (!cs) {
    return (
      <BrandHub title="Experiment" width="wide">
        <EmptyState
          title="Experiment not found"
          description="This change-set may have been abandoned or removed."
          action={
            <Button size="sm" variant="outline" onClick={onBack}>
              <ArrowLeft />
              Back to experiments
            </Button>
          }
        />
      </BrandHub>
    );
  }

  const governed = governedOpCount(cs.ops);

  return (
    <BrandHub
      title={cs.name}
      description={cs.description || undefined}
      width="wide"
      actions={
        <Button size="sm" variant="outline" onClick={onBack}>
          <ArrowLeft />
          Experiments
        </Button>
      }
      toolbar={
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <ChangeSetStatusBadge status={cs.status} />
          {governed > 0 && (
            <Badge variant="outline" className="gap-1 border-primary/40 text-[10px] text-primary">
              <Lock className="size-3" />
              Governed
            </Badge>
          )}
          <span>by {nameOf(cs.created_by)}</span>
          <span aria-hidden>·</span>
          <span>updated {formatRelative(cs.updated_at)}</span>
          {cs.merged_at && (
            <>
              <span aria-hidden>·</span>
              <span>merged {formatRelative(cs.merged_at)}</span>
            </>
          )}
        </div>
      }
    >
      <div className="space-y-6">
        <LifecycleBar changeset={cs} />
        <div className="grid gap-6 lg:grid-cols-3">
          <div className="space-y-6 lg:col-span-2">
            <BlastRadiusSection changesetId={changesetId} />
            <OpsSection changeset={cs} />
          </div>
          <div className="space-y-6">
            <ReviewsPanel changeset={cs} />
            <PilotsPanel changeset={cs} />
          </div>
        </div>
      </div>
    </BrandHub>
  );
}

// ── Lifecycle bar ────────────────────────────────────────────────────────────

function LifecycleBar({ changeset }: { changeset: ChangeSetDetail }) {
  const submit = useSubmitChangeset(changeset.id);
  const abandon = useAbandonChangeset(changeset.id);
  const [mergeOpen, setMergeOpen] = useState(false);

  const terminal = changeset.status === "merged" || changeset.status === "abandoned";
  const busy = submit.isPending || abandon.isPending;

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/20 p-3">
      <span className="text-sm text-muted-foreground">{lifecycleHint(changeset)}</span>
      <div className="ml-auto flex flex-wrap items-center gap-2">
        {changeset.status === "draft" && (
          <Button
            size="sm"
            onClick={() => submit.mutate()}
            disabled={busy || changeset.ops.length === 0}
          >
            <GitPullRequest />
            Submit for review
          </Button>
        )}
        {changeset.status === "approved" && (
          <Button size="sm" onClick={() => setMergeOpen(true)}>
            <GitMerge />
            Merge
          </Button>
        )}
        {!terminal && (
          <Button
            size="sm"
            variant="ghost"
            className="text-muted-foreground hover:text-destructive"
            onClick={() => abandon.mutate()}
            disabled={busy}
          >
            Abandon
          </Button>
        )}
      </div>
      <MergeConfirmDialog
        open={mergeOpen}
        onOpenChange={setMergeOpen}
        changesetId={changeset.id}
        changesetName={changeset.name}
      />
    </div>
  );
}

function lifecycleHint(cs: ChangeSetDetail): string {
  switch (cs.status) {
    case "draft":
      return cs.ops.length === 0
        ? "Add operations to this draft, then submit it for review."
        : "Ready to submit for review.";
    case "in_review":
      return "Awaiting approval from someone other than the author.";
    case "approved":
      return "Approved — merge to apply these changes to the live graph.";
    case "merged":
      return cs.merged_at ? `Merged ${formatRelative(cs.merged_at)}.` : "Merged.";
    case "abandoned":
      return "Abandoned. No changes were applied.";
  }
}

// ── Blast radius (the hero) ──────────────────────────────────────────────────

function BlastRadiusSection({ changesetId }: { changesetId: string }) {
  const { data, isLoading } = useChangesetBlastRadius(changesetId);
  return (
    <section className="space-y-3">
      <div className="flex items-center gap-2">
        <Network className="size-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">Blast radius</h2>
      </div>
      <BlastRadiusPanel impact={data} isLoading={isLoading} />
    </section>
  );
}

// ── Operations ───────────────────────────────────────────────────────────────

function OpsSection({ changeset }: { changeset: ChangeSetDetail }) {
  const remove = useRemoveChangesetOp(changeset.id);
  const editable = changeset.status === "draft";
  const [addOpen, setAddOpen] = useState(false);

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">Operations</h3>
          {editable && (
            <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
              <Plus />
              Add operation
            </Button>
          )}
        </div>
        <OpsDiff
          ops={changeset.ops}
          editable={editable}
          onRemove={(seq) => remove.mutate(seq)}
          removingSeq={remove.isPending ? (remove.variables ?? null) : null}
        />
        <AddOpDialog changesetId={changeset.id} open={addOpen} onOpenChange={setAddOpen} />
      </CardContent>
    </Card>
  );
}

function AddOpDialog({
  changesetId,
  open,
  onOpenChange,
}: {
  changesetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Add an operation</DialogTitle>
        </DialogHeader>
        <OpBuilder changesetId={changesetId} />
        <div className="flex justify-end">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Done
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
