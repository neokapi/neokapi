import { Button, cn } from "@neokapi/ui-primitives";
import { RunSequence, resolveOverlaySpans, segmentText } from "@neokapi/ui-primitives/preview";
import type { ContentNode } from "@neokapi/ui-primitives/preview";
import type {
  BlockTermMatch,
  QAIssue,
  ReviewContext,
  ReviewDecision,
  ReviewNeighbour,
  ReviewOrigin,
  MemoryMatchInfo,
} from "../../types/api";
import type { VoiceFinding } from "../../voice/types";
import { memoryScoreClass } from "../editor/blockStatus";
import { AlertTriangle, ArrowDown, ArrowUp, CircleCheck, Clock, Info, Tag } from "../icons";

/**
 * The five layers of context a reviewer decides in, as the pieces both review
 * surfaces compose. The queue lays them out around the source/target cells; the
 * document's inspector stacks the ones a reader wants beside the page.
 *
 * Everything here draws what the server resolved and says so when there is
 * nothing: an absent memory match, an undecided unit and an ungoverned point
 * each read as themselves rather than as a blank panel.
 */

/** The section heading both surfaces use above a context layer. */
export function ContextHeading({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
      {children}
    </span>
  );
}

/** A one-line "nothing here, and why" for an empty layer. */
export function ContextEmpty({ children }: { children: React.ReactNode }) {
  return <p className="text-xs text-muted-foreground">{children}</p>;
}

// ── Point ───────────────────────────────────────────────────────────────────

/**
 * Where this content sits and what governs it there: the voice profile by name
 * with the guidance the producer was given, the vocabulary rules in force, the
 * terms the source matches, and the collection and coordinates that place it.
 */
export function PointRail({
  context,
  loading,
}: {
  context: ReviewContext | null;
  loading?: boolean;
}) {
  const profile = context?.voice_profile;
  const terms = context?.terms ?? [];
  const coordinates = Object.entries(context?.coordinates ?? {});

  return (
    <section className="space-y-2" data-testid="review-point">
      <ContextHeading>In force here</ContextHeading>
      {loading && !context ? (
        <ContextEmpty>Resolving what governs this block…</ContextEmpty>
      ) : (
        <div className="space-y-2 rounded-md border border-border bg-muted/20 p-2.5">
          {profile ? (
            <div className="space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium" data-testid="point-profile-name">
                  {profile.name}
                </span>
                <span className="rounded-full border border-border bg-card px-2 py-0.5 text-[11px] tabular-nums text-muted-foreground">
                  Bar {profile.compliance_bar}
                </span>
              </div>
              {profile.guidance && (
                // Clamped, with the whole guidance in the title: the rail sits
                // beside the text a reviewer is reading, so a long profile must
                // not push the decision's own evidence off the pane.
                <p
                  className="line-clamp-4 text-xs leading-relaxed text-muted-foreground"
                  title={profile.guidance}
                  data-testid="point-guidance"
                >
                  {profile.guidance}
                </p>
              )}
              {profile.term_rules.length > 0 && (
                <ul className="flex flex-wrap gap-1.5 pt-0.5" data-testid="point-term-rules">
                  {profile.term_rules.map((rule, i) => (
                    <li
                      key={`${rule.term}-${i}`}
                      className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-xs"
                      title={rule.note}
                    >
                      <span className="font-medium line-through decoration-muted-foreground/60">
                        {rule.term}
                      </span>
                      {rule.replacement && (
                        <span className="text-muted-foreground">&rarr; {rule.replacement}</span>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ) : (
            <ContextEmpty>No voice profile is bound at this point.</ContextEmpty>
          )}

          <div className="space-y-1 border-t border-border/60 pt-2">
            <ContextHeading>Terms</ContextHeading>
            {terms.length === 0 ? (
              <ContextEmpty>No terms matched this block.</ContextEmpty>
            ) : (
              <ul className="flex flex-wrap gap-1.5" data-testid="point-terms">
                {terms.map((term, i) => (
                  <TermChip key={`${term.source_term}-${i}`} term={term} />
                ))}
              </ul>
            )}
          </div>

          {(context?.collection_name || coordinates.length > 0) && (
            <div className="flex flex-wrap items-center gap-1.5 border-t border-border/60 pt-2">
              {context?.collection_name && (
                <span
                  className="rounded-full border border-border bg-card px-2 py-0.5 text-[11px] text-muted-foreground"
                  data-testid="point-collection"
                >
                  {context.collection_name}
                </span>
              )}
              {coordinates.map(([axis, value]) => (
                <span
                  key={axis}
                  className="rounded-full border border-border bg-card px-2 py-0.5 text-[11px] text-muted-foreground"
                  data-testid={`point-coordinate-${axis}`}
                >
                  {axis}: {value}
                </span>
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

/** One term hit: the source term, and the renderings the terms store mandates. */
export function TermChip({ term }: { term: BlockTermMatch }) {
  return (
    <li
      className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-xs"
      title={term.domain ? `Domain: ${term.domain}` : undefined}
    >
      <Tag className="h-3 w-3 text-muted-foreground" />
      <span className="font-medium">{term.source_term}</span>
      {term.target_terms?.length > 0 && (
        <span className="text-muted-foreground">&rarr; {term.target_terms.join(", ")}</span>
      )}
    </li>
  );
}

// ── Neighbourhood ───────────────────────────────────────────────────────────

/**
 * One adjacent unit. The source travels as a run sequence and is drawn by the
 * kit's RunSequence, which answers for every run kind — a loop over `run.text`
 * here would quietly drop every placeholder and paired code the neighbour
 * carries.
 */
export function NeighbourCell({
  neighbour,
  where,
}: {
  neighbour: ReviewNeighbour | undefined;
  where: "previous" | "next";
}) {
  const Arrow = where === "previous" ? ArrowUp : ArrowDown;
  return (
    <div
      className="flex items-start gap-2 rounded-md border border-dashed border-border bg-muted/10 px-2.5 py-1.5 text-xs"
      data-testid={`neighbour-${where}`}
    >
      <Arrow className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
      {neighbour ? (
        <span className="min-w-0 flex-1 text-muted-foreground">
          <RunSequence runs={neighbour.source_runs} />
        </span>
      ) : (
        <span className="text-muted-foreground/70">
          {where === "previous" ? "Start of the item." : "End of the item."}
        </span>
      )}
    </div>
  );
}

// ── History ─────────────────────────────────────────────────────────────────

/**
 * The wording the corpus already blessed for this source, with both sides
 * shown. `onUse` is offered only where the surface already has a write: the
 * queue's editor and the inspector's. Without one the card still reads, which
 * is the point — a bulk pass used to apply these unseen.
 */
export function MemoryMatchCard({
  match,
  onUse,
  applied,
}: {
  match: MemoryMatchInfo | undefined;
  onUse?: () => void;
  applied?: boolean;
}) {
  if (!match) {
    return <ContextEmpty>No content-memory match for this block.</ContextEmpty>;
  }
  return (
    <div
      className="space-y-1 rounded-md border border-border bg-muted/20 p-2.5"
      data-testid="memory-match"
    >
      <div className="flex items-center gap-2">
        <span
          className={cn(
            "rounded px-1.5 py-px text-[11px] font-bold tabular-nums",
            memoryScoreClass(match.score),
          )}
        >
          {Math.round(match.score * 100)}%
        </span>
        <span className="text-[10px] text-muted-foreground">
          {match.match_type.replace(/-/g, " ")}
        </span>
      </div>
      <p className="text-xs text-muted-foreground" data-testid="memory-match-source">
        {match.source}
      </p>
      <p className="text-xs font-medium" data-testid="memory-match-target">
        {match.target}
      </p>
      {onUse && (
        <Button
          size="sm"
          variant="outline"
          className="mt-1 h-6 px-2 text-[11px]"
          onClick={onUse}
          disabled={applied}
          data-testid="memory-match-use"
        >
          {applied ? "Used" : "Use this wording"}
        </Button>
      )}
    </div>
  );
}

// ── Judgement ───────────────────────────────────────────────────────────────

/**
 * The target as the checks marked it. Spans come from the projected node's
 * overlays through the kit's own resolver, so a finding that carries a run
 * anchor lands on the words it names and one that carries none is listed below
 * rather than guessed onto a span.
 */
export function AnchoredTarget({
  node,
  side,
  text,
}: {
  node: ContentNode | null;
  side: string;
  text: string;
}) {
  const spans = node ? resolveOverlaySpans(node.overlays, side, text) : [];
  const segments = segmentText(text, spans);
  if (segments.length === 0) return null;
  return (
    <p className="text-sm leading-relaxed" data-testid="anchored-target">
      {segments.map((segment, i) =>
        segment.overlay ? (
          <mark
            key={i}
            className={cn("rounded-sm px-0.5", segment.overlay.style.className)}
            title={segment.overlay.tooltip}
            data-testid={`anchored-mark-${segment.overlay.type}`}
          >
            {segment.text}
          </mark>
        ) : (
          <span key={i}>{segment.text}</span>
        ),
      )}
    </p>
  );
}

/** Whether a finding's severity fails rather than warns. */
function findingFails(severity: string): boolean {
  return severity === "major" || severity === "critical";
}

/**
 * Every finding on the unit, from both checkers, with what each one names: the
 * text it was raised against, and the wording it asks for instead. A finding
 * carrying a run anchor says so, because the same finding is marked in the text
 * above.
 */
export function FindingsList({
  issues,
  findings,
}: {
  issues: QAIssue[];
  findings: VoiceFinding[];
}) {
  if (issues.length === 0 && findings.length === 0) {
    return (
      <div
        className="flex items-center gap-2 rounded-md border border-success/25 bg-success/5 px-3 py-2 text-sm text-success"
        data-testid="findings-none"
      >
        <CircleCheck className="h-4 w-4 shrink-0" />
        Nothing flagged on this block.
      </div>
    );
  }
  return (
    <ul
      className="space-y-1.5 rounded-md border border-border bg-muted/20 p-2"
      data-testid="findings"
    >
      {issues.map((issue, i) => (
        <FindingRow
          key={`issue-${i}`}
          testId={`finding-issue-${i}`}
          category={issue.type}
          message={issue.message}
          failing={issue.severity === "error"}
          originalText={issue.original_text}
          suggestion={issue.suggestion}
        />
      ))}
      {findings.map((finding, i) => (
        <FindingRow
          key={`voice-${i}`}
          testId={`finding-voice-${i}`}
          category={finding.category}
          message={finding.message}
          failing={findingFails(finding.severity)}
          originalText={finding.original_text}
          suggestion={finding.suggestion ?? finding.metadata?.replacement}
        />
      ))}
    </ul>
  );
}

function FindingRow({
  testId,
  category,
  message,
  failing,
  originalText,
  suggestion,
}: {
  testId: string;
  category: string;
  message: string;
  failing: boolean;
  originalText?: string;
  suggestion?: string;
}) {
  return (
    <li className="flex items-start gap-1.5 text-xs" data-testid={testId}>
      <AlertTriangle
        className={cn("mt-0.5 h-3 w-3 shrink-0", failing ? "text-destructive" : "text-warning")}
      />
      <div className="min-w-0 flex-1 space-y-0.5">
        <span className={failing ? "text-destructive" : "text-warning"}>
          <span className="font-medium">{category}:</span> {message}
        </span>
        {(originalText || suggestion) && (
          <div className="flex flex-wrap items-center gap-1.5">
            {originalText && (
              <span
                className="rounded bg-destructive/10 px-1.5 py-px font-mono text-[11px] text-destructive"
                data-testid={`${testId}-original`}
              >
                {originalText}
              </span>
            )}
            {suggestion && (
              <>
                <span className="text-muted-foreground">&rarr;</span>
                <span
                  className="rounded bg-success/10 px-1.5 py-px font-mono text-[11px] text-success"
                  data-testid={`${testId}-suggestion`}
                >
                  {suggestion}
                </span>
              </>
            )}
          </div>
        )}
      </div>
    </li>
  );
}

// ── Provenance ──────────────────────────────────────────────────────────────

/** How the origin kinds the model records read to a person. */
const ORIGIN_LABELS: Record<string, string> = {
  human: "Written by a person",
  memory: "Recycled from content memory",
  mt: "Machine translation",
  ai: "AI translation",
  ocr: "Read from an image",
  asr: "Transcribed from audio",
};

/** How a decision's review state reads to a person. */
const DECISION_LABELS: Record<string, string> = {
  approved: "Approved",
  rejected: "Rejected",
  "signed-off": "Signed off",
};

/**
 * How this target came to say what it says, and what was last decided about it.
 * Neither surface showed either: the ledger row the review handler writes was
 * read only by sync, and provenance never left the store.
 */
export function ProvenanceBlock({
  origin,
  decision,
  note,
}: {
  origin: ReviewOrigin | undefined;
  decision: ReviewDecision | undefined;
  note?: string;
}) {
  if (!origin && !decision && !note) {
    return <ContextEmpty>No decision recorded, and no provenance stamped.</ContextEmpty>;
  }
  const originLabel = origin?.kind ? (ORIGIN_LABELS[origin.kind] ?? origin.kind) : "";
  const engine = [origin?.engine, origin?.tool].filter(Boolean).join(" · ");
  return (
    <div
      className="space-y-1.5 rounded-md border border-border bg-muted/20 p-2.5 text-xs"
      data-testid="review-provenance"
    >
      {origin && (
        <div className="flex flex-wrap items-center gap-1.5" data-testid="provenance-origin">
          <span className="font-medium">{originLabel || "Origin"}</span>
          {engine && <span className="text-muted-foreground">{engine}</span>}
          {origin.timestamp && (
            <span className="inline-flex items-center gap-1 text-muted-foreground">
              <Clock className="h-3 w-3" />
              {origin.timestamp}
            </span>
          )}
        </div>
      )}
      {decision && (
        <div className="space-y-0.5" data-testid="provenance-decision">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="font-medium">
              {decision.state ? (DECISION_LABELS[decision.state] ?? decision.state) : "Last change"}
            </span>
            {decision.by && <span className="text-muted-foreground">by {decision.by}</span>}
            {decision.at && <span className="text-muted-foreground">on {decision.at}</span>}
          </div>
          {decision.source_moved && (
            <span className="inline-flex items-center gap-1 text-warning">
              <Info className="h-3 w-3" />
              The source has changed since that decision.
            </span>
          )}
        </div>
      )}
      {(decision?.note || note) && (
        <p className="text-muted-foreground" data-testid="provenance-note">
          &ldquo;{decision?.note || note}&rdquo;
        </p>
      )}
    </div>
  );
}
