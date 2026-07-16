import { describe, it, expect } from "vite-plus/test";
import type { DriftResult, StoredScore } from "../../brand/types";
import { aggregateBrandRollup, rollupEntry, type RollupProjectInput } from "./rollup";

function score(s: number, checkedAt: string): StoredScore {
  return {
    id: `s-${s}-${checkedAt}`,
    project_id: "p",
    stream: "main",
    block_id: "b",
    profile_id: "pr",
    locale: "en-US",
    score: s,
    dimensions: [
      { dimension: "vocabulary", score: s, penalty: 100 - s, issues: 1 },
      { dimension: "tone", score: s, penalty: 100 - s, issues: 0 },
    ],
    findings: [],
    checked_at: checkedAt,
  };
}

const drifting: DriftResult = {
  drifted: true,
  recent_avg: 72,
  baseline_avg: 84,
  drop: 12,
  recent_days: 7,
  recent_count: 6,
};

describe("rollupEntry", () => {
  it("folds scores + drift into a row (overall mean, block count, last check, trend)", () => {
    const input: RollupProjectInput = {
      id: "p1",
      name: "Marketing",
      scores: [score(80, "2026-06-01T00:00:00Z"), score(90, "2026-06-09T00:00:00Z")],
      drift: drifting,
    };
    const entry = rollupEntry(input);
    expect(entry).toMatchObject({
      project_id: "p1",
      project_name: "Marketing",
      overall: 85,
      scored_blocks: 2,
      last_scored_at: "2026-06-09T00:00:00Z",
      trend: "down",
    });
    expect(entry.dimensions?.map((d) => d.dimension)).toEqual(["tone", "vocabulary"]);
  });

  it("represents a never-scored project as null overall and no activity", () => {
    const entry = rollupEntry({ id: "p2", name: "Empty", scores: [] });
    expect(entry.overall).toBeNull();
    expect(entry.scored_blocks).toBe(0);
    expect(entry.last_scored_at).toBeNull();
    expect(entry.trend).toBe(""); // no drift → unknown
  });
});

describe("aggregateBrandRollup", () => {
  const inputs: RollupProjectInput[] = [
    { id: "p-c", name: "Charlie", scores: [score(60, "2026-06-01T00:00:00Z")] },
    { id: "p-a", name: "Alpha", scores: [score(95, "2026-06-02T00:00:00Z")] },
    { id: "p-b", name: "Bravo", scores: [] },
  ];

  it("orders by name and reports the full total", () => {
    const rollup = aggregateBrandRollup(inputs, 0, 50);
    expect(rollup.projects.map((p) => p.project_name)).toEqual(["Alpha", "Bravo", "Charlie"]);
    expect(rollup.total).toBe(3);
    expect(rollup.limit).toBe(50);
    expect(rollup.offset).toBe(0);
  });

  it("paginates within the sorted order", () => {
    const rollup = aggregateBrandRollup(inputs, 1, 1);
    expect(rollup.projects.map((p) => p.project_name)).toEqual(["Bravo"]);
    expect(rollup.total).toBe(3);
  });
});
