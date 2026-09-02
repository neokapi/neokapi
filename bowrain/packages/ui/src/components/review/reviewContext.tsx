import {
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  CoordinateChip,
  NeighbourhoodTable,
  When,
  checkIssueTone,
  cn,
  findingFails,
  findingToneBadgeClass,
  findingSeverityTone,
  findingToneTextClass,
  type FindingTone,
  type NeighbourhoodEntry,
} from "@neokapi/ui-primitives";
import { resolveOverlaySpans, segmentText } from "@neokapi/ui-primitives/preview";
import type { ContentNode } from "@neokapi/ui-primitives/preview";
import type {
  BlockTermMatch,
  CheckIssue,
  ReviewContext,
  ReviewDecision,
  ReviewNeighbour,
  ReviewOrigin,
  ReviewTermRule,
  MemoryMatchInfo,
} from "../../types/api";
import type { VoiceFinding } from "../../voice/types";
import { AlertTriangle, ChevronDown, CircleCheck, Clock, Info, Lock, Tag } from "../icons";

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

/**
 * One layer of context, headed by the single line that answers it.
 *
 * A reviewer reads five layers for every unit, and reading five panels to learn
 * that four of them say nothing costs more attention than the decision does. So
 * each layer states its answer on the heading row (the coordinates it sits at,
 * how many neighbours it has, what the corpus already said, what the checks
 * found, where the target came from) and keeps the evidence a disclosure away.
 * The layer itself is always drawn: a layer with nothing in it says so, which is
 * an answer, and hiding it would leave the reader to wonder.
 */
export function ContextLayer({
  title,
  summary,
  children,
  collapsible = true,
  defaultOpen = true,
  testId,
  className,
}: {
  /** The layer's name. */
  title: string;
  /** The one line a reader gets without opening anything. */
  summary: React.ReactNode;
  /** The evidence behind the summary. */
  children: React.ReactNode;
  /** False keeps the detail permanently below the summary. */
  collapsible?: boolean;
  /** Whether the detail starts open. */
  defaultOpen?: boolean;
  testId?: string;
  className?: string;
}) {
  const head = (
    <>
      <ContextHeading>{title}</ContextHeading>
      <span
        className="min-w-0 flex-1 truncate text-xs text-muted-foreground"
        data-testid={testId ? `${testId}-summary` : undefined}
      >
        {summary}
      </span>
    </>
  );

  if (!collapsible) {
    return (
      <section className={cn("space-y-2", className)} data-testid={testId}>
        <div className="flex items-center gap-2">{head}</div>
        {children}
      </section>
    );
  }

  return (
    <Collapsible
      defaultOpen={defaultOpen}
      className={cn("space-y-2", className)}
      data-testid={testId}
      asChild
    >
      <section>
        <CollapsibleTrigger className="group flex w-full items-center gap-2 text-left">
          {head}
          <ChevronDown
            aria-hidden
            className="h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform group-data-[state=open]:rotate-180"
          />
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-2">{children}</CollapsibleContent>
      </section>
    </Collapsible>
  );
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
  terms: termsOverride,
  termsLoading,
}: {
  context: ReviewContext | null;
  loading?: boolean;
  /**
   * The term hits to show, when the surface already holds them from its own
   * lookup. The document surface does: the same hits drive its inline marks,
   * so passing them here keeps one list rather than two copies of it.
   */
  terms?: BlockTermMatch[];
  /** That lookup is still in flight. */
  termsLoading?: boolean;
}) {
  const profile = context?.voice_profile;
  const terms = termsOverride ?? context?.terms ?? [];
  const coordinates = Object.entries(context?.coordinates ?? {});

  return (
    <ContextLayer
      title="In force here"
      summary={
        loading && !context ? (
          "Resolving what governs this block…"
        ) : (
          <PointSummary context={context} />
        )
      }
      testId="review-point"
    >
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
                <VoiceScoreChip
                  score={context?.voice_score}
                  bar={context?.voice_bar ?? profile.compliance_bar}
                />
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
                    <TermRuleChip key={`${rule.term}-${i}`} rule={rule} index={i} />
                  ))}
                </ul>
              )}
            </div>
          ) : (
            <ContextEmpty>No voice profile is bound at this point.</ContextEmpty>
          )}

          <div className="space-y-1 border-t border-border/60 pt-2">
            <ContextHeading>Terms</ContextHeading>
            {termsLoading ? (
              <ContextEmpty>Looking up terms…</ContextEmpty>
            ) : terms.length === 0 ? (
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
                <CoordinateChip
                  key={axis}
                  axis={axis}
                  value={value}
                  data-testid={`point-coordinate-${axis}`}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </ContextLayer>
  );
}

/**
 * The block's voice score against the bar it is held to, beside the profile
 * that set that bar.
 *
 * A score below the bar is a verdict on this unit, so it takes the finding tone
 * the rest of the surface gives a failing finding; a score that clears the bar
 * is context and stays muted. An unscored block has no verdict either way and
 * reads as the bar alone, which is what the profile asks of it.
 */
export function VoiceScoreChip({ score, bar }: { score?: number; bar: number }) {
  const below = score !== undefined && score < bar;
  return (
    <span
      className={cn(
        "rounded-full border px-2 py-0.5 text-[11px] tabular-nums",
        below
          ? findingToneBadgeClass("destructive")
          : "border-border bg-card text-muted-foreground",
      )}
      data-testid="point-voice-score"
      data-below-bar={below ? "true" : undefined}
      title={
        score === undefined
          ? "The lowest voice score this profile accepts. This block has not been scored."
          : "The block\u2019s latest voice score against the bar its profile sets"
      }
    >
      {score === undefined ? `Bar ${bar}` : `Voice ${score} of ${bar}`}
    </span>
  );
}

/**
 * The point in one line: the coordinates the unit sits at, drawn as chips, with
 * the profile that governs it named beside them.
 */
export function PointSummary({ context }: { context: ReviewContext | null }) {
  const coordinates = Object.entries(context?.coordinates ?? {});
  if (coordinates.length === 0) {
    return <>{context?.voice_profile?.name ?? "No coordinates declared"}</>;
  }
  return (
    <span className="flex flex-wrap items-center gap-1">
      {coordinates.map(([axis, value]) => (
        <CoordinateChip key={axis} axis={axis} value={value} />
      ))}
    </span>
  );
}

/**
 * How hard a rule bites, in the words a reviewer reads. `minor` and `neutral`
 * report a violation; every other severity, unset included, fails the check,
 * because a rule resolved from the terms store carries no severity of its own.
 */
function warnsOnly(rule: ReviewTermRule): boolean {
  const s = (rule.severity ?? "").toLowerCase();
  return s === "minor" || s === "neutral";
}

/**
 * One term rule bound at this point, drawn as context: the term, the wording
 * asked for instead, and a lock where the rule holds the term in the source
 * language.
 *
 * The rail says what the model was told about a word, so a rule is neutral
 * whatever its severity, and the bite reads in the tooltip. `packages/ui/docs/
 * judgement-colours.md` is the rule; Kapi Desktop's own PointRail draws the
 * same chip, down to the wording of the tooltip, because a reviewer moving
 * between the two surfaces reads one vocabulary.
 */
export function TermRuleChip({ rule, index }: { rule: ReviewTermRule; index?: number }) {
  const bite = warnsOnly(rule) ? "warns only" : "blocks approval";
  const label = rule.do_not_translate ? `do not translate · ${bite}` : bite;
  return (
    <li
      className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-xs text-foreground"
      title={rule.note ? `${label} · ${rule.note}` : label}
      data-severity={warnsOnly(rule) ? "warns" : "blocks"}
      data-dnt={rule.do_not_translate ? "true" : undefined}
      data-testid={index === undefined ? "point-term-rule" : `point-term-rule-${index}`}
    >
      {rule.do_not_translate && (
        <Lock className="h-3 w-3 shrink-0 text-muted-foreground" aria-hidden />
      )}
      <span className="font-medium">{rule.term}</span>
      {rule.replacement && <span className="text-muted-foreground">&rarr; {rule.replacement}</span>}
    </li>
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

/** One adjacent unit, as the shared table reads it. */
function neighbourRows(neighbour: ReviewNeighbour | undefined): NeighbourhoodEntry[] {
  return neighbour ? [{ key: neighbour.block_id, source: neighbour.source_runs }] : [];
}

/**
 * The unit in its item, with the units either side of it, drawn through the
 * kit's NeighbourhoodTable so a reviewer reads the same document here and in
 * Kapi Desktop. The neighbours travel as run sequences and go through the
 * declared run projection, so a placeholder in a neighbour reads as a chip
 * rather than disappearing.
 *
 * The server sends one neighbour each side and sends the source alone, so a
 * neighbour row here carries no target line. Its key and its source are what
 * the payload holds (`reviewNeighbour` in the review-context handler).
 */
export function NeighbourhoodView({
  context,
  unitKey,
  unitSource,
  unitTarget,
  sourceLocale,
  locale,
}: {
  context: ReviewContext | null;
  /** The unit under decision, drawn in place between its neighbours. */
  unitKey?: string;
  unitSource?: string;
  unitTarget?: string;
  sourceLocale?: string;
  locale?: string;
}) {
  return (
    <NeighbourhoodTable
      before={neighbourRows(context?.previous)}
      after={neighbourRows(context?.next)}
      unitKey={unitKey}
      unitSource={unitSource}
      unitTarget={unitTarget}
      sourceLocale={sourceLocale}
      targetLocale={locale}
    />
  );
}

/** How many units this one sits between, in one line. */
export function neighbourhoodSummary(context: ReviewContext | null): string {
  const before = context?.previous ? 1 : 0;
  const after = context?.next ? 1 : 0;
  const total = before + after;
  if (total === 0) return "The only unit in the item";
  if (total === 2) return "Between 2 units";
  return before === 1 ? "End of the item" : "Start of the item";
}

// ── History ─────────────────────────────────────────────────────────────────

/** What the corpus already said for this source, in one line. */
export function memorySummary(match: MemoryMatchInfo | undefined): string {
  if (!match) return "No match";
  return `${Math.round(match.score * 100)}% ${match.match_type.replace(/-/g, " ")}`;
}

/**
 * The wording the corpus already blessed for this source, with both sides
 * shown. `onUse` is offered only where the surface already has a write: the
 * queue's editor and the inspector's. Without one the card still reads: a
 * bulk pass applies these matches, and the reviewer sees them here first.
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
      {/* The score is how close the corpus came, which is context rather than a
          verdict, so it reads in the neutral chip every other bound fact uses
          and puts what the number means in the tooltip. */}
      <div className="flex items-center gap-2">
        <span
          className="rounded border border-border bg-card px-1.5 py-px text-[11px] font-medium tabular-nums text-foreground"
          title={`${Math.round(match.score * 100)}% match against the source of this block`}
          data-testid="memory-match-score"
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

/**
 * What the two checkers made of this unit, in one line: how many findings there
 * are and how many of them fail, counted over both lists, since a reviewer
 * reads them as one.
 */
export function findingsSummary(issues: CheckIssue[], findings: VoiceFinding[]): string {
  const total = issues.length + findings.length;
  if (total === 0) return "Nothing flagged";
  const failing =
    issues.filter((i) => checkIssueTone(i.severity) === "destructive").length +
    findings.filter((f) => findingFails(f.severity)).length;
  const counted = `${total} finding${total === 1 ? "" : "s"}`;
  return failing > 0 ? `${counted}, ${failing} failing` : counted;
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
  issues: CheckIssue[];
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
          tone={checkIssueTone(issue.severity)}
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
          tone={findingSeverityTone(finding.severity)}
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
  tone,
  originalText,
  suggestion,
}: {
  testId: string;
  category: string;
  message: string;
  /** How hard this one bites, from the shared severity scale. */
  tone: FindingTone;
  originalText?: string;
  suggestion?: string;
}) {
  const ink = findingToneTextClass(tone);
  return (
    <li className="flex items-start gap-1.5 text-xs" data-testid={testId} data-tone={tone}>
      <AlertTriangle className={cn("mt-0.5 h-3 w-3 shrink-0", ink)} />
      <div className="min-w-0 flex-1 space-y-0.5">
        <span className={ink}>
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

/** Where this target came from, and what was last decided, in one line. */
export function provenanceSummary(
  origin: ReviewOrigin | undefined,
  decision: ReviewDecision | undefined,
): string {
  const from = origin?.kind ? (ORIGIN_LABELS[origin.kind] ?? origin.kind) : "";
  const state = decision?.state ? (DECISION_LABELS[decision.state] ?? decision.state) : "";
  if (from && state) return `${from} · ${state}`;
  if (from) return from;
  if (state) return state;
  return "Nothing recorded";
}

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
              <When iso={origin.timestamp} />
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
            {decision.at && (
              <span className="text-muted-foreground">
                on <When iso={decision.at} />
              </span>
            )}
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
