// A feed of context operations for Storybook and tests, shaped the way the
// backend renders one: an agent session with a candidate awaiting a decision,
// a rule already in force, and a person's own day of work.

import type {
  ContextCheckRun,
  ContextFeed,
  ContextFeedEntry,
  ContextFeedGroup,
} from "../../types/api";

const PROJECT = { project_key: "kapimart", project_name: "KapiMart" };

/** One operation, with the fields a feed entry always carries filled in. */
export function feedEntry(entry: Partial<ContextFeedEntry> & { id: string }): ContextFeedEntry {
  return {
    short: entry.id,
    kind: "observe",
    status: "suggested",
    actor: { kind: "agent", name: "claude", session: "sess-1", host: "studio" },
    subject: {},
    evidence: [],
    scope: { level: "project", describe: "project" },
    at: "2026-09-21T09:00:00Z",
    decidable: false,
    droppable: entry.decidable ?? false,
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
    checked: 0,
    failing: 0,
    caught: 0,
    files: 0,
    clean: false,
    failed: false,
    quiet: true,
    ...PROJECT,
    recipe: "/fakehome/project/kapi.yaml",
    runs: [],
    ...group,
    entries,
  };
}

/** One check run, a clean one unless the fields say otherwise. */
export function checkRun(
  run: Partial<ContextCheckRun> & { id: string; at: string },
): ContextCheckRun {
  return {
    project_key: PROJECT.project_key,
    hook: "stop",
    files: ["docs/pricing.md"],
    edited: true,
    file_count: 1,
    reached: true,
    clean: true,
    failing: 0,
    rules: [],
    findings: [],
    caught: 0,
    ...run,
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

/** A rule the person already accepted, which can be dropped or applied more widely. */
export const IN_FORCE = feedEntry({
  id: "9",
  kind: "observe",
  status: "established",
  droppable: true,
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
  truncated: false,
  read_only: false,
};

/** A workspace where nothing has been recorded. */
export const EMPTY_FEED: ContextFeed = {
  groups: [],
  truncated: false,
  read_only: false,
};

/** A person reset the project to before claude's session. */
export const RESET = feedEntry({
  id: "0nk3reset00000000000001",
  short: "0nk3reset0",
  kind: "reset",
  status: "",
  before: "sess-1",
  actor: { kind: "person", name: "asgeir" },
  at: "2026-09-22T10:15:00Z",
});

/** A feed whose latest session is a reset, which can itself be taken back. */
export const RESET_FEED: ContextFeed = {
  groups: [
    feedGroup({
      id: "actor:person/asgeir:2026-09-22",
      day: "2026-09-22",
      actor: { kind: "person", name: "asgeir" },
      entries: [RESET],
    }),
    ...CONTEXT_FEED.groups,
  ],
  truncated: false,
  read_only: false,
};

/** A second proposal in claude's session, so the keys have somewhere to go. */
export const SECOND_CANDIDATE = feedEntry({
  id: "13",
  kind: "observe",
  status: "suggested",
  decidable: true,
  at: "2026-09-21T08:55:00Z",
  subject: {
    kind: "term",
    term: "basket",
    replacement: "cart",
    describe: 'term "basket", use "cart"',
  },
  evidence: [{ path: "store/checkout.json", unit: "cart.add", quote: "Add to basket" }],
});

/** claude's session with two proposals waiting and a rule already in force. */
export const AWAITING_FEED: ContextFeed = {
  ...CONTEXT_FEED,
  groups: [
    feedGroup({
      id: "session:sess-1",
      session: "sess-1",
      recorded: 3,
      kept: 1,
      entries: [CANDIDATE, SECOND_CANDIDATE, IN_FORCE],
    }),
    ...CONTEXT_FEED.groups.slice(1),
  ],
};

const MANY_ACTORS = [
  { kind: "agent", name: "claude-code", host: "studio" },
  { kind: "agent", name: "codex", host: "laptop" },
  { kind: "person", name: "asgeir" },
] as const;

/** Forty sessions, newest first, by two assistants and a person, with older ones cut off. */
export const MANY_SESSIONS_FEED: ContextFeed = {
  groups: Array.from({ length: 40 }, (_, i) => {
    const actor = MANY_ACTORS[i % MANY_ACTORS.length];
    const at = new Date(Date.UTC(2026, 8, 21, 9) - i * 5 * 3600_000).toISOString();
    const person = actor.kind === "person";
    const session = person ? undefined : `sess-m${i}`;
    const entries = Array.from({ length: 1 + (i % 4) }, (_, j) =>
      feedEntry({
        id: `m${i}-${j}`,
        at,
        actor: { ...actor, session },
        status: j === 0 && i < 3 ? "suggested" : "established",
        decidable: j === 0 && i < 3,
        droppable: true,
        subject: {
          kind: "term",
          term: `term ${i}.${j}`,
          replacement: `preferred ${i}.${j}`,
          describe: `term "term ${i}.${j}", use "preferred ${i}.${j}"`,
        },
      }),
    );
    return feedGroup({
      id: person ? `actor:person/asgeir:day-${i}` : `session:${session}`,
      session,
      day: person ? at.slice(0, 10) : undefined,
      actor: { ...actor, session },
      recorded: entries.length,
      entries,
    });
  }),
  truncated: true,
  read_only: false,
};

/** The wording the check reported in the session's first turns. */
const SIGN_IN = { file: "docs/pricing.md", rule: "terms.vocabulary", quote: "sign in" };
const UTILISE = { file: "docs/onboarding/welcome.md", rule: "terms.vocabulary", quote: "utilise" };

/**
 * A session that only ran the check on every edit: the first run found two
 * findings, the next no longer found one of them, and the last was clean.
 */
export const HOOK_ONLY_SESSION = feedGroup({
  id: "session:sess-hook",
  session: "sess-hook",
  actor: { kind: "agent", name: "claude-code", session: "sess-hook", host: "studio" },
  first: "2026-09-21T08:10:00Z",
  last: "2026-09-21T08:30:00Z",
  files: 2,
  checked: 3,
  caught: 2,
  failed: true,
  entries: [],
  runs: [
    checkRun({ id: "run-3", at: "2026-09-21T08:30:00Z" }),
    checkRun({
      id: "run-2",
      at: "2026-09-21T08:20:00Z",
      hook: "edit",
      clean: false,
      failing: 1,
      rules: ["terms.vocabulary"],
      findings: [{ ...SIGN_IN, caught: true }],
      caught: 1,
    }),
    checkRun({
      id: "run-1",
      at: "2026-09-21T08:10:00Z",
      files: ["docs/pricing.md", "docs/onboarding/welcome.md"],
      file_count: 2,
      clean: false,
      failing: 2,
      rules: ["terms.vocabulary"],
      findings: [
        { ...SIGN_IN, caught: false },
        { ...UTILISE, caught: true },
      ],
      caught: 1,
    }),
  ],
});

/**
 * A session that recorded suggestions and ran the check between them: a
 * failing run before the rule in force, and a clean one after it.
 */
export const MIXED_SESSION = feedGroup({
  id: "session:sess-1",
  session: "sess-1",
  recorded: 2,
  kept: 1,
  files: 1,
  checked: 2,
  caught: 1,
  failed: true,
  entries: [CANDIDATE, IN_FORCE],
  first: "2026-09-21T08:30:00Z",
  runs: [
    checkRun({ id: "run-b", at: "2026-09-21T08:50:00Z" }),
    checkRun({
      id: "run-a",
      at: "2026-09-21T08:30:00Z",
      clean: false,
      failing: 1,
      rules: ["terms.vocabulary"],
      findings: [{ ...SIGN_IN, caught: true }],
      caught: 1,
    }),
  ],
});
