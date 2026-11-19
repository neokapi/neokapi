import { describe, expect, it } from "vite-plus/test";

import { planFromServer, planProducts, planRuleLabel } from "../src/index.ts";

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
      sourceOnly: true,
      priority: undefined,
    });
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
