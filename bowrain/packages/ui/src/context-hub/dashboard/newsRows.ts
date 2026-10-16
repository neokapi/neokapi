// What the dashboard's What's new lists, as pure data so it is tested
// directly: one row per change-set a steward must decide or that moved
// recently, and one row per voice profile whose team corrections suggest
// rules. The rows that wait for a person lead: change-sets in review or
// approved, the longest-waiting first, then profiles with a disagreement, then
// profiles with suggestions. Change-sets that moved recently follow, newest
// first.
import type { ChangeSet } from "../../types/brand-graph";
import type { CandidateRule } from "../../voice/types";
import { candidateRows, dimensionKind, termKey } from "../../voice/candidateRows";
import { pendingDecisions, sortByRecent, waitingSince } from "./metrics";

/** How many change-sets that moved recently the list shows beside the ones that wait. */
export const RECENT_CHANGESETS = 5;

/** A change-set on What's new. */
export interface ChangeSetNews {
  type: "changeset";
  key: string;
  changeset: ChangeSet;
  /** The instant the row is dated by: when it started waiting, or when it last moved. */
  at: string;
  /** In review or approved: a steward decides it. */
  waiting: boolean;
}

/** A voice profile's suggested rules on What's new. */
export interface ProfileNews {
  type: "profile";
  key: string;
  profileId: string;
  name: string;
  /** Suggestions about words, the vocabulary dimension. */
  words: number;
  /** Suggestions about how the content is written: tone, style, clarity, compliance. */
  notes: number;
  /** Accepted by a reviewer, not yet promoted. */
  approved: number;
  /** Phrasings the team corrected to more than one form. */
  conflicts: number;
}

export type NewsRow = ChangeSetNews | ProfileNews;

/** A profile, with the candidates its review lists. */
export interface ProfileCandidates {
  id: string;
  name: string;
  candidates: readonly CandidateRule[];
}

/** One profile's suggested rules, counted the way its review groups them; null when none wait. */
export function profileNews(profile: ProfileCandidates): ProfileNews | null {
  const news: ProfileNews = {
    type: "profile",
    key: `profile:${profile.id}`,
    profileId: profile.id,
    name: profile.name,
    words: 0,
    notes: 0,
    approved: 0,
    conflicts: 0,
  };
  for (const row of candidateRows(profile.candidates)) {
    if (row.group === "conflict") news.conflicts += 1;
    else if (row.group === "approved") news.approved += 1;
    else if (row.group === "suggested") {
      if (dimensionKind(row.candidate.dimension) === "word") news.words += 1;
      else news.notes += 1;
    }
  }
  return news.words + news.notes + news.approved + news.conflicts > 0 ? news : null;
}

/** The rows of What's new, in the order the list shows them. */
export function whatsNewRows(
  changesets: readonly ChangeSet[],
  profiles: readonly ProfileCandidates[],
): NewsRow[] {
  const waiting = pendingDecisions([...changesets]);
  const waitingIds = new Set(waiting.map((cs) => cs.id));
  const recent = sortByRecent([...changesets])
    .filter((cs) => !waitingIds.has(cs.id))
    .slice(0, RECENT_CHANGESETS);

  const byProfile = profiles.map(profileNews).filter((n): n is ProfileNews => n !== null);
  const disputed = byProfile.filter((n) => n.conflicts > 0);
  const suggesting = byProfile.filter((n) => n.conflicts === 0);

  return [
    ...waiting.map((cs) => changesetRow(cs, true)),
    ...disputed,
    ...suggesting,
    ...recent.map((cs) => changesetRow(cs, false)),
  ];
}

function changesetRow(changeset: ChangeSet, waiting: boolean): ChangeSetNews {
  return {
    type: "changeset",
    key: `changeset:${changeset.id}`,
    changeset,
    at: waiting ? waitingSince(changeset) : changeset.updated_at,
    waiting,
  };
}

/** The workspace's suggestions and disagreements over every profile, each phrasing and form once. */
export function suggestionTotals(profiles: readonly ProfileCandidates[]): {
  suggestions: number;
  conflicts: number;
} {
  const forms = new Set<string>();
  const disputed = new Set<string>();
  for (const profile of profiles) {
    for (const row of candidateRows(profile.candidates)) {
      if (row.group === "conflict") {
        disputed.add(termKey(row.candidate.term));
        for (const side of row.sides ?? []) {
          forms.add(`${termKey(side.term)}\u0000${side.replacement.trim()}`);
        }
      } else if (row.group === "suggested" || row.group === "approved") {
        forms.add(`${termKey(row.candidate.term)}\u0000${row.candidate.replacement.trim()}`);
      }
    }
  }
  return { suggestions: forms.size, conflicts: disputed.size };
}
