// Fixture sources for the stories, reused by the tests (Apache-2.0).
//
// Two shapes, because the point of the package is that one component set serves
// both: a PINNED source standing in for kapi's local project, and a FREE source
// standing in for a Bowrain workspace. The fixtures differ in reach and in which
// optional methods they implement — not in the shape of any answer.

import type { ContextDataSource } from "../source";
import type { Dimension, ScopeTuple } from "../scope";
import type { ContentAnswer, GovernanceAnswer, RelationsAnswer, SearchAnswer } from "../types";

export const pinnedScope: ScopeTuple = {
  workspace: "northsea",
  project: "site",
  stream: "main",
  collection: "docs",
  path: "help/billing.md",
};

export const freeScope: ScopeTuple = { workspace: "northsea" };

const governanceAtPoint: GovernanceAnswer = {
  point: {
    path: "help/billing.md",
    profile: "support",
    channel: "northsea/docs",
    collection: "docs",
    ref: "profiles.support.channel",
    default: false,
  },
  scope: "project",
  voice: {
    name: "Support",
    source: ".kapi/profiles/support/voice.yaml",
    field: "profiles.support.voice",
  },
  terms: [
    {
      concept_id: "c-signin",
      term: "sign in",
      locale: "en",
      status: "preferred",
      definition: "The act of authenticating into the product.",
      discouraged: false,
      uses: 42,
      use_blocks: 31,
    },
    {
      concept_id: "c-signin",
      term: "log in",
      locale: "en",
      status: "discouraged",
      replacement: "sign in",
      discouraged: true,
      valid_to: "2026-12-31",
    },
  ],
  terms_total: 18,
  profiles: [
    { name: "support", state: "active", valid_from: "2026-01-01" },
    { name: "campaign-spring", state: "expired", valid_to: "2026-06-30" },
  ],
  rules: [
    {
      id: "r-sentence-case",
      label: "Headings are sentence case",
      gate: "voice",
      severity: "error",
    },
  ],
  notes: [],
};

const contentAtPoint: ContentAnswer = {
  point: governanceAtPoint.point,
  scope: "project",
  collections: [],
  items: [
    {
      id: "b-1",
      document: "help/billing.md",
      collection: "docs",
      locale: "en",
      text: "Sign in to see your invoices.",
      ship_state: "governed",
    },
    {
      id: "b-2",
      document: "help/billing.md",
      collection: "docs",
      locale: "nb",
      text: "Logg inn for å se fakturaene dine.",
      ship_state: "pending",
      stale: true,
    },
  ],
  items_total: 24,
  coverage: [
    { locale: "en", units: 24, covered: 24, ship_state: "governed" },
    { locale: "nb", units: 24, covered: 20, ship_state: "pending", stale: 3 },
  ],
  notes: [],
};

const relationsInProject: RelationsAnswer = {
  subject: { kind: "concept", id: "c-signin", label: "sign in" },
  reach: "project",
  occurrences: [
    {
      concept_id: "c-signin",
      term: "sign in",
      locale: "en",
      collection: "docs",
      document: "help/billing.md",
      block_id: "b-1",
      matched: "Sign in",
      snippet: "Sign in to see your invoices.",
    },
  ],
  occurrences_total: 42,
  projects: [],
  blessings: [{ unit: "help/billing.md#b-1", variant: "nb", status: "approved", stale: true }],
  notes: [],
};

const relationsAcrossWorkspace: RelationsAnswer = {
  ...relationsInProject,
  reach: "workspace",
  occurrences: [
    ...relationsInProject.occurrences,
    {
      concept_id: "c-signin",
      term: "sign in",
      locale: "en",
      project: "app",
      collection: "strings",
      document: "en.json",
      matched: "Sign in",
      snippet: "Sign in",
    },
  ],
  projects: [
    // One project says the preferred word, the other still says the
    // discouraged one — which is why the row carries a standing rather than the
    // workspace carrying one flag for the concept.
    {
      project: "site",
      project_name: "Northsea site",
      stream: "main",
      occurrences: 42,
      collections: ["docs"],
      status: "deprecated",
      discouraged: true,
      replacement: "sign in",
      terms: [
        { term: "log in", locale: "en", status: "deprecated", discouraged: true, occurrences: 42 },
      ],
    },
    {
      project: "app",
      project_name: "Northsea app",
      stream: "main",
      occurrences: 7,
      collections: ["strings"],
      status: "preferred",
      discouraged: false,
      terms: [{ term: "sign in", locale: "en", status: "preferred", occurrences: 7 }],
    },
  ],
};

const searchAnswer: SearchAnswer = {
  query: "sign in",
  scope: "project",
  groups: [
    { kind: "terms", hits: governanceAtPoint.terms },
    {
      kind: "precedent",
      hits: [
        {
          entry_id: "m-1",
          text: "Sign in to manage your subscription.",
          locale: "en",
          discouraged: [],
        },
      ],
    },
    { kind: "profiles", hits: governanceAtPoint.profiles },
  ],
  notes: [],
};

export interface FixtureOptions {
  /** Notes every answer carries — the staleness note is the reason this exists. */
  notes?: string[];
  /** Omit `lives`, so the pane must state that the surface reads no content. */
  withoutContent?: boolean;
  /** Omit `relates` entirely. */
  withoutRelations?: boolean;
  /** Answer relations across the workspace rather than one project. */
  workspaceReach?: boolean;
  /** Fail every read, so the panes must show a load failure, not an empty set. */
  failing?: boolean;
  /**
   * Answer governance without the sections a source may not populate — rules,
   * clearance and the voice guide. kapi's local source carries none of the
   * first two, and a pane that drew an empty section for them would read as
   * broken rather than as silent.
   */
  bareGovernance?: boolean;
  /** Overrides merged onto the governance answer. */
  governance?: Partial<GovernanceAnswer>;
}

/** A source with the dimensions pinned — kapi's local project. */
export function makePinnedSource(opts: FixtureOptions = {}): ContextDataSource {
  return makeSource({ free: [], ...opts });
}

/** A source with the dimensions free — a Bowrain workspace. */
export function makeFreeSource(opts: FixtureOptions = {}): ContextDataSource {
  return makeSource({
    free: ["project", "stream", "collection", "coordinate", "locale"],
    workspaceReach: true,
    ...opts,
  });
}

function makeSource(opts: FixtureOptions & { free: Dimension[] }): ContextDataSource {
  const notes = opts.notes ?? [];
  const fail = <T>(): T => {
    throw new Error("the source is unreachable");
  };

  const source: ContextDataSource = {
    capabilities: {
      free: opts.free,
      governance: "resolved",
      relations: opts.withoutRelations ? "none" : opts.workspaceReach ? "workspace" : "project",
      content: !opts.withoutContent,
      search: true,
    },
    governs: () => {
      if (opts.failing) return fail<GovernanceAnswer>();
      const answer: GovernanceAnswer = { ...governanceAtPoint, notes };
      if (opts.bareGovernance) {
        delete answer.rules;
        delete answer.clearance;
      }
      return { ...answer, ...opts.governance };
    },
    search: () => (opts.failing ? fail<SearchAnswer>() : { ...searchAnswer, notes }),
    options: (_scope, dimension) =>
      dimension === "project"
        ? [
            { value: "site", label: "Northsea site", count: 24 },
            { value: "app", label: "Northsea app", count: 7 },
          ]
        : dimension === "locale"
          ? [{ value: "en" }, { value: "nb" }]
          : [],
  };

  if (!opts.withoutContent) {
    source.lives = () => (opts.failing ? fail<ContentAnswer>() : { ...contentAtPoint, notes });
  }
  if (!opts.withoutRelations) {
    const answer = opts.workspaceReach ? relationsAcrossWorkspace : relationsInProject;
    source.relates = () => (opts.failing ? fail<RelationsAnswer>() : { ...answer, notes });
  }
  return source;
}

/** The one note the freshness layer stamps when the graph moved under a reader. */
export const STALENESS_NOTE =
  "terms moved since this was last read here — re-read the project's context before continuing";
