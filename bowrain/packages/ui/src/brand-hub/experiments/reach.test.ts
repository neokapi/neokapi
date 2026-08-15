import { describe, expect, it } from "vitest";
import type { ChangeSetImpact, Reach, ReachClass } from "../../types/brand-graph";
import { hasReach, reachSegments, reachTotal, splitCoversAffected, formatLocales } from "./reach";
import { byCollection } from "./blastRadius";

const cls = (over: Partial<ReachClass> = {}): ReachClass => ({
  blocks: 0,
  words: 0,
  collections: 0,
  projects: 0,
  targets: 0,
  approved: 0,
  locales: [],
  ...over,
});

const reach = (annotate: Partial<ReachClass>, transform: Partial<ReachClass>): Reach => ({
  annotate: cls(annotate),
  transform: cls(transform),
  transform_projects: [],
});

const impact = (over: Partial<ChangeSetImpact> = {}): ChangeSetImpact => ({
  total_blocks: 100,
  affected_blocks: 0,
  new_violations: 0,
  resolved: 0,
  words: 0,
  projects: null,
  samples: null,
  ...over,
});

describe("reachSegments", () => {
  it("sizes each class by its share and drops the empty one", () => {
    const segments = reachSegments(reach({ blocks: 30 }, { blocks: 10 }));
    expect(segments.map((s) => s.kind)).toEqual(["annotate", "transform"]);
    expect(segments[0].share).toBeCloseTo(0.75);
    expect(segments[1].share).toBeCloseTo(0.25);

    expect(reachSegments(reach({ blocks: 5 }, {})).map((s) => s.kind)).toEqual(["annotate"]);
    expect(reachSegments(reach({}, { blocks: 5 })).map((s) => s.kind)).toEqual(["transform"]);
  });

  it("leads with the larger class, so the bar's shape is the headline", () => {
    const segments = reachSegments(reach({ blocks: 2 }, { blocks: 40 }));
    expect(segments[0].kind).toBe("transform");
  });

  it("has no segments and no division by zero when nothing is affected", () => {
    expect(reachSegments(reach({}, {}))).toEqual([]);
    expect(reachTotal(reach({}, {}))).toBe(0);
  });
});

describe("hasReach", () => {
  it("is false without a split and false when the split is empty", () => {
    expect(hasReach(undefined)).toBe(false);
    expect(hasReach(impact())).toBe(false);
    expect(hasReach(impact({ reach: reach({}, {}) }))).toBe(false);
    expect(hasReach(impact({ reach: reach({ blocks: 1 }, {}) }))).toBe(true);
  });
});

describe("splitCoversAffected", () => {
  it("holds when each affected row is one block", () => {
    expect(
      splitCoversAffected(
        impact({ affected_blocks: 4, reach: reach({ blocks: 3 }, { blocks: 1 }) }),
      ),
    ).toBe(true);
  });

  it("does not hold when one block was reached in several locales", () => {
    expect(
      splitCoversAffected(impact({ affected_blocks: 6, reach: reach({ blocks: 3 }, {}) })),
    ).toBe(false);
  });
});

describe("formatLocales", () => {
  it("joins for reading and stays empty when there is nothing to say", () => {
    expect(formatLocales(["de", "nb"])).toBe("de, nb");
    expect(formatLocales([])).toBe("");
    expect(formatLocales(undefined)).toBe("");
  });
});

describe("byCollection", () => {
  const twoProjects = impact({
    affected_blocks: 9,
    projects: [
      {
        project_id: "p1",
        project_name: "Site",
        affected_blocks: 6,
        new_violations: 6,
        resolved: 0,
        words: 60,
        collections: [
          {
            collection_id: "c1",
            collection_name: "Docs",
            affected_blocks: 4,
            new_violations: 4,
            resolved: 0,
            words: 40,
            locales: [],
          },
          {
            collection_id: "c2",
            collection_name: "Marketing",
            affected_blocks: 2,
            new_violations: 2,
            resolved: 0,
            words: 20,
            locales: [],
          },
        ],
      },
      {
        project_id: "p2",
        project_name: "App",
        affected_blocks: 3,
        new_violations: 3,
        resolved: 0,
        words: 30,
        collections: [
          {
            collection_id: "c3",
            collection_name: "Docs",
            affected_blocks: 3,
            new_violations: 3,
            resolved: 0,
            words: 30,
            locales: [],
          },
        ],
      },
    ],
  });

  it("keeps two projects' same-named collections apart and qualifies the label", () => {
    const rows = byCollection(twoProjects);
    expect(rows.map((r) => r.label)).toEqual(["Site · Docs", "App · Docs", "Site · Marketing"]);
    expect(new Set(rows.map((r) => r.key)).size).toBe(3);
  });

  it("drops the project qualifier when only one project is affected", () => {
    const one = impact({ projects: [twoProjects.projects![0]] });
    expect(byCollection(one).map((r) => r.label)).toEqual(["Docs", "Marketing"]);
  });

  it("names an unresolved collection rather than rendering a blank bar", () => {
    const unresolved = impact({
      projects: [
        {
          project_id: "p1",
          project_name: "Site",
          affected_blocks: 1,
          new_violations: 1,
          resolved: 0,
          words: 5,
          collections: [
            {
              collection_id: "",
              collection_name: "",
              affected_blocks: 1,
              new_violations: 1,
              resolved: 0,
              words: 5,
              locales: [],
            },
          ],
        },
      ],
    });
    expect(byCollection(unresolved)[0].label).toBe("unassigned");
  });
});
