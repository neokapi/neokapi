import {
  Button,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  cn,
} from "@neokapi/ui-primitives";
import { BlockInspector } from "@neokapi/ui-primitives/preview";
import type { ContentNode } from "@neokapi/ui-primitives/preview";
import type { BlockInfo, BlockTermMatch, QAIssue, ReviewContext } from "../../types/api";
import { CollapsedTargetCell } from "../editor/GridTargetRenderer";
import { UnifiedTargetEditor, type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { getBlockStatus, getTargetText, statusConfig } from "../editor/blockStatus";
import {
  ContextHeading,
  FindingsList,
  MemoryMatchCard,
  PointRail,
  ProvenanceBlock,
} from "./reviewContext";
import { Check, Pencil, X } from "../icons";

export interface ReviewInspectorProps {
  /** The block under review; null closes the panel. */
  block: BlockInfo | null;
  /** The projected content node for `block` — source runs, targets, overlays. */
  node: ContentNode | null;
  /** The item the block belongs to, shown as the panel's subject. */
  itemName: string;
  /** The locale being reviewed. */
  locale: string;
  /** Display name for `locale`. */
  localeLabel: string;
  /** QA findings for this block from the file's last QA run. */
  issues: QAIssue[];
  /** Term hits over this block's source, loaded when the panel opens. */
  terms: BlockTermMatch[];
  /** The term lookup is still in flight. */
  termsLoading?: boolean;
  /**
   * The context the server resolved for this block: the content-memory match
   * with its wording, the last decision and its note, the target's origin, and
   * the findings behind its voice score. Null until it lands.
   */
  context?: ReviewContext | null;
  /** The target editor is open. */
  editing: boolean;
  /** A review decision or a batch is in flight — decisions are held. */
  busy?: boolean;
  /**
   * Whether the viewer may approve this language. Approving needs the `review`
   * permission, which a translator does not hold. Defaults to true.
   */
  canApprove?: boolean;
  /** The block is in the batch the bulk actions will carry. */
  marked: boolean;
  onClose: () => void;
  onApprove: () => void;
  onReject: () => void;
  onEditToggle: () => void;
  onSaveEdit: (result: UnifiedSaveResult) => void | Promise<void>;
  onCancelEdit: () => void;
  onToggleMark: () => void;
}

/**
 * ReviewInspector is the review surface's slide-in detail: the document stays
 * the surface, and one block's particulars — its run sequence, every overlay
 * interpreting it, its QA findings and term hits, and the decision itself —
 * arrive beside it rather than replacing it.
 *
 * The content-model view is the shared kit's BlockInspector, so a block reads
 * here exactly as it reads in the desktop app's preview. Everything below it is
 * the review act: edit the target, approve, reject, or hold the block for the
 * batch.
 */
export function ReviewInspector({
  block,
  node,
  itemName,
  locale,
  localeLabel,
  issues,
  terms,
  termsLoading,
  context = null,
  editing,
  busy,
  canApprove = true,
  marked,
  onClose,
  onApprove,
  onReject,
  onEditToggle,
  onSaveEdit,
  onCancelEdit,
  onToggleMark,
}: ReviewInspectorProps) {
  const status = block ? getBlockStatus(block, locale) : "not-started";
  // An empty translation is nothing to approve (the server 422s it) and nothing
  // to reject (clearing it is a server-side no-op).
  const hasTarget = block ? getTargetText(block, locale).trim().length > 0 : false;

  return (
    // Non-modal on purpose: the document is the surface, so it stays readable
    // and clickable with the inspector open — the reader moves to the next
    // block by reading on, and the panel follows.
    <Sheet open={!!block} onOpenChange={(open) => !open && onClose()} modal={false}>
      <SheetContent
        side="right"
        className="gap-3 data-[side=right]:w-full data-[side=right]:sm:w-3/4 data-[side=right]:sm:max-w-none data-[side=right]:lg:w-[42rem]"
        data-testid="review-inspector"
        // Reading on is not dismissing: a click inside the document moves the
        // inspector rather than closing it. Anywhere else still dismisses.
        onInteractOutside={(e) => {
          const target = e.target as HTMLElement | null;
          if (target?.closest('[data-testid="review-document"]')) e.preventDefault();
        }}
      >
        <SheetHeader className="pb-0">
          <SheetTitle className="flex flex-wrap items-center gap-2 text-sm">
            <span className="truncate font-mono" translate="no">
              {block?.id}
            </span>
            <span
              className={cn(
                "rounded px-1.5 py-0.5 text-[10px] font-semibold",
                statusConfig[status].className,
              )}
              data-testid={block ? `review-status-${block.id}` : undefined}
            >
              {statusConfig[status].label}
            </span>
          </SheetTitle>
          <SheetDescription>
            {itemName} · {localeLabel}
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-auto px-4 pb-4">
          {/* The content model, collapsed to a one-liner: the sections below say
              what a reviewer acts on, and this expands to what the block
              actually holds — runs, every target, and the overlays behind the
              marks in the document. */}
          {node && <BlockInspector node={node} />}

          {/* Target — read as rendered, or opened for correction. */}
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Target ({locale})
              </span>
              {!editing && block && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-6 px-2 text-xs"
                  onClick={onEditToggle}
                  data-testid="inspector-edit"
                >
                  <Pencil className="mr-1 h-3 w-3" /> Edit
                  <kbd className="ml-1.5 rounded border border-border px-1 text-[10px]">E</kbd>
                </Button>
              )}
            </div>
            {block && editing ? (
              <div
                className="rounded-lg border border-primary/40 bg-card p-2"
                data-testid="inspector-editor"
              >
                <UnifiedTargetEditor
                  block={block}
                  locale={locale}
                  onSave={onSaveEdit}
                  onCancel={onCancelEdit}
                  compact
                />
              </div>
            ) : (
              block && (
                <div className="rounded-lg border border-border bg-card p-3 text-sm leading-relaxed">
                  {hasTarget ? (
                    <CollapsedTargetCell
                      block={block}
                      locale={locale}
                      testId={`review-target-${block.id}`}
                    />
                  ) : (
                    <span className="text-muted-foreground">
                      No translation yet for this locale.
                    </span>
                  )}
                </div>
              )
            )}
          </section>

          {/* What governs this point: the same rail the queue draws, fed the
              term hits this surface already looked up — the ones behind the
              inline marks in the document, so the rail and the marks cannot
              disagree. */}
          <div data-testid="inspector-point">
            <PointRail context={context} terms={terms} termsLoading={termsLoading} />
          </div>

          {/* Findings — the check results and the findings behind the block's
              voice score, read as one list. A positioned finding is already
              marked in the document; each says what it was raised against and
              what to say instead. */}
          <section className="space-y-2" data-testid="inspector-qa">
            <ContextHeading>Findings</ContextHeading>
            <FindingsList issues={issues} findings={context?.voice_findings ?? []} />
          </section>

          {/* Content memory — the wording the corpus already blessed for this
              source. The bulk pass writes these; the reviewer reads them. */}
          <section className="space-y-2" data-testid="inspector-memory">
            <ContextHeading>Content memory</ContextHeading>
            <MemoryMatchCard
              match={context?.memory_match}
              onUse={
                context?.memory_match && block && !editing
                  ? () =>
                      void onSaveEdit({
                        kind: "flat",
                        codedText: context.memory_match?.target ?? "",
                        spans: [],
                      })
                  : undefined
              }
            />
          </section>

          {/* Provenance — how this target was produced, and what was last
              decided about it. */}
          <section className="space-y-2" data-testid="inspector-provenance">
            <ContextHeading>Provenance</ContextHeading>
            <ProvenanceBlock
              origin={context?.origin}
              decision={context?.decision}
              note={context?.notes?.[context.notes.length - 1]?.text}
            />
          </section>
        </div>

        {/* Decision bar */}
        <div className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-2.5">
          <Button
            size="sm"
            onClick={onApprove}
            disabled={busy || status === "reviewed" || !hasTarget || !canApprove}
            title={
              canApprove ? undefined : "Approving needs the review permission for this language"
            }
            className="bg-success text-white hover:bg-success/90"
            data-testid={block ? `approve-${block.id}` : undefined}
          >
            <Check className="mr-1 h-4 w-4" /> Approve
            <kbd className="ml-1.5 rounded bg-white/20 px-1 text-[10px]">A</kbd>
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={onReject}
            disabled={busy || !hasTarget}
            className="border-destructive/40 text-destructive hover:bg-destructive/10"
            data-testid={block ? `reject-${block.id}` : undefined}
          >
            <X className="mr-1 h-4 w-4" /> Reject
            <kbd className="ml-1.5 rounded bg-destructive/15 px-1 text-[10px]">R</kbd>
          </Button>
          <div className="flex-1" />
          <Button
            size="sm"
            variant={marked ? "default" : "outline"}
            onClick={onToggleMark}
            data-testid={block ? `review-mark-${block.id}` : undefined}
            aria-pressed={marked}
          >
            {marked ? "In batch" : "Add to batch"}
            <kbd
              className={cn(
                "ml-1.5 rounded px-1 text-[10px]",
                marked ? "bg-white/20" : "border border-border",
              )}
            >
              M
            </kbd>
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
