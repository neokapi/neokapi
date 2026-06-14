// Pure aggregation + formatting helpers for a change-set's blast radius
// (knowledge.ChangeSetImpact, AD-021). The server reports impact as a
// project → collection → (stream, locale) tree; these functions roll it up for
// the charts and headline numbers, and turn word counts into a legible effort
// proxy. No React, so they are unit-tested directly.
import type { ChangeSetImpact } from "../../types/brand-graph";

// ── Roll-ups ─────────────────────────────────────────────────────────────────

/** One row of impact, used for both project and locale break-downs. */
export interface ImpactBar {
  /** A stable key for React lists / chart x-axis. */
  key: string;
  /** Display label (project name or locale code). */
  label: string;
  affected_blocks: number;
  new_violations: number;
  resolved: number;
  words: number;
}

/** Per-project break-down, largest blast radius first. */
export function byProject(impact: ChangeSetImpact): ImpactBar[] {
  return (impact.projects ?? [])
    .map((p) => ({
      key: p.project_id,
      label: p.project_name || p.project_id,
      affected_blocks: p.affected_blocks,
      new_violations: p.new_violations,
      resolved: p.resolved,
      words: p.words,
    }))
    .sort((a, b) => b.affected_blocks - a.affected_blocks);
}

/**
 * Per-locale break-down, summed across every project → collection → stream.
 * A locale that appears in several projects is merged into one bar. Largest
 * affected-block count first.
 */
export function byLocale(impact: ChangeSetImpact): ImpactBar[] {
  const acc = new Map<string, ImpactBar>();
  for (const project of impact.projects ?? []) {
    for (const coll of project.collections ?? []) {
      for (const leaf of coll.locales ?? []) {
        const cur = acc.get(leaf.locale) ?? {
          key: leaf.locale,
          label: leaf.locale,
          affected_blocks: 0,
          new_violations: 0,
          resolved: 0,
          words: 0,
        };
        cur.affected_blocks += leaf.affected_blocks;
        cur.new_violations += leaf.new_violations;
        cur.resolved += leaf.resolved;
        cur.words += leaf.words;
        acc.set(leaf.locale, cur);
      }
    }
  }
  return [...acc.values()].sort((a, b) => b.affected_blocks - a.affected_blocks);
}

/** The net change in flagged blocks: positive = more flags, negative = a net clean-up. */
export function netViolationDelta(impact: ChangeSetImpact): number {
  return impact.new_violations - impact.resolved;
}

/** Share of stored blocks the change-set touches, 0–1 (0 when nothing is stored). */
export function affectedShare(impact: ChangeSetImpact): number {
  if (impact.total_blocks <= 0) return 0;
  return impact.affected_blocks / impact.total_blocks;
}

// ── Formatting ───────────────────────────────────────────────────────────────

/** Compact integer: 1234 → "1.2k", 980 → "980", 2_000_000 → "2.0M". */
export function formatCompact(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (abs >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(Math.round(n));
}

/** A percentage string from a 0–1 ratio, e.g. 0.027 → "2.7%", 0 → "0%". */
export function formatPercent(ratio: number): string {
  if (ratio <= 0) return "0%";
  const pct = ratio * 100;
  if (pct < 1) return `${pct.toFixed(1)}%`;
  return `${Math.round(pct)}%`;
}

// Effort proxy: assume careful localized review/re-translation runs at roughly
// 500 words per hour. This is a planning hint, not a quote.
const WORDS_PER_HOUR = 500;

/** Estimated review effort in hours for a word count (a planning proxy). */
export function effortHours(words: number): number {
  if (words <= 0) return 0;
  return words / WORDS_PER_HOUR;
}

/** A legible effort estimate: "—", "~15 min", "~2.5 h". */
export function formatEffort(words: number): string {
  if (words <= 0) return "—";
  const hours = effortHours(words);
  if (hours < 1) {
    const mins = Math.max(5, Math.round((hours * 60) / 5) * 5);
    return `~${mins} min`;
  }
  return `~${hours.toFixed(1)} h`;
}
