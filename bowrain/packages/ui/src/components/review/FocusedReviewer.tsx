import { useCallback, useState } from "react";
import { Button, Badge, cn } from "@neokapi/ui-primitives";
import type { OnBrandBasis } from "../../types/api";
import { OnBrandRateChip } from "../OnBrandRateChip";
import { SourceCellDisplay } from "../editor/SourceCellDisplay";
import { HighlightedSource } from "../editor/HighlightedSource";
import { CollapsedTargetCell } from "../editor/GridTargetRenderer";
import { UnifiedTargetEditor, type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { getBlockStatus, statusConfig } from "../editor/blockStatus";
import {
  Check,
  X,
  Pencil,
  RefreshCw,
  Tag,
  Wand2,
  CircleCheck,
  AlertTriangle,
  Info,
} from "../icons";
import { entryCheckStatus, type ReviewEntry } from "./reviewQueue";

/** Locale-level on-brand context for the reviewer header chip. */
export interface ReviewerOnBrand {
  rate: number;
  basis: OnBrandBasis;
  onBrandBlocks?: number;
  translatedBlocks?: number;
}

export interface FocusedReviewerProps {
  /** The pending block being reviewed. */
  entry: ReviewEntry;
  /** Source language code, for the source column label. */
  sourceLocale: string;
  /** 1-based position + total for the "3 of 12" header. */
  position: { index: number; total: number };
  /** Locale display-name resolver. */
  localeName?: (locale: string) => string;
  /** Locale-level on-brand context; renders the header chip when present. */
  onBrand?: ReviewerOnBrand;
  /** In edit mode the target column shows the inline-code editor. */
  editing: boolean;
  /** Disables the action buttons while a decision is in flight. */
  busy?: boolean;
  /** Shows the re-check spinner. */
  reChecking?: boolean;
  /** When set, the source-side + brand-correction affordances are enabled. */
  brandProfileId?: string;
  onApprove: () => void;
  onReject: () => void;
  onEditToggle: () => void;
  onSaveEdit: (result: UnifiedSaveResult) => void | Promise<void>;
  onCancelEdit: () => void;
  onReCheck: () => void;
  /** Mark the selected source text as a term (governance-aware, in the parent). */
  onMarkTerm: (sourceText: string) => void;
  /** Suggest a brand/voice rule from the selected source text. */
  onSuggestBrandRule: (sourceText: string) => void;
  /** Turn the reviewer's target fix into a brand-voice correction/rule. */
  onMakeRule: () => void;
}

const checkChip: Record<string, { label: string; className: string }> = {
  failing: {
    label: "Failing checks",
    className: "border-destructive/40 bg-destructive/10 text-destructive",
  },
  off_brand: { label: "Off-brand", className: "border-warning/40 bg-warning/10 text-warning" },
  clean: { label: "Passes checks", className: "border-success/40 bg-success/10 text-success" },
};

/**
 * FocusedReviewer is the review session's right pane: one pending block shown
 * source-vs-target, rendered faithfully through the same content-model
 * primitives the translation editor uses (inline codes as protected tokens,
 * term/entity overlays), with its checks and on-brand signal inline. Review is
 * bidirectional — the reviewer can act on the target (approve / reject / edit →
 * re-check, and turn a fix into a brand rule) and on the source (select a span
 * → mark a term or suggest a brand rule). All actions are emitted to the parent
 * session, which owns the data, keyboard model, and governance.
 */
export function FocusedReviewer({
  entry,
  sourceLocale,
  position,
  localeName,
  onBrand,
  editing,
  busy,
  reChecking,
  brandProfileId,
  onApprove,
  onReject,
  onEditToggle,
  onSaveEdit,
  onCancelEdit,
  onReCheck,
  onMarkTerm,
  onSuggestBrandRule,
  onMakeRule,
}: FocusedReviewerProps) {
  const { block, locale, issues } = entry;
  const status = getBlockStatus(block, locale);
  const check = entryCheckStatus(entry);
  const chip = checkChip[check];
  const [selection, setSelection] = useState("");

  const localeLabel = localeName ? `${localeName(locale)} (${locale})` : locale;

  // Capture the current source-text selection so the source-side lane can act
  // on it. Constrained to the source column via the onMouseUp target.
  const captureSelection = useCallback(() => {
    const text = window.getSelection?.()?.toString().trim() ?? "";
    setSelection(text);
  }, []);

  const errorCount = issues.filter((i) => i.severity === "error").length;

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="focused-reviewer">
      {/* Header: identity, position, status, on-brand */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        <span className="truncate text-sm font-semibold" title={entry.itemName}>
          {entry.itemName}
        </span>
        <Badge variant="outline" className="font-normal">
          {localeLabel}
        </Badge>
        <span
          className={cn(
            "rounded px-1.5 py-0.5 text-[10px] font-semibold",
            statusConfig[status].className,
          )}
        >
          {statusConfig[status].label}
        </span>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
            chip.className,
          )}
          data-testid={`reviewer-check-${check}`}
        >
          {check === "clean" ? (
            <CircleCheck className="h-3 w-3" />
          ) : (
            <AlertTriangle className="h-3 w-3" />
          )}
          {chip.label}
        </span>
        {onBrand && (
          <OnBrandRateChip
            rate={onBrand.rate}
            basis={onBrand.basis}
            onBrandBlocks={onBrand.onBrandBlocks}
            translatedBlocks={onBrand.translatedBlocks}
          />
        )}
        <div className="flex-1" />
        <span
          className="text-xs tabular-nums text-muted-foreground"
          data-testid="reviewer-position"
        >
          {position.index} of {position.total}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        {/* Source vs target, generous side-by-side */}
        <div className="grid gap-4 lg:grid-cols-2">
          {/* Source */}
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Source ({sourceLocale})
              </span>
            </div>
            <div
              className="rounded-lg border border-border bg-muted/20 p-3 text-sm leading-relaxed"
              onMouseUp={captureSelection}
              onKeyUp={captureSelection}
              data-testid="reviewer-source"
            >
              {block.has_spans && block.source_coded ? (
                <SourceCellDisplay
                  codedText={block.source_coded}
                  spans={block.source_spans ?? []}
                  entities={block.entities}
                />
              ) : (
                <HighlightedSource text={block.source} termMatches={[]} entities={block.entities} />
              )}
            </div>
            {/* Source-side review lane */}
            <div className="flex flex-wrap items-center gap-2" data-testid="source-lane">
              {selection ? (
                <>
                  <span
                    className="max-w-[16rem] truncate text-xs text-muted-foreground"
                    title={selection}
                  >
                    &ldquo;{selection}&rdquo;
                  </span>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 px-2 text-xs"
                    onClick={() => onMarkTerm(selection)}
                    data-testid="source-mark-term"
                  >
                    <Tag className="mr-1 h-3.5 w-3.5" /> Mark as term
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 px-2 text-xs"
                    disabled={!brandProfileId}
                    onClick={() => onSuggestBrandRule(selection)}
                    data-testid="source-suggest-rule"
                  >
                    <Wand2 className="mr-1 h-3.5 w-3.5" /> Suggest brand rule
                  </Button>
                </>
              ) : (
                <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                  <Info className="h-3 w-3" /> Select source text to mark a term or suggest a rule.
                </span>
              )}
            </div>
          </section>

          {/* Target */}
          <section className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Target ({locale})
              </span>
              {!editing && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-6 px-2 text-xs"
                  onClick={onEditToggle}
                  data-testid="reviewer-edit"
                >
                  <Pencil className="mr-1 h-3 w-3" /> Edit
                </Button>
              )}
            </div>
            {editing ? (
              <div
                className="rounded-lg border border-primary/40 bg-card p-2"
                data-testid="reviewer-editor"
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
              <div
                className="rounded-lg border border-border bg-card p-3 text-sm leading-relaxed"
                data-testid="reviewer-target"
              >
                <CollapsedTargetCell block={block} locale={locale} testId="reviewer-target-cell" />
              </div>
            )}
            {brandProfileId && !editing && (
              <Button
                size="sm"
                variant="ghost"
                className="h-7 px-2 text-xs text-muted-foreground"
                onClick={onMakeRule}
                data-testid="reviewer-make-rule"
              >
                <Wand2 className="mr-1 h-3.5 w-3.5" /> Turn a fix into a brand rule
              </Button>
            )}
          </section>
        </div>

        {/* Checks + on-brand, inline: why this block is flagged (or why it passed) */}
        <section className="mt-4 space-y-2" data-testid="reviewer-checks">
          <div className="flex items-center gap-2">
            <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Checks
            </span>
            <Button
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-xs"
              onClick={onReCheck}
              disabled={reChecking}
              data-testid="reviewer-recheck"
            >
              <RefreshCw className={cn("mr-1 h-3 w-3", reChecking && "animate-spin")} /> Re-check
            </Button>
          </div>
          {issues.length === 0 ? (
            <div className="flex items-center gap-2 rounded-md border border-success/25 bg-success/5 px-3 py-2 text-sm text-success">
              <CircleCheck className="h-4 w-4 shrink-0" />
              Passes checks{onBrand ? " and the on-brand bar" : ""}.
            </div>
          ) : (
            <ul className="space-y-1 rounded-md border border-border bg-muted/20 p-2">
              {issues.map((issue, i) => (
                <li key={i} className="flex items-start gap-1.5 text-xs">
                  <AlertTriangle
                    className={cn(
                      "mt-0.5 h-3 w-3 shrink-0",
                      issue.severity === "error" ? "text-destructive" : "text-warning",
                    )}
                  />
                  <span
                    className={issue.severity === "error" ? "text-destructive" : "text-warning"}
                  >
                    <span className="font-medium">{issue.type}:</span> {issue.message}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>

      {/* Decision bar + keyboard hints */}
      <div className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-2.5">
        <Button
          size="sm"
          onClick={onApprove}
          disabled={busy}
          className="bg-success text-white hover:bg-success/90"
          data-testid="reviewer-approve"
        >
          <Check className="mr-1 h-4 w-4" /> Approve
          <kbd className="ml-1.5 rounded bg-white/20 px-1 text-[10px]">A</kbd>
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={onReject}
          disabled={busy}
          className="border-destructive/40 text-destructive hover:bg-destructive/10"
          data-testid="reviewer-reject"
        >
          <X className="mr-1 h-4 w-4" /> Reject
          <kbd className="ml-1.5 rounded bg-destructive/15 px-1 text-[10px]">R</kbd>
        </Button>
        <div className="flex-1" />
        <span
          className="hidden items-center gap-3 text-[11px] text-muted-foreground sm:flex"
          aria-hidden
        >
          <span>
            <kbd className="rounded border border-border px-1">J</kbd>/
            <kbd className="rounded border border-border px-1">K</kbd> move
          </span>
          <span>
            <kbd className="rounded border border-border px-1">A</kbd> approve
          </span>
          <span>
            <kbd className="rounded border border-border px-1">R</kbd> reject
          </span>
          <span>
            <kbd className="rounded border border-border px-1">E</kbd> edit
          </span>
          <span className="text-muted-foreground/70">
            {errorCount > 0 ? `${errorCount} error(s)` : ""}
          </span>
        </span>
      </div>
    </div>
  );
}
