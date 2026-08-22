import type { VoiceProfile } from "../brand/types";
import type {
  ContextScanArtefact,
  ContextScanAxis,
  ContextScanDraft,
  ContextScanTerm,
} from "../types/api";

/**
 * Reading a scan's proposals by kind, in one place.
 *
 * A scan returns a list of artefacts at points, so every surface that wants
 * "the voice" or "the terms" would otherwise write its own search — and each
 * one would answer differently the day a kind is added, silently. These are the
 * declared projections instead: a caller says which kind it wants, and a new
 * kind changes what they return only where that is intended.
 *
 * Mirrors the accessors on jobs.ContextScanResult.
 */

/** The voice profile a scan proposes, with its evidence. Null when it proposed none. */
export function scanVoice(draft: ContextScanDraft): {
  profile: VoiceProfile;
  evidence: ContextScanArtefact["evidence"];
} | null {
  const found = draft.artefacts.find((a) => a.kind === "voice" && a.voice);
  if (!found?.voice) return null;
  return { profile: found.voice, evidence: found.evidence };
}

/** Every candidate concept a scan proposes, across its terms artefacts. */
export function scanTerms(draft: ContextScanDraft): ContextScanTerm[] {
  return draft.artefacts.filter((a) => a.kind === "terms").flatMap((a) => a.terms ?? []);
}

/**
 * The points a scan proposed at, in first-seen order, as their axis maps.
 *
 * An empty map is the project's default point. Today a scan of one project's
 * own sources proposes only there, so this is a single empty entry — the review
 * surface uses it to decide whether a point is worth showing at all.
 */
export function scanPoints(draft: ContextScanDraft): Record<string, string>[] {
  const seen = new Set<string>();
  const out: Record<string, string>[] = [];
  for (const a of draft.artefacts) {
    const at = a.at ?? {};
    const key = Object.keys(at)
      .sort()
      .map((axis) => `${axis}=${at[axis]}`)
      .join(",");
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(at);
  }
  return out;
}

/** True when every proposal sits at the default point — the onboarding case. */
export function scanIsSinglePoint(draft: ContextScanDraft): boolean {
  const points = scanPoints(draft);
  return points.length <= 1 && Object.keys(points[0] ?? {}).length === 0;
}

/**
 * The axes a scan proposed, strongest evidence first.
 *
 * Empty is the ordinary answer — a corpus with no internal variation has no
 * axes — so a caller renders nothing rather than an empty state pretending
 * something went wrong.
 */
export function scanAxes(draft: ContextScanDraft): ContextScanAxis[] {
  return draft.axes ?? [];
}
