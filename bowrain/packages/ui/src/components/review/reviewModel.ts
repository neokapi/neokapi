// The platform's review context, as the shared review cards read it.
//
// The server assembles the five layers in its own shape (`reviewContextResponse`
// in `bowrain/server/handlers_review_context.go`): the voice with its score and
// bar, positioned term hits, one neighbour each side, the memory match on a
// fraction scale, the ledger's decision and the target's origin. Each layer
// maps onto its card's view here, and the cards in `@neokapi/ui-primitives`
// draw the same thing kapi desktop draws over `host.ReviewContext`.

import {
  checkIssueTone,
  findingSeverityTone,
  type NeighbourhoodEntry,
  type ReviewFindingView,
  type ReviewHistoryView,
  type ReviewNeighbourhoodView,
  type ReviewPointView,
  type ReviewProvenanceView,
} from "@neokapi/ui-primitives";
import type { BlockTermMatch, CheckIssue, ReviewContext, ReviewNeighbour } from "../../types/api";
import type { VoiceFinding } from "../../voice/types";

/**
 * Where the unit sits and what governs it. The term hits are the context's
 * unless the surface already holds them from its own lookup, which the
 * document surface does: the same hits drive its inline marks, so passing
 * them keeps one list rather than two copies of it.
 */
export function toPointView(
  context: ReviewContext,
  terms?: BlockTermMatch[],
  termsLoading?: boolean,
): ReviewPointView {
  const profile = context.voice_profile;
  const hits = terms ?? context.terms ?? [];
  return {
    collection: context.collection_name,
    coordinates: context.coordinates,
    voice: profile
      ? {
          name: profile.name,
          guide: profile.guidance,
          score: context.voice_score,
          bar: context.voice_bar ?? profile.compliance_bar,
        }
      : undefined,
    termRules: profile?.term_rules ?? [],
    termHits: hits.map((hit) => ({
      term: hit.source_term,
      renderings: hit.target_terms,
      domain: hit.domain,
    })),
    termHitsLoading: termsLoading,
  };
}

/** One adjacent unit, as the shared table reads it. */
function neighbourRows(neighbour: ReviewNeighbour | undefined): NeighbourhoodEntry[] {
  if (!neighbour) return [];
  return [
    { key: neighbour.block_id, source: neighbour.source_runs, target: neighbour.target_runs },
  ];
}

/** The units either side of this one in the item. */
export function toNeighbourhoodView(context: ReviewContext): ReviewNeighbourhoodView {
  return {
    key: context.block_id,
    before: neighbourRows(context.previous),
    after: neighbourRows(context.next),
  };
}

/** The content memory's best match, on the percent scale the card reads. */
export function toHistoryView(context: ReviewContext): ReviewHistoryView {
  const match = context.memory_match;
  return {
    match: match
      ? {
          score: Math.round(match.score * 100),
          source: match.source,
          target: match.target,
          kind: match.match_type,
        }
      : undefined,
  };
}

/**
 * Every finding on the unit, from both checkers, as one list: the check
 * issues on the server's error/warning scale, then the findings behind the
 * voice score on core/check's. The ids are the row hooks the surfaces' tests
 * and the anchored marks refer to.
 */
export function toFindingViews(
  issues: CheckIssue[],
  findings: VoiceFinding[],
): ReviewFindingView[] {
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
    ...findings.map<ReviewFindingView>((finding, i) => ({
      id: `finding-voice-${i}`,
      category: finding.category,
      severity: finding.severity,
      tone: findingSeverityTone(finding.severity),
      message: finding.message,
      suggestion: finding.suggestion ?? finding.metadata?.replacement,
      originalText: finding.original_text,
    })),
  ];
}

/** How the target came to say what it says, and what was last decided about it. */
export function toProvenanceView(context: ReviewContext): ReviewProvenanceView {
  const origin = context.origin;
  const decision = context.decision;
  return {
    origin: origin
      ? { kind: origin.kind, engine: origin.engine, tool: origin.tool, timestamp: origin.timestamp }
      : undefined,
    decision: decision
      ? {
          state: decision.state,
          by: decision.by,
          at: decision.at,
          note: decision.note,
          sourceMoved: decision.source_moved,
        }
      : undefined,
    note: context.notes?.[context.notes.length - 1]?.text,
  };
}
