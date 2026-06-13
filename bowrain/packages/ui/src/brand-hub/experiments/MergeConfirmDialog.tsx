// Merge confirmation (AD-021): merging applies a change-set's ops to the live
// graph, so we re-show the blast radius one last time before the user commits —
// and if the merge hits stale-draft conflicts (409 + OpConflict list), we
// surface them clearly with the re-base guidance rather than a raw error string.
import { useState } from "react";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  cn,
} from "@neokapi/ui-primitives";
import { GitMerge, AlertTriangle } from "../../components/icons";
import type { OpConflict } from "../../types/brand-graph";
import { useChangesetBlastRadius, useMergeChangeset } from "../../hooks/useChangesetsApi";
import { BlastRadiusPanel } from "./BlastRadiusPanel";
import { parseMergeError } from "./merge";

export interface MergeConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  changesetId: string;
  changesetName: string;
  onMerged?: () => void;
}

export function MergeConfirmDialog({
  open,
  onOpenChange,
  changesetId,
  changesetName,
  onMerged,
}: MergeConfirmDialogProps) {
  const { data: impact, isLoading } = useChangesetBlastRadius(changesetId, open);
  const merge = useMergeChangeset(changesetId);
  const [conflicts, setConflicts] = useState<OpConflict[]>([]);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const onMerge = () => {
    setConflicts([]);
    setErrorMsg(null);
    merge.mutate(undefined, {
      onSuccess: (result) => {
        if (result.conflicts && result.conflicts.length > 0) {
          setConflicts(result.conflicts);
          return;
        }
        onMerged?.();
        onOpenChange(false);
      },
      onError: (err) => {
        const parsed = parseMergeError(err);
        setConflicts(parsed.conflicts);
        setErrorMsg(parsed.conflicts.length > 0 ? null : parsed.message);
      },
    });
  };

  const blocked = conflicts.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GitMerge className="size-4" />
            Merge “{changesetName}”
          </DialogTitle>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          Merging applies these operations to the live graph and retires any pilots. Here is the
          measured impact one last time.
        </p>

        <BlastRadiusPanel impact={impact} isLoading={isLoading} hideSamples />

        {blocked && <ConflictView conflicts={conflicts} />}

        {errorMsg && (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" />
            {errorMsg}
          </p>
        )}

        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {blocked ? "Close" : "Cancel"}
          </Button>
          <Button onClick={onMerge} disabled={merge.isPending}>
            <GitMerge />
            {merge.isPending ? "Merging…" : blocked ? "Retry merge" : "Merge experiment"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ConflictView({ conflicts }: { conflicts: OpConflict[] }) {
  return (
    <div className={cn("space-y-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3")}>
      <div className="flex items-center gap-2">
        <AlertTriangle className="size-4 text-destructive" />
        <h3 className="text-sm font-medium text-destructive">
          {conflicts.length} stale-draft conflict{conflicts.length === 1 ? "" : "s"}
        </h3>
      </div>
      <p className="text-xs text-muted-foreground">
        These operations were authored against concept revisions that have since changed. Re-base
        them — reopen the op, re-validate against the current concept, and resubmit — then merge
        again.
      </p>
      <ul className="space-y-1.5">
        {conflicts.map((c) => (
          <li
            key={c.seq}
            className="flex items-start gap-2 rounded-md border bg-card px-3 py-2 text-sm"
          >
            <Badge variant="outline" className="shrink-0 font-mono text-[10px]">
              op #{c.seq}
            </Badge>
            <div className="min-w-0">
              <div className="truncate font-medium text-foreground">{c.concept_id}</div>
              <p className="text-xs text-muted-foreground">{c.reason}</p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
