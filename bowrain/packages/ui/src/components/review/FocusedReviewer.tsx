import { useCallback, useMemo, useState } from "react";
import { Button, Badge, cn, directionAttrs } from "@neokapi/ui-primitives";
import type { EntityInfo, ComplianceBasis, ReviewContext } from "../../types/api";
import { ComplianceRateChip } from "../ComplianceRateChip";
import { entityLabel } from "../editor/entityMarks";
import { FormattedSourceDisplay } from "../editor/FormattedSourceDisplay";
import { CollapsedTargetCell } from "../editor/GridTargetRenderer";
import { UnifiedTargetEditor, type UnifiedSaveResult } from "../UnifiedTargetEditor";
import { getBlockStatus, getTargetText, statusConfig } from "../editor/blockStatus";
import { blockToContentNode } from "../../preview/toContentTree";
import {
  AnchoredTarget,
  ContextHeading,
  FindingsList,
  MemoryMatchCard,
  NeighbourCell,
  PointRail,
  ProvenanceBlock,
} from "./reviewContext";
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
  FileText,
} from "../icons";
import {
  BLOCKER_LABELS,
  entryBlockers,
  entryVerdict,
  VERDICT_LABELS,
  type ReviewEntry,
  type ReviewQueueVerdict,
} from "./reviewQueue";

/** Locale-level compliance context for the reviewer header chip. */
export interface ReviewerCompliance {
  rate: number;
  basis: ComplianceBasis;
  compliantBlocks?: number;
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
  /** Locale-level compliance context; renders the header chip when present. */
  compliance?: ReviewerCompliance;
  /** In edit mode the target column shows the inline-code editor. */
  editing: boolean;
  /** Disables the action buttons while a decision is in flight. */
  busy?: boolean;
  /**
   * Whether the viewer may approve this language. Approving needs the `review`
   * permission, which a translator does not hold, so the button is disabled and
   * says why rather than failing on click. Defaults to true.
   */
  canApprove?: boolean;
  /** Shows the re-check spinner. */
  reChecking?: boolean;
  /** When set, the source-side + brand-correction affordances are enabled. */
  voiceProfileId?: string;
  /**
   * The five layers of context the server resolved for this unit: what governs
   * it, what surrounds it, what the corpus and the ledger already said, what
   * the scoring pass found, and how the target was produced. Null while the
   * fetch is in flight or when it failed — the reviewer still decides, on the
   * source, the target and the checks.
   */
  context?: ReviewContext | null;
  /** The context fetch is in flight for this entry. */
  contextLoading?: boolean;
  onApprove: () => void;
  onReject: () => void;
  onEditToggle: () => void;
  onSaveEdit: (result: UnifiedSaveResult) => void | Promise<void>;
  onCancelEdit: () => void;
  onReCheck: () => void;
  /** Mark the selected source text as a term (governance-aware, in the parent). */
  onMarkTerm: (sourceText: string) => void;
  /** Suggest a brand/voice rule from the selected source text. */
  onSuggestVoiceRule: (sourceText: string) => void;
  /** Turn the reviewer's target fix into a voice correction/rule. */
  onMakeRule: () => void;
  /**
   * Propose a change to the SOURCE text (back-to-source review, RV-F). Unlike a
   * term mark (which re-checks the locales) an approved source change re-drafts
   * every locale — a wider blast radius the dialog makes legible. Receives the
   * selected span, or the whole block source when nothing is selected.
   */
  onProposeSourceChange?: (sourceText: string) => void;
  /** Promote a marked source entity to a terms store concept (lights the popover's Promote button). */
  onEntityPromote?: (entityKey: string) => void;
}

// Source and target are read against each other, so they are the same cell:
// same frame, same type, same renderer — only the label above them differs. A
// side that styles its text differently reads as a difference in the content.
const CELL = "rounded-lg border border-border bg-card p-3 text-sm leading-relaxed";

// The verdict over all three bars the server applies on approve, not over
// checks alone: "Passes checks" was a claim about one of them.
const verdictChip: Record<ReviewQueueVerdict, { label: string; className: string }> = {
  failing: {
    label: VERDICT_LABELS.failing,
    className: "border-destructive/40 bg-destructive/10 text-destructive",
  },
  passing: {
    label: VERDICT_LABELS.passing,
    className: "border-success/40 bg-success/10 text-success",
  },
};

/**
 * FocusedReviewer is the review session's right pane: one pending block shown
 * source-vs-target, both sides rendered by the same cell primitive the
 * translation editor uses (inline codes as chips, entity marks, formatting
 * applied), with its checks and compliance signal inline. Review is
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
  compliance,
  editing,
  busy,
  canApprove = true,
  reChecking,
  voiceProfileId,
  context = null,
  contextLoading,
  onApprove,
  onReject,
  onEditToggle,
  onSaveEdit,
  onCancelEdit,
  onReCheck,
  onMarkTerm,
  onSuggestVoiceRule,
  onMakeRule,
  onProposeSourceChange,
  onEntityPromote,
}: FocusedReviewerProps) {
  const { block, locale, issues } = entry;
  const status = getBlockStatus(block, locale);
  const verdict = entryVerdict(entry);
  const blockers = entryBlockers(entry);
  const chip = verdictChip[verdict];
  const [selection, setSelection] = useState("");

  const localeLabel = localeName ? `${localeName(locale)} (${locale})` : locale;

  // Capture the current source-text selection so the source-side lane can act
  // on it. Constrained to the source column via the onMouseUp target.
  //
  // The cell renders the block's literal text plus non-selectable chips for its
  // inline codes, so a DOM selection over it yields exactly the source text a
  // term or a voice rule is matched against: entity marks and formatting split
  // the text into more elements but `Selection.toString()` reads across them,
  // and a chip's label is not part of the source, so it stays out of it.
  const captureSelection = useCallback(() => {
    const text = window.getSelection?.()?.toString().trim() ?? "";
    setSelection(text);
  }, []);

  const entities: EntityInfo[] = block.entities ?? [];
  const errorCount = issues.filter((i) => i.severity === "error").length;

  const voiceFindings = context?.voice_findings ?? [];
  const targetText = getTargetText(block, locale);
  // The same declared projection the document surface reads through: the block
  // plus its evidence, layered as run-anchored overlays. The reviewer then sees
  // a positioned finding on the words it names rather than as a line of prose
  // detached from the text.
  const node = useMemo(
    () =>
      blockToContentNode(block, {
        evidence: {
          issues,
          issueLocale: locale,
          terms: context?.terms,
          findings: voiceFindings.map((finding) => ({ ...finding, side: locale })),
        },
        locales: [locale],
        sourceLocale,
      }),
    [block, issues, locale, sourceLocale, context?.terms, voiceFindings],
  );
  const memoryMatch = context?.memory_match;

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="focused-reviewer">
      {/* Header: identity, position, status, compliance */}
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
          data-testid={`reviewer-verdict-${verdict}`}
        >
          {verdict === "passing" ? (
            <CircleCheck className="h-3 w-3" />
          ) : (
            <AlertTriangle className="h-3 w-3" />
          )}
          {chip.label}
        </span>
        {/* Which bar, not just that one was missed — the reviewer's next act
            depends on whether it is a check, a term, or the voice score. */}
        {blockers.map((b) => (
          <span
            key={b}
            className="rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-muted-foreground"
            data-testid={`reviewer-blocker-${b}`}
          >
            {BLOCKER_LABELS[b]}
          </span>
        ))}
        {entry.voiceScore !== undefined && (
          <span
            className="rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] tabular-nums text-muted-foreground"
            data-testid="reviewer-voice-score"
            title="The block's latest voice score against its profile's compliance bar"
          >
            Voice {entry.voiceScore}/{entry.voiceBar}
          </span>
        )}
        {compliance && (
          <ComplianceRateChip
            rate={compliance.rate}
            basis={compliance.basis}
            compliantBlocks={compliance.compliantBlocks}
            translatedBlocks={compliance.translatedBlocks}
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

      {/* The unit on the left, the context it is decided in on the right. The
          rail is a column rather than a footer because a reviewer reads the
          governance WHILE reading the text, and a block that has to be scrolled
          past the decision bar is a block nobody reads. */}
      <div className="grid min-h-0 flex-1 gap-4 overflow-auto p-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="min-w-0">
          {/* The unit before this one. The document surface has the neighbourhood
            by construction; the queue is a flat list across items, so it says
            what this block sits between. */}
          <div className="mb-2">
            <NeighbourCell neighbour={context?.previous} where="previous" />
          </div>

          {/* Source vs target, generous side-by-side */}
          <div className="grid gap-4 lg:grid-cols-2">
            {/* Source */}
            <section className="space-y-2">
              <div className="flex h-6 items-center justify-between">
                <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Source ({sourceLocale})
                </span>
              </div>
              <div
                className={CELL}
                onMouseUp={captureSelection}
                onKeyUp={captureSelection}
                data-testid="reviewer-source"
                {...directionAttrs(sourceLocale)}
              >
                <FormattedSourceDisplay
                  codedText={block.source_coded ?? block.source ?? ""}
                  spans={block.source_spans ?? []}
                  entities={entities}
                  locale={sourceLocale}
                />
              </div>
              {/* Marked entities. The document marks them where they occur; this
                lane is where they can be acted on — a tooltip cannot hold a
                button. */}
              {entities.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5" data-testid="source-entities">
                  {entities.map((entity) => (
                    <span
                      key={entity.key}
                      className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-xs"
                    >
                      <span className="text-muted-foreground">{entityLabel(entity.type)}</span>
                      <span className="font-medium">{entity.text}</span>
                      {entity.dnt && (
                        <span className="rounded bg-muted px-1 text-[10px] uppercase tracking-wide text-muted-foreground">
                          dnt
                        </span>
                      )}
                      {onEntityPromote && (
                        <button
                          type="button"
                          className="text-primary hover:underline"
                          onClick={() => onEntityPromote(entity.key)}
                          data-testid={`promote-entity-${entity.key}`}
                        >
                          Promote
                        </button>
                      )}
                    </span>
                  ))}
                </div>
              )}
              {/* Source-side review lane. Mark-as-term and suggest-rule act on a
                selection (annotate → re-check the locales); propose-source-change
                is the back-to-source lane (transform → re-draft all locales). */}
              <div className="flex flex-wrap items-center gap-2" data-testid="source-lane">
                {selection && (
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
                      disabled={!voiceProfileId}
                      onClick={() => onSuggestVoiceRule(selection)}
                      data-testid="source-suggest-rule"
                    >
                      <Wand2 className="mr-1 h-3.5 w-3.5" /> Suggest brand rule
                    </Button>
                  </>
                )}
                {onProposeSourceChange && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-7 px-2 text-xs"
                    onClick={() => onProposeSourceChange(selection || block.source)}
                    data-testid="source-propose-change"
                    title="Propose a fix to the source text — a source change re-drafts every locale"
                  >
                    <FileText className="mr-1 h-3.5 w-3.5" /> Propose source change
                  </Button>
                )}
                {!selection && (
                  <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                    <Info className="h-3 w-3" /> Select source text to mark a term, or propose a
                    source change for the whole block.
                  </span>
                )}
              </div>
            </section>

            {/* Target */}
            <section className="space-y-2">
              <div className="flex h-6 items-center justify-between">
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
                <div className={CELL} data-testid="reviewer-target" {...directionAttrs(locale)}>
                  <CollapsedTargetCell
                    block={block}
                    locale={locale}
                    testId="reviewer-target-cell"
                  />
                </div>
              )}
              {voiceProfileId && !editing && (
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

          {/* The unit after this one. */}
          <div className="mt-2">
            <NeighbourCell neighbour={context?.next} where="next" />
          </div>

          {/* Findings, from both checkers, anchored where they name a span. The
            check findings and the voice findings judge the same target, so they
            are read as one list rather than as a score beside a list. */}
          <section className="mt-4 space-y-2" data-testid="reviewer-checks">
            <div className="flex items-center gap-2">
              <ContextHeading>Findings</ContextHeading>
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
            {targetText && (issues.length > 0 || voiceFindings.length > 0) && (
              <div className={CELL} {...directionAttrs(locale)}>
                <AnchoredTarget node={node} side={locale} text={targetText} />
              </div>
            )}
            <FindingsList issues={issues} findings={voiceFindings} />
          </section>
        </div>

        {/* What governs this point, what the corpus already said about it, and
            how the target came to say what it says. */}
        <div className="min-w-0 space-y-4" data-testid="reviewer-context-rail">
          <PointRail context={context} loading={contextLoading} />
          <div className="space-y-4">
            <section className="space-y-2" data-testid="reviewer-memory">
              <ContextHeading>Content memory</ContextHeading>
              <MemoryMatchCard
                match={memoryMatch}
                onUse={
                  memoryMatch && !editing
                    ? () =>
                        void onSaveEdit({ kind: "flat", codedText: memoryMatch.target, spans: [] })
                    : undefined
                }
              />
            </section>
            <section className="space-y-2" data-testid="reviewer-provenance">
              <ContextHeading>Provenance</ContextHeading>
              <ProvenanceBlock
                origin={context?.origin}
                decision={context?.decision}
                note={context?.notes?.[context.notes.length - 1]?.text}
              />
            </section>
          </div>
        </div>
      </div>

      {/* Decision bar + keyboard hints */}
      <div className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-2.5">
        <Button
          size="sm"
          onClick={onApprove}
          disabled={busy || !canApprove}
          title={canApprove ? undefined : "Approving needs the review permission for this language"}
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
