// Feed entries for stories and tests, dated from now so they read under Today
// and Yesterday. One of each kind, plus generated pages and long content.
import type { FeedEntry } from "@neokapi/bowrain-client";

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

const ada = { id: "u-ada", kind: "person" as const, name: "Ada Lindqvist" };

/** One entry of every kind. */
export const feedEveryKind: FeedEntry[] = [
  {
    id: "e-push",
    kind: "push",
    at: minutesAgo(4),
    actor: ada,
    counts: { files: 3, blocks: 40, collections: 2, files_removed: 1 },
    stream: "main",
    link: { screen: "stream", id: "push-1", stream: "main" },
  },
  {
    id: "e-run",
    kind: "convergence_run",
    at: minutesAgo(9),
    actor: { id: "system", kind: "tool", name: "convergence" },
    counts: { passes: 2, locales: 2, produced: 40 },
    locales: ["nb", "de"],
    state: "converged",
    standing: [
      { locale: "nb", state: "shippable", units: 30, produced: 30 },
      { locale: "de", state: "pending", units: 24, produced: 10 },
    ],
    link: { screen: "run", id: "run-1" },
  },
  {
    id: "e-context",
    kind: "context_change",
    at: minutesAgo(30),
    actor: { id: "u-ada", kind: "agent", name: "claude-code", session: "0ab4e399", host: "laptop" },
    on_behalf_of: { id: "u-ada", name: "Ada Lindqvist" },
    counts: { term_rule: 6, add: 6 },
    samples: [
      {
        kind: "term_rule",
        operation: "add",
        term: "translation memory",
        replacement: "content memory",
      },
      { kind: "term_rule", operation: "add", term: "termbase", replacement: "terms store" },
      { kind: "term_rule", operation: "add", term: "glossary", replacement: "terms store" },
    ],
    link: { screen: "context_deck", mode: "pending" },
  },
  {
    id: "e-review",
    kind: "review_summary",
    at: minutesAgo(70),
    actor: ada,
    counts: { approved: 28, rejected: 2, bulk_approved: 120 },
    locales: ["nb"],
    stream: "main",
    link: { screen: "review_deck", locale: "nb", stream: "main" },
  },
  {
    id: "e-milestone",
    kind: "coverage_milestone",
    at: minutesAgo(75),
    actor: { id: "system", kind: "tool" },
    counts: { percent: 90, blocks: 1840 },
    locales: ["nb"],
    link: { screen: "language", locale: "nb" },
  },
  {
    id: "e-recipe",
    kind: "recipe_change",
    at: minutesAgo(26 * 60),
    actor: ada,
    counts: { coordinates_changed: 1, targets_added: 1 },
    locales: ["fr"],
    coordinates: { product: "bowrain" },
    diff: {
      collections: [
        {
          name: "docs",
          change: "coordinates",
          before: { product: "kapi" },
          after: { product: "bowrain" },
        },
      ],
      targets: { before: ["nb", "de"], after: ["nb", "de", "fr"] },
    },
    link: { screen: "recipe_diff", id: "e-recipe" },
  },
  {
    id: "e-changeset",
    kind: "change_set",
    at: minutesAgo(27 * 60),
    actor: { id: "u-sam", kind: "person", name: "Sam Okafor" },
    counts: { "term.add": 3, "term.status": 1 },
    name: "Checkout terms",
    state: "merged",
    link: { screen: "change_set", id: "cs-1" },
  },
];

const convergence = { id: "system", kind: "tool" as const, name: "convergence" };

/** Three runs within the fold window, and a quiet run the feed leaves out. */
export const feedFolded: FeedEntry[] = [
  {
    id: "e-fold-1",
    kind: "convergence_run",
    at: minutesAgo(3),
    actor: convergence,
    counts: { passes: 1, locales: 2, produced: 12 },
    locales: ["nb", "de"],
    state: "converged",
    standing: [
      { locale: "nb", state: "shippable", units: 12, produced: 12 },
      { locale: "de", state: "shippable", units: 8, produced: 8 },
    ],
    link: { screen: "run", id: "run-f1" },
  },
  {
    id: "e-fold-quiet",
    kind: "convergence_run",
    at: minutesAgo(6),
    actor: convergence,
    counts: { passes: 0, locales: 0, produced: 0 },
    state: "converged",
    link: { screen: "run", id: "run-q" },
  },
  {
    id: "e-fold-2",
    kind: "convergence_run",
    at: minutesAgo(9),
    actor: convergence,
    counts: { passes: 1, locales: 1, produced: 4 },
    locales: ["nb"],
    state: "converged",
    link: { screen: "run", id: "run-f2" },
  },
  {
    id: "e-fold-3",
    kind: "convergence_run",
    at: minutesAgo(20),
    actor: convergence,
    counts: { passes: 2, locales: 1, produced: 7 },
    locales: ["de"],
    state: "converged",
    link: { screen: "run", id: "run-f3" },
  },
];

/** A busy project: many pushes and runs over several days. */
export const feedMany: FeedEntry[] = Array.from({ length: 140 }, (_, i) => {
  const base = feedEveryKind[i % feedEveryKind.length];
  return { ...base, id: `e-many-${i}`, at: minutesAgo(i * 41) };
});

/** Names and values longer than a row has room for. */
export const feedLong: FeedEntry[] = [
  {
    id: "e-long-push",
    kind: "push",
    at: minutesAgo(2),
    actor: { id: "u-x", kind: "person", name: "Maria-Luisa Fernández de la Cruz Montenegro" },
    counts: { files: 1204, blocks: 204_315 },
    stream: "release-2026-q4-holiday-campaign-with-a-long-name",
    link: { screen: "stream", stream: "release-2026-q4-holiday-campaign-with-a-long-name" },
  },
  {
    id: "e-long-recipe",
    kind: "recipe_change",
    at: minutesAgo(6),
    actor: { id: "u-x", kind: "person", name: "Maria-Luisa Fernández de la Cruz Montenegro" },
    counts: { settings_changed: 2, collections_added: 1 },
    diff: {
      collections: [
        {
          name: "marketing-site-landing-pages",
          change: "added",
          after: { brand: "fernwell", product: "checkout", channel: "web", market: "nordics" },
        },
      ],
      settings: [
        { name: "converge_policy", before: "on-push", after: "manual" },
        { name: "translate_after", before: "", after: "established" },
      ],
    },
    link: { screen: "recipe_diff", id: "e-long-recipe" },
  },
  ...feedEveryKind,
];
