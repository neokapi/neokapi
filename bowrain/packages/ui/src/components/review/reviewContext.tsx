import type { CheckFinding } from "@neokapi/contract-types";
import {
  checkFindingViews,
  checkIssueTone,
  cn,
  type ReviewFindingView,
  type ReviewTermHitView,
} from "@neokapi/ui-primitives";
import { resolveOverlaySpans, segmentText } from "@neokapi/ui-primitives/preview";
import type { ContentNode } from "@neokapi/ui-primitives/preview";
import type { BlockNote, BlockTermMatch, CheckIssue } from "../../types/api";

/**
 * What the platform keeps for itself around the shared review cards. The five
 * layers of the review model are the cards in `@neokapi/ui-primitives`, and
 * they take the model as the server serialises it; what remains here is the
 * platform's own rows beside it (its check issues, its positioned term hits,
 * its block notes) projected for the cards, and the target with the findings
 * marked on it in place, which the desktop reads by opening the document.
 */

/**
 * Every finding on the unit, from both checkers, as one list: the check issues
 * on the server's error/warning scale, then the findings behind the voice
 * score on core/check's. The ids are the row hooks the surfaces' tests and the
 * anchored marks refer to.
 */
export function findingViews(issues: CheckIssue[], findings: CheckFinding[]): ReviewFindingView[] {
  return [
    ...issues.map<ReviewFindingView>((issue, i) => ({
      id: `finding-issue-${i}`,
      category: issue.type,
      severity: issue.severity,
      tone: checkIssueTone(issue.severity),
      message: issue.message,
      suggestion: issue.suggestion,
      originalText: issue.original_text,
    })),
    ...checkFindingViews(
      findings.map((f) => ({ ...f, replacement: f.metadata?.replacement })),
      "finding-voice",
    ),
  ];
}

/** The terms the source matches, as the point card reads them. */
export function termHitViews(hits: BlockTermMatch[]): ReviewTermHitView[] {
  return hits.map((hit) => ({
    term: hit.source_term,
    renderings: hit.target_terms,
    domain: hit.domain,
  }));
}

/** The latest note on the unit, drawn beside the decision in force. */
export function latestNote(notes: BlockNote[]): string | undefined {
  return notes[notes.length - 1]?.text;
}

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
