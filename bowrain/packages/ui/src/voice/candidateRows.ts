// The rows of a voice profile's suggested-rule review, in the order the list
// shows them, and where the selection goes as a reviewer works down the list.
//
// The server lists one candidate per phrasing the team corrected and what
// they corrected it to, and records one decision per phrasing. So a phrasing
// corrected to two different forms is a disagreement: promoting it means
// choosing the form, and rejecting it sets every form aside. The list has five
// groups: Needs you (each disagreement), Suggested (a phrasing with one form
// and no decision), Approved (accepted by a reviewer, not yet promoted),
// Established (promoted into the terms store) and Rejected.
import type { DigestKind } from "@neokapi/ui-primitives";
import type { CandidateRule } from "./types";

/** The list's groups, in the order they are shown. */
export const CANDIDATE_ROW_GROUPS = [
  "conflict",
  "suggested",
  "approved",
  "promoted",
  "rejected",
] as const;

export type CandidateRowGroup = (typeof CANDIDATE_ROW_GROUPS)[number];

/** The groups whose rows wait for a decision. */
const OPEN_GROUPS: ReadonlySet<CandidateRowGroup> = new Set(["conflict", "suggested", "approved"]);

/** One row of the list. */
export interface CandidateRow {
  /** Unique within the list: the group, the phrasing and, outside a disagreement, its form. */
  key: string;
  group: CandidateRowGroup;
  /** The candidate the row stands for. A disagreement's row stands for its most corrected form. */
  candidate: CandidateRule;
  /** Every form of a disagreement, most corrected first. */
  sides?: CandidateRule[];
}

/** How the server matches a decision to a phrasing: case-insensitive, trimmed. */
export function termKey(term: string): string {
  return term.trim().toLowerCase();
}

/** Most corrections first, then by form, so the order is stable. */
function byCorrections(a: CandidateRule, b: CandidateRule): number {
  return b.correction_count - a.correction_count || a.replacement.localeCompare(b.replacement);
}

/** Most recently decided first. A decision without a time sorts last. */
function byDecided(a: CandidateRule, b: CandidateRule): number {
  return (b.decided_at ?? "").localeCompare(a.decided_at ?? "");
}

function groupOf(status: CandidateRule["status"]): CandidateRowGroup {
  switch (status) {
    case "approved":
      return "approved";
    case "promoted":
      return "promoted";
    case "rejected":
      return "rejected";
    default:
      return "suggested";
  }
}

/**
 * One candidate per phrasing and form. The server counts corrections per
 * dimension as well, so the same correction recorded under two dimensions
 * arrives twice; promoting and rejecting name only the phrasing and the form,
 * so the two are one rule here, with both counts behind it.
 */
function collapseForms(candidates: readonly CandidateRule[]): CandidateRule[] {
  const byForm = new Map<string, CandidateRule>();
  for (const candidate of candidates) {
    const key = `${termKey(candidate.term)}\u0000${candidate.replacement.trim()}`;
    const seen = byForm.get(key);
    if (!seen) {
      byForm.set(key, { ...candidate });
      continue;
    }
    // The dimension with more corrections names the rule.
    if (candidate.correction_count > seen.correction_count) seen.dimension = candidate.dimension;
    seen.correction_count += candidate.correction_count;
  }
  return [...byForm.values()];
}

/** Every row of a candidate list, grouped and in the order the list shows them. */
export function candidateRows(candidates: readonly CandidateRule[]): CandidateRow[] {
  const byTerm = new Map<string, CandidateRule[]>();
  for (const candidate of collapseForms(candidates)) {
    const key = termKey(candidate.term);
    const forms = byTerm.get(key);
    if (forms) forms.push(candidate);
    else byTerm.set(key, [candidate]);
  }

  const byGroup: Record<CandidateRowGroup, CandidateRow[]> = {
    conflict: [],
    suggested: [],
    approved: [],
    promoted: [],
    rejected: [],
  };
  for (const [key, forms] of byTerm) {
    const sorted = [...forms].sort(byCorrections);
    const group = groupOf(sorted[0].status);
    // The forms are distinct already, so two of them is a disagreement.
    if (OPEN_GROUPS.has(group) && sorted.length > 1) {
      byGroup.conflict.push({
        key: `conflict:${key}`,
        group: "conflict",
        candidate: sorted[0],
        sides: sorted,
      });
      continue;
    }
    // A decided phrasing keeps a row per form the server lists, since the
    // decision does not say which of them it took.
    for (const candidate of sorted) {
      const g = groupOf(candidate.status);
      byGroup[g].push({ key: `${g}:${key}→${candidate.replacement}`, group: g, candidate });
    }
  }

  const total = (row: CandidateRow) =>
    (row.sides ?? [row.candidate]).reduce((n, c) => n + c.correction_count, 0);
  byGroup.conflict.sort((a, b) => total(b) - total(a));
  byGroup.suggested.sort((a, b) => byCorrections(a.candidate, b.candidate));
  byGroup.approved.sort((a, b) => byCorrections(a.candidate, b.candidate));
  byGroup.promoted.sort((a, b) => byDecided(a.candidate, b.candidate));
  byGroup.rejected.sort((a, b) => byDecided(a.candidate, b.candidate));

  return CANDIDATE_ROW_GROUPS.flatMap((group) => byGroup[group]);
}

/** Whether a row waits for a decision. */
export function isOpen(row: CandidateRow): boolean {
  return OPEN_GROUPS.has(row.group);
}

/** The row the list opens on: the first that waits for a decision and was not just decided. */
export function defaultRowKey(
  rows: readonly CandidateRow[],
  decided: ReadonlySet<string>,
): string | undefined {
  return rows.find((r) => isOpen(r) && !decided.has(r.key))?.key;
}

/**
 * Where the selection goes after a decision on `from`: the next row that still
 * waits for one, then the first such row above it, else nothing.
 */
export function nextRowKey(
  rows: readonly CandidateRow[],
  from: string,
  decided: ReadonlySet<string>,
): string | null {
  const at = rows.findIndex((r) => r.key === from);
  const waiting = (r: CandidateRow) => isOpen(r) && !decided.has(r.key) && r.key !== from;
  const after = rows.slice(at + 1).find(waiting);
  if (after) return after.key;
  return rows.slice(0, Math.max(at, 0)).find(waiting)?.key ?? null;
}

/**
 * The kind a row is drawn as. A disagreement and an established rule say where
 * the row stands; the rest say what the correction was about: a word for the
 * vocabulary dimension, how the content is written for the others.
 */
export function candidateKind(row: CandidateRow): DigestKind {
  if (row.group === "conflict") return "conflict";
  if (row.group === "promoted") return "established";
  return dimensionKind(row.candidate.dimension);
}

/** The digest kind a voice dimension's corrections are drawn as. */
export function dimensionKind(dimension: string): DigestKind {
  return dimension === "vocabulary" ? "word" : "note";
}

/** How many corrections stand behind a row: every form's, for a disagreement. */
export function rowCorrections(row: CandidateRow): number {
  return (row.sides ?? [row.candidate]).reduce((n, c) => n + c.correction_count, 0);
}
