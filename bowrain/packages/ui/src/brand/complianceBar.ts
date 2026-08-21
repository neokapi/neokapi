// The compliance bar and the score bands derived from it — the client mirror of
// core/profile's DefaultMinScore / VoiceProfile.ComplianceBar. Every surface
// that colours a 0–100 voice score reads its bar from here, so a profile with
// min_score 90 shows an 85 as below-bar in the gauge, the dimension bars, the
// dashboard and the rollup alike.

/** The bar applied when a profile sets no min_score (core/profile.DefaultMinScore). */
export const DEFAULT_MIN_SCORE = 80;

/** Carries a profile's own compliance bar; anything with a `min_score` field. */
export interface HasMinScore {
  min_score?: number;
}

/**
 * The effective compliance bar for a profile: its `min_score` when set (capped at
 * 100), DEFAULT_MIN_SCORE otherwise. A missing profile answers the default,
 * matching VoiceProfile.ComplianceBar on a nil receiver.
 */
export function complianceBar(profile?: HasMinScore | null): number {
  const min = profile?.min_score ?? 0;
  if (min <= 0) return DEFAULT_MIN_SCORE;
  return Math.min(min, 100);
}

/** A profile identified by id, carrying its own bar. */
export interface ProfileBarSource extends HasMinScore {
  id: string;
}

/**
 * The bar of the named profile within a loaded list, the default when the list
 * or the id is missing. Mirrors the server pairing each stored score with its
 * scoring profile's bar, so a deleted profile's scores fall back to the default
 * rather than losing their bar.
 */
export function barForProfile(
  profiles: readonly ProfileBarSource[] | undefined,
  profileId?: string,
): number {
  if (!profiles || !profileId) return DEFAULT_MIN_SCORE;
  return complianceBar(profiles.find((p) => p.id === profileId));
}

/** Where a score sits relative to the bar. */
export type ScoreBand = "pass" | "warn" | "fail";

/**
 * The band a score falls in: at or above the bar passes, at or above half the
 * bar warns, below that fails.
 */
export function scoreBand(score: number, bar: number = DEFAULT_MIN_SCORE): ScoreBand {
  if (score >= bar) return "pass";
  if (score >= bar / 2) return "warn";
  return "fail";
}

const TEXT_CLASS: Record<ScoreBand, string> = {
  pass: "text-success",
  warn: "text-warning",
  fail: "text-destructive",
};

const STROKE_CLASS: Record<ScoreBand, string> = {
  pass: "stroke-success",
  warn: "stroke-warning",
  fail: "stroke-destructive",
};

const FILL_CLASS: Record<ScoreBand, string> = {
  pass: "bg-success",
  warn: "bg-warning",
  fail: "bg-destructive",
};

/** Foreground colour for a score against a bar. */
export function scoreTextClass(score: number, bar?: number): string {
  return TEXT_CLASS[scoreBand(score, bar)];
}

/** SVG stroke colour for a score against a bar. */
export function scoreStrokeClass(score: number, bar?: number): string {
  return STROKE_CLASS[scoreBand(score, bar)];
}

/** Bar/track fill colour for a score against a bar. */
export function scoreFillClass(score: number, bar?: number): string {
  return FILL_CLASS[scoreBand(score, bar)];
}
