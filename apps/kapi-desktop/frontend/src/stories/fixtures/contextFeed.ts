// A feed of context operations for Storybook and tests, shaped the way the
// backend renders one: an agent session with a candidate awaiting a decision,
// a rule already in force, and a person's own day of work.

import type { ContextFeed, ContextFeedEntry, ContextFeedGroup } from "../../types/api";

const PROJECT = { project_key: "kapimart", project_name: "KapiMart" };

/** One operation, with the fields a feed entry always carries filled in. */
export function feedEntry(entry: Partial<ContextFeedEntry> & { id: string }): ContextFeedEntry {
  return {
    seq: Number(entry.id),
    kind: "observe",
    status: "suggested",
    actor: { kind: "agent", name: "claude", session: "sess-1", host: "studio" },
    subject: {},
    evidence: [],
    scope: { level: "project", describe: "project" },
    at: "2026-09-21T09:00:00Z",
    decidable: false,
    revertible: false,
    widen_to: [],
    recipe: "/fakehome/project/kapi.yaml",
    ...PROJECT,
    ...entry,
  };
}

/** One session, with the counts a summary line reads out. */
export function feedGroup(group: Partial<ContextFeedGroup> & { id: string }): ContextFeedGroup {
  const entries = group.entries ?? [];
  return {
    actor: { kind: "agent", name: "claude", session: "sess-1", host: "studio" },
    first: entries.at(-1)?.at ?? "2026-09-21T09:00:00Z",
    last: entries[0]?.at ?? "2026-09-21T09:00:00Z",
    awaiting: entries.filter((e) => e.decidable).length,
    recorded: 0,
    corrected: 0,
    kept: 0,
    dropped: 0,
    quiet: true,
    ...PROJECT,
    recipe: "/fakehome/project/kapi.yaml",
    ...group,
    entries,
  };
}

/** The candidate an agent proposed, with the evidence behind it. */
export const CANDIDATE = feedEntry({
  id: "12",
  kind: "observe",
  status: "suggested",
  decidable: true,
  subject: {
    kind: "term",
    term: "sign in",
    replacement: "log in",
    severity: "major",
    describe: 'term "sign in", use "log in"',
  },
  evidence: [
    {
      path: "docs/pricing.md",
      unit: "pricing-hero",
      quote: "Please sign in to see your prices.",
    },
  ],
  scope: {
    level: "project",
    coordinates: { brand: "kapimart", product: "store" },
    describe: "project brand=kapimart,product=store",
  },
});

/** A rule the person already accepted, which can be undone or widened. */
export const IN_FORCE = feedEntry({
  id: "9",
  kind: "observe",
  status: "established",
  revertible: true,
  widen_to: ["workspace", "brand", "product"],
  at: "2026-09-21T08:40:00Z",
  subject: {
    kind: "term",
    term: "utilise",
    replacement: "use",
    severity: "minor",
    describe: 'voice "utilise", use "use"',
  },
  evidence: [{ path: "docs/guide.md", quote: "Utilise the bulk import." }],
});

/** Something a person noticed, which states no rule. */
export const OBSERVATION = feedEntry({
  id: "4",
  kind: "observe",
  status: "suggested",
  actor: { kind: "person", name: "asgeir" },
  at: "2026-09-21T07:30:00Z",
  subject: { kind: "note", text: "prices are written with no space before the currency" },
  evidence: [{ path: "docs/pricing.md" }],
  recipe: "/fakehome/project/kapi.yaml",
});

/** A feed with one agent session and one person's day. */
export const CONTEXT_FEED: ContextFeed = {
  groups: [
    feedGroup({
      id: "session:sess-1",
      session: "sess-1",
      recorded: 2,
      kept: 1,
      quiet: true,
      entries: [CANDIDATE, IN_FORCE],
    }),
    feedGroup({
      id: "actor:person/asgeir:2026-09-21",
      session: undefined,
      day: "2026-09-21",
      actor: { kind: "person", name: "asgeir" },
      recorded: 1,
      quiet: true,
      entries: [OBSERVATION],
    }),
  ],
  awaiting: [{ project_key: "kapimart", project_name: "KapiMart", count: 1 }],
  awaiting_total: 1,
  awaiting_here: 1,
  truncated: false,
  read_only: false,
};

/** A workspace where nothing has been recorded. */
export const EMPTY_FEED: ContextFeed = {
  groups: [],
  awaiting: [],
  awaiting_total: 0,
  awaiting_here: 0,
  truncated: false,
  read_only: false,
};
