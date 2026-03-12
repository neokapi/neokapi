import type { StreamMergeResult } from "../types/api";
import { Button } from "./ui/button";
import { Plus, ArrowRight, Trash2, Check } from "./icons";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "./ui/dialog";

export interface StreamMergeDialogProps {
  /** Dry-run merge result with block counts. */
  result: StreamMergeResult;
  /** Name of the stream being merged. */
  streamName: string;
  /** Name of the parent stream receiving the merge. */
  parentName: string;
  /** Called when the user confirms the merge. */
  onConfirm: () => void;
  /** Called to close the dialog. */
  onClose: () => void;
  /** Whether the dialog is open. */
  open: boolean;
}

interface StatRowProps {
  icon: typeof Plus;
  label: string;
  count: number;
  color: string;
}

function StatRow({ icon: Icon, label, count, color }: StatRowProps) {
  if (count === 0) return null;
  return (
    <div className="flex items-center gap-3 py-2">
      <Icon className={`h-4 w-4 shrink-0 ${color}`} />
      <span className="flex-1 text-sm">{label}</span>
      <span className="text-sm font-medium tabular-nums">{count}</span>
    </div>
  );
}

/** Confirmation dialog for merging a stream into its parent. Shows dry-run counts. */
export function StreamMergeDialog({
  result,
  streamName,
  parentName,
  onConfirm,
  onClose,
  open,
}: StreamMergeDialogProps) {
  const totalChanges = result.added_blocks + result.modified_blocks + result.removed_blocks;

  return (
    <Dialog open={open} onOpenChange={(v: boolean) => { if (!v) onClose(); }}>
      <DialogContent size="sm" onInteractOutside={(e: Event) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>Merge Stream</DialogTitle>
          <DialogDescription>
            Merge <span className="font-medium">{streamName}</span> into{" "}
            <span className="font-medium">{parentName}</span>.
          </DialogDescription>
        </DialogHeader>

        <div className="py-2">
          {totalChanges === 0 ? (
            <div className="rounded-lg border border-border/50 p-4 text-center text-sm text-muted-foreground">
              No changes to merge. The streams are identical.
            </div>
          ) : (
            <div className="rounded-lg border border-border/50 divide-y divide-border/50 px-3">
              <StatRow
                icon={Check}
                label="Total blocks merged"
                count={result.merged_blocks}
                color="text-foreground"
              />
              <StatRow
                icon={Plus}
                label="Blocks added"
                count={result.added_blocks}
                color="text-emerald-600 dark:text-emerald-400"
              />
              <StatRow
                icon={ArrowRight}
                label="Blocks modified"
                count={result.modified_blocks}
                color="text-amber-600 dark:text-amber-400"
              />
              <StatRow
                icon={Trash2}
                label="Blocks removed"
                count={result.removed_blocks}
                color="text-red-600 dark:text-red-400"
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={totalChanges === 0}>
            Confirm Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
