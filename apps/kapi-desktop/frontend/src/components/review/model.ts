// host.ReviewContext, as the shared review cards read it.
//
// The Wails types mirror the Go struct field for field, so each layer maps
// onto its card's view in a few lines. The cards themselves live in
// `@neokapi/ui-primitives` and draw the same thing on the platform.

import {
  findingSeverityTone,
  type ReviewFindingView,
  type ReviewHistoryView,
  type ReviewNeighbourhoodView,
  type ReviewPointView,
  type ReviewProvenanceView,
} from "@neokapi/ui-primitives";
import type {
  DesktopFinding,
  ReviewHistory,
  ReviewNeighbourhood,
  ReviewPoint,
  ReviewProvenance,
  TargetOrigin,
} from "../../types/api";

export function toPointView(point: ReviewPoint): ReviewPointView {
  return {
    ref: point.ref,
    default: point.default,
    path: point.path,
    collection: point.collection,
    coordinates: point.coordinates,
    voice: point.voice
      ? { name: point.voice.name, source: point.voice.source, guide: point.voice.guide }
      : undefined,
    termRules: point.term_rules,
    termsTotal: point.terms_total,
    profiles: point.profiles,
    notes: point.notes,
  };
}

export function toNeighbourhoodView(n: ReviewNeighbourhood): ReviewNeighbourhoodView {
  return { key: n.key, before: n.before ?? [], after: n.after ?? [] };
}

export function toHistoryView(h: ReviewHistory): ReviewHistoryView {
  return {
    prior: h.prior
      ? { source: h.prior.source, target: h.prior.target, governed: h.prior.governed }
      : undefined,
    match: h.match
      ? { score: h.match.score, source: h.match.source, target: h.match.target }
      : undefined,
    unseeded: h.unseeded,
  };
}

/** The unit's check findings, painted on the shared severity scale. */
export function toFindingViews(findings: DesktopFinding[]): ReviewFindingView[] {
  return findings.map((f, i) => ({
    id: `finding-${i}`,
    category: f.category,
    severity: f.severity,
    tone: findingSeverityTone(f.severity),
    message: f.message,
    suggestion: f.suggestion ?? f.replacement,
    originalText: f.original_text,
    field: f.field,
  }));
}

/**
 * The provenance layer, with the flat fields the queue row carries as the
 * fallback until the model arrives.
 */
export function toProvenanceView(
  provenance: ReviewProvenance | undefined,
  fallback: { origin?: TargetOrigin; reviewState?: string; note?: string } = {},
): ReviewProvenanceView {
  const origin = provenance?.origin ?? fallback.origin;
  const state = provenance?.review_state ?? fallback.reviewState;
  const by = provenance?.by;
  const at = provenance?.at;
  return {
    origin: origin
      ? { kind: origin.kind, engine: origin.engine, tool: origin.tool, timestamp: origin.timestamp }
      : undefined,
    decision:
      state || by || at
        ? {
            state,
            by,
            at,
            note: provenance?.note,
            sourceMoved: provenance?.stale,
          }
        : undefined,
    note: provenance?.note ?? fallback.note,
  };
}
