import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  cn,
} from "@neokapi/ui-primitives";
import type { AppliedMemory, BlockInfo } from "../../types/api";
import { memoryScoreClass } from "../editor/blockStatus";

export interface ApplyMemoryDialogProps {
  /**
   * What the pass answered it would write, or null when no preview is open.
   * An empty array is an answer: the batch matches nothing.
   */
  preview: AppliedMemory[] | null;
  /** The page's blocks, so each match can be shown against the source it replaces. */
  blocks: BlockInfo[];
  /** The locale the wording lands in. */
  locale: string;
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

/**
 * What a bulk content-memory pass is about to write, before it writes it.
 *
 * The pass answers for itself: the list here is the preview the same handler
 * returns, so what the reviewer reads and what lands are computed once. Each
 * row pairs the source it matched with the wording that will replace the
 * target, because a match the corpus holds for a different context reads wrong
 * only when someone can see it.
 */
export function ApplyMemoryDialog({
  preview,
  blocks,
  locale,
  busy,
  onCancel,
  onConfirm,
}: ApplyMemoryDialogProps) {
  const sourceOf = (blockId: string) => blocks.find((b) => b.id === blockId)?.source ?? blockId;
  const count = preview?.length ?? 0;

  return (
    <Dialog open={preview !== null} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="sm:max-w-[560px]" data-testid="apply-memory-dialog">
        <DialogHeader>
          <DialogTitle>
            {count === 0
              ? "Nothing to apply"
              : `Apply ${count} content-memory match${count === 1 ? "" : "es"}?`}
          </DialogTitle>
          <DialogDescription>
            {count === 0
              ? "No block in this batch has a match at the exact-match threshold."
              : `Each of these targets in ${locale} will be replaced with the wording the content memory holds.`}
          </DialogDescription>
        </DialogHeader>

        {count > 0 && (
          <ul
            className="max-h-72 space-y-2 overflow-auto rounded-md border border-border bg-muted/20 p-2"
            data-testid="apply-memory-list"
          >
            {preview?.map((item) => (
              <li key={item.block_id} className="space-y-0.5 text-xs">
                <div className="flex items-center gap-2">
                  <span
                    className={cn(
                      "rounded px-1.5 py-px text-[11px] font-bold tabular-nums",
                      memoryScoreClass(item.score),
                    )}
                  >
                    {Math.round(item.score * 100)}%
                  </span>
                  <span className="truncate text-muted-foreground">{sourceOf(item.block_id)}</span>
                </div>
                <p className="font-medium">{item.text}</p>
              </li>
            ))}
          </ul>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
          <Button
            onClick={onConfirm}
            disabled={busy || count === 0}
            data-testid="apply-memory-confirm"
          >
            Apply {count > 0 ? count : ""}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
