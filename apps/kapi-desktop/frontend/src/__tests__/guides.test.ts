// Every tour step points at an element the frontend draws, and every help
// topic's website link names a page that exists. Both are read from the
// sources, so a renamed slot or a moved page fails here rather than leaving a
// bubble in the middle of the window or a link to nowhere.

import { describe, expect, it, vi } from "vite-plus/test";
import { buildGuides, GUIDE_IDS } from "../guide/guides";
import { buildTopics } from "../guide/topics";

const sources = Object.values(
  import.meta.glob<string>(
    [
      "../**/*.tsx",
      "!../__tests__/**",
      "!../stories/**",
      "../../../../../packages/project-screens/src/**/*.tsx",
      "../../../../../packages/ui/src/components/ui/stat-row.tsx",
    ],
    { query: "?raw", import: "default", eager: true },
  ),
).join("\n");

const kapiPages = Object.keys(
  import.meta.glob("../../../../../web/docs/kapi/**/*.{md,mdx}", { eager: false }),
).map((p) => p.replace(/^.*\/web\/docs\//, "").replace(/\.mdx?$/, ""));
const bowrainPages = Object.keys(
  import.meta.glob("../../../../../bowrain/web/docs/docs/**/*.{md,mdx}", { eager: false }),
).map((p) => p.replace(/^.*\/bowrain\/web\/docs\/docs\//, "").replace(/\.mdx?$/, ""));

const guides = buildGuides({ navigate: vi.fn(), hasProject: () => true });
const topics = buildTopics();

describe("guides", () => {
  it("reads the sources it checks against", () => {
    expect(sources).toContain('data-slot="overview-counts"');
    expect(kapiPages).toContain("kapi/desktop/overview");
    expect(bowrainPages).toContain("getting-started/kapi-vs-bowrain");
  });

  const steps = guides.flatMap((g) => g.steps.map((s) => ({ guide: g.id, ...s })));
  it.each(steps.map((s) => [`${s.guide} / ${s.id}`, s.target ?? ""]))(
    "%s points at a slot the frontend draws",
    (_, target) => {
      const slots = [...target.matchAll(/data-slot="([^"]+)"/g)].map((m) => m[1]);
      expect(slots.length).toBeGreaterThan(0);
      for (const slot of slots) {
        // The rail's entries carry their slot as a JSX attribute like any other.
        expect(sources, `data-slot="${slot}"`).toContain(`data-slot="${slot}"`);
      }
      for (const [, view] of target.matchAll(/data-view="([^"]+)"/g)) {
        expect(sources, `rail view ${view}`).toContain(`view: "${view}"`);
      }
    },
  );

  it("names a topic that exists for each tour that names one", () => {
    const ids = new Set(topics.map((t) => t.id));
    for (const g of guides) if (g.topic) expect(ids.has(g.topic), g.topic).toBe(true);
  });

  it("starts nowhere without a project", () => {
    const closed = buildGuides({ navigate: vi.fn(), hasProject: () => false });
    for (const g of closed) {
      expect(g.available?.()).toEqual({ ok: false, reason: "Open a project first." });
    }
  });

  it("opens each tour's first page before its first step", () => {
    const navigate = vi.fn();
    const live = buildGuides({ navigate, hasProject: () => true });
    const first = (id: string) => live.find((g) => g.id === id)!.steps[0]!.before?.();
    first(GUIDE_IDS.project);
    first(GUIDE_IDS.decide);
    first(GUIDE_IDS.check);
    first(GUIDE_IDS.languages);
    first(GUIDE_IDS.assistant);
    expect(navigate.mock.calls.map((c) => c[0])).toEqual([
      "map",
      "context:rules",
      "map",
      "project-settings",
      "project-settings:assistants",
    ]);
  });
});

describe("help topics", () => {
  it.each(topics.map((t) => [t.id, t.moreUrl ?? ""]))("%s links a page that exists", (_, url) => {
    const kapi = url.match(/^https:\/\/neokapi\.github\.io\/(kapi\/.+)$/);
    const bowrain = url.match(/^https:\/\/bowrain\.cloud\/docs\/(.+)$/);
    expect(kapi || bowrain, url).toBeTruthy();
    if (kapi) expect(kapiPages).toContain(kapi[1]);
    if (bowrain) expect(bowrainPages).toContain(bowrain[1]);
  });

  it("names guides that exist", () => {
    const ids = new Set(guides.map((g) => g.id));
    for (const t of topics) for (const g of t.guides ?? []) expect(ids.has(g), g).toBe(true);
  });

  it("keeps retired words and the em dash out of the copy", () => {
    const copy = [
      ...topics.flatMap((t) => [t.title, t.summary, t.body]),
      ...guides.flatMap((g) => [
        g.title,
        g.summary,
        ...g.steps.flatMap((s) => [s.title, String(s.body)]),
      ]),
    ];
    for (const text of copy)
      expect(text).not.toMatch(/—|simply|easily|glossary|termbase|translation memory/i);
  });
});
