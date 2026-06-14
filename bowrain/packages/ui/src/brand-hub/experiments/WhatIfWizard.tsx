// The what-if wizard (AD-021): compose a change-set without leaving the page.
// Name the experiment (which creates a real draft), build its operations with
// the op builder, and watch the blast radius refresh live as each op lands —
// turning a what-if into a measured experiment before you submit it for review.
import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Textarea,
} from "@neokapi/ui-primitives";
import { FlaskConical, Lock, GitPullRequest, ArrowRight } from "../../components/icons";
import {
  useCreateChangeset,
  useChangeset,
  useChangesetBlastRadius,
  useSubmitChangeset,
  useRemoveChangesetOp,
} from "../../hooks/useChangesetsApi";
import { OpBuilder } from "./OpBuilder";
import { OpsDiff } from "./OpsDiff";
import { BlastRadiusPanel } from "./BlastRadiusPanel";
import { governedOpCount } from "./ops";

export interface WhatIfWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the draft's id when the user opens it (saved or submitted). */
  onSubmitted: (changesetId: string) => void;
}

export function WhatIfWizard({ open, onOpenChange, onSubmitted }: WhatIfWizardProps) {
  const [draftId, setDraftId] = useState<string | null>(null);

  // Reset when the dialog is freshly opened so each "New experiment" starts clean.
  useEffect(() => {
    if (open) setDraftId(null);
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] gap-0 overflow-hidden p-0 sm:max-w-4xl">
        <DialogHeader className="border-b px-5 py-4">
          <DialogTitle className="flex items-center gap-2">
            <FlaskConical className="size-4 text-primary" />
            Compose an experiment
          </DialogTitle>
        </DialogHeader>
        {draftId ? (
          <ComposeStep
            draftId={draftId}
            onSubmitted={onSubmitted}
            onClose={() => onOpenChange(false)}
          />
        ) : (
          <NameStep onCreated={setDraftId} />
        )}
      </DialogContent>
    </Dialog>
  );
}

// ── Step 1: name the experiment (creates the draft) ──────────────────────────

function NameStep({ onCreated }: { onCreated: (id: string) => void }) {
  const create = useCreateChangeset();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const canSubmit = name.trim().length > 0 && !create.isPending;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    create.mutate(
      { name: name.trim(), description: description.trim() || undefined },
      { onSuccess: (cs) => onCreated(cs.id) },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4 p-5">
      <p className="text-sm text-muted-foreground">
        Give the experiment a name. It starts as a draft you can keep building or abandon — nothing
        touches the live graph until it merges.
      </p>
      <div className="space-y-1.5">
        <Label htmlFor="wiz-name">Name</Label>
        <Input
          id="wiz-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Retire ‘utilize’ across DACH"
          autoFocus
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="wiz-desc">Description</Label>
        <Textarea
          id="wiz-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={3}
          placeholder="What this change proposes and why."
        />
      </div>
      {create.isError && (
        <p className="text-sm text-destructive">
          {create.error instanceof Error ? create.error.message : "Could not start the experiment."}
        </p>
      )}
      <div className="flex justify-end">
        <Button type="submit" disabled={!canSubmit}>
          {create.isPending ? "Starting…" : "Start composing"}
          <ArrowRight />
        </Button>
      </div>
    </form>
  );
}

// ── Step 2: build ops + watch the blast radius ───────────────────────────────

function ComposeStep({
  draftId,
  onSubmitted,
  onClose,
}: {
  draftId: string;
  onSubmitted: (id: string) => void;
  onClose: () => void;
}) {
  const { data: cs } = useChangeset(draftId);
  const ops = cs?.ops ?? [];
  const hasOps = ops.length > 0;
  const governed = governedOpCount(ops);

  const {
    data: impact,
    isLoading: impactLoading,
    isFetching,
  } = useChangesetBlastRadius(draftId, hasOps);
  const submit = useSubmitChangeset(draftId);
  const remove = useRemoveChangesetOp(draftId);

  return (
    <div className="flex max-h-[calc(88vh-8rem)] flex-col">
      <div className="grid flex-1 gap-0 overflow-hidden md:grid-cols-2">
        {/* Left: build operations */}
        <div className="space-y-5 overflow-y-auto border-b p-5 md:border-r md:border-b-0">
          <OpBuilder changesetId={draftId} />
          <div className="space-y-2">
            <h3 className="text-sm font-medium">This experiment</h3>
            <OpsDiff
              ops={ops}
              editable
              onRemove={(seq) => remove.mutate(seq)}
              removingSeq={remove.isPending ? (remove.variables ?? null) : null}
            />
          </div>
        </div>

        {/* Right: live blast radius */}
        <div className="overflow-y-auto bg-muted/10 p-5">
          {hasOps ? (
            <BlastRadiusPanel
              impact={impact}
              isLoading={impactLoading}
              caption={isFetching ? "Live preview · updating…" : "Live preview"}
            />
          ) : (
            <div className="flex h-full min-h-40 flex-col items-center justify-center gap-2 text-center">
              <FlaskConical className="size-7 text-muted-foreground" />
              <p className="text-sm font-medium">Add an operation</p>
              <p className="max-w-xs text-sm text-muted-foreground">
                The blast radius over your stored content appears here and refreshes as you build.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="flex flex-wrap items-center gap-2 border-t px-5 py-3">
        {governed > 0 && (
          <Badge variant="outline" className="gap-1 border-primary/40 text-[10px] text-primary">
            <Lock className="size-3" />
            Governed — needs a second approval
          </Badge>
        )}
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" onClick={() => onSubmitted(draftId)}>
            Save &amp; open
          </Button>
          <Button
            onClick={() =>
              submit.mutate(undefined, {
                onSuccess: () => {
                  onClose();
                  onSubmitted(draftId);
                },
              })
            }
            disabled={!hasOps || submit.isPending}
          >
            <GitPullRequest />
            {submit.isPending ? "Submitting…" : "Submit for review"}
          </Button>
        </div>
      </div>
    </div>
  );
}
