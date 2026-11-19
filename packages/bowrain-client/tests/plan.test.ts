import { describe, expect, it } from "vite-plus/test";

import {
  coordinateGrid,
  coordinatesFromServer,
  coordinatesOpen,
  planFromServer,
  planProducts,
  planRuleLabel,
} from "../src/index.ts";

const server = {
  stream: "main",
  involvement: "context",
  next: "key_language",
  coordinates: {
    source_language: "en",
    target_languages: ["nb"],
    points: [
      {
        collection: "neokapi-cli",
        coordinates: { product: "neokapi", channel: "cli" },
        voice: "neokapi-documentation",
        priority: "high",
      },
      { collection: "neokapi-docs", coordinates: { product: "neokapi", channel: "docs" } },
      {
        collection: "source-go-comments",
        coordinates: { product: "source", channel: "comments" },
        source_only: true,
      },
    ],
    confirmed: true,
    confirmed_at: "2026-11-19T01:00:00Z",
  },
  key_language: {
    waiting: 12,
    by_kind: { concept: 9, voice_rule: 3 },
    concepts: 57,
    untranslated: { nb: 20 },
  },
  exceptions: {
    source_findings: 593,
    checked_at: "2026-11-19T00:00:00Z",
    by_rule: {
      source: [
        {
          key: "terms.vocabulary:utilize",
          rule: "terms.vocabulary",
          subject: "utilize",
          message: 'Use "use" instead of "utilize"',
          concept_id: "c-use",
          fails: true,
          count: 210,
        },
      ],
      targets: {
        nb: [{ key: "qa.trailing-whitespace", rule: "qa.trailing-whitespace", count: 12 }],
        de: null,
      },
    },
  },
  languages: [{ locale: "nb", waiting: 1200, failing: 40, pages: 300, settleable: 800 }],
};

describe("planFromServer", () => {
  it("maps the plan and keeps unknown values safe", () => {
    const plan = planFromServer(server);
    expect(plan.next).toBe("key_language");
    expect(plan.involvement).toBe("context");
    expect(plan.coordinates.points[2]).toEqual({
      collection: "source-go-comments",
      coordinates: { product: "source", channel: "comments" },
      voice: undefined,
      voiceId: undefined,
      sourceOnly: true,
      priority: undefined,
    });
    expect(plan.coordinates.changes).toEqual([]);
    expect(plan.coordinates.changed).toBe(0);
    expect(coordinatesOpen(plan.coordinates)).toBe(false);
    expect(plan.keyLanguage.untranslated).toEqual({ nb: 20 });
    expect(plan.exceptions.sourceFindings).toBe(593);
    expect(plan.exceptions.byRule.source[0]).toEqual({
      key: "terms.vocabulary:utilize",
      rule: "terms.vocabulary",
      subject: "utilize",
      message: 'Use "use" instead of "utilize"',
      conceptId: "c-use",
      fails: true,
      count: 210,
    });
    expect(plan.exceptions.byRule.targets).toEqual({
      nb: [
        {
          key: "qa.trailing-whitespace",
          rule: "qa.trailing-whitespace",
          subject: undefined,
          message: undefined,
          conceptId: undefined,
          fails: false,
          count: 12,
        },
      ],
      de: [],
    });
    expect(planRuleLabel(plan.exceptions.byRule.source[0]!)).toBe('Use "use" instead of "utilize"');
    expect(planRuleLabel(plan.exceptions.byRule.targets.nb![0]!)).toBe("qa.trailing-whitespace");
    expect(plan.languages[0]).toMatchObject({ locale: "nb", settleable: 800, pagesMore: false });

    const bare = planFromServer({ ...server, involvement: "sometimes", next: "", exceptions: {} });
    expect(bare.involvement).toBe("context");
    expect(bare.next).toBeNull();
    expect(bare.exceptions.sourceFindings).toBeNull();
    expect(bare.exceptions.byRule).toEqual({ source: [], targets: {} });
    expect(bare.steps).toEqual([]);
    expect(bare.held).toBeNull();
    expect(bare.languages[0]?.held).toBe(0);
  });

  it("maps the step states and the held translations", () => {
    const plan = planFromServer({
      ...server,
      held: "key_language",
      languages: [{ ...server.languages[0]!, waiting: 40, held: 1160 }],
      steps: [
        { level: "coordinates", state: "attention", count: 2, progress: 100, changed: 2 },
        { level: "key_language", state: "not_started", count: 0, progress: 0, scan: "failed" },
        { level: "exceptions", state: "open", count: 3, progress: 50 },
        { level: "interfaces", state: "something_new", count: 34, progress: 0 },
        { level: "policy", count: 0, progress: 100, done: true },
      ],
    });
    expect(plan.held).toBe("key_language");
    expect(plan.languages[0]).toMatchObject({ waiting: 40, held: 1160 });
    expect(plan.steps.map((s) => s.state)).toEqual([
      "attention",
      "not_started",
      "open",
      "open",
      "done",
    ]);
    expect(plan.steps[1]?.scan).toBe("failed");
  });
});

describe("planProducts", () => {
  it("groups the points' channels by product", () => {
    expect(planProducts(planFromServer(server).coordinates.points)).toEqual([
      { product: "neokapi", channels: ["cli", "docs"] },
      { product: "source", channels: ["comments"] },
    ]);
  });
});

describe("coordinatesFromServer", () => {
  const map = coordinatesFromServer({
    stream: "main",
    coordinates: {
      ...server.coordinates,
      confirmed_by: "u-ada",
      confirmed_by_name: "Ada",
      changes: [
        {
          kind: "point_added",
          collection: "neokapi-mobile",
          coordinates: { product: "neokapi", channel: "mobile" },
          voice: "app",
        },
        { kind: "language_added", locale: "sv" },
        { kind: "something_new" },
      ],
      changed: 3,
    },
    cells: [
      {
        product: "neokapi",
        channel: "mobile",
        collections: [
          {
            name: "neokapi-mobile",
            coordinates: { product: "neokapi", channel: "mobile" },
            voice: "app",
            blocks: 412,
            coverage: { nb: 0.25 },
          },
        ],
        blocks: 412,
        voices: ["app"],
        coverage: { nb: 0.25 },
        changes: ["point_added", "something_new"],
      },
      { product: "neokapi", channel: "cli", blocks: 2140, coverage: null },
      { product: "bowrain", channel: "app", blocks: 3576 },
    ],
  });

  it("maps the changes, the confirmer and the cells", () => {
    expect(map.coordinates.confirmedByName).toBe("Ada");
    expect(map.coordinates.changes.map((c) => c.kind)).toEqual(["point_added", "language_added"]);
    expect(map.coordinates.changes[1]).toMatchObject({ locale: "sv", coordinates: {} });
    expect(map.coordinates.changed).toBe(3);
    expect(coordinatesOpen(map.coordinates)).toBe(true);
    expect(map.cells[0]).toMatchObject({
      blocks: 412,
      voices: ["app"],
      sourceOnly: false,
      coverage: { nb: 0.25 },
      changes: ["point_added"],
    });
    expect(map.cells[0]!.collections[0]).toMatchObject({ name: "neokapi-mobile", blocks: 412 });
    expect(map.cells[1]).toMatchObject({ collections: [], voices: [], coverage: {}, changes: [] });
  });

  it("lays the cells out as products by channels", () => {
    const grid = coordinateGrid(map.cells);
    expect(grid.products).toEqual(["bowrain", "neokapi"]);
    expect(grid.channels).toEqual(["app", "cli", "mobile"]);
    expect(grid.at("neokapi", "cli")?.blocks).toBe(2140);
    expect(grid.at("bowrain", "cli")).toBeUndefined();
  });
});
