import { describe, it, expect } from "vite-plus/test";
import type { ChangeSet } from "../../types/brand-graph";
import type { StoredScore } from "../../brand/types";
import {
  averageScore,
  aggregateDimensions,
  waitingSince,
  sortByWaiting,
  sortByRecent,
  activeExperiments,
  pendingDecisions,
} from "./metrics";

function changeset(id: string, partial: Partial<ChangeSet>): ChangeSet {
  return {
    id,
    workspace_id: "ws-1",
    name: id,
    status: "draft",
    created_by: "alex",
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...partial,
  };
}

describe("averageScore", () => {
  const score = (s: number): StoredScore => ({
    id: `s-${s}`,
    project_id: "p",
    stream: "main",
    block_id: "b",
    profile_id: "pr",
    locale: "en-US",
    score: s,
    dimensions: [],
    findings: [],
    checked_at: "2026-06-01T00:00:00Z",
  });

  it("returns null when empty", () => {
    expect(averageScore([])).toBeNull();
  });

  it("rounds the mean", () => {
    expect(averageScore([score(80), score(90), score(85)])).toBe(85);
    expect(averageScore([score(81), score(82)])).toBe(82); // 81.5 → 82
  });
});

describe("aggregateDimensions", () => {
  it("averages dimensions, sums issues, and orders canonically", () => {
    const mk = (overrides: Partial<StoredScore>): StoredScore => ({
      id: "s",
      project_id: "p",
      stream: "main",
      block_id: "b",
      profile_id: "pr",
      locale: "en-US",
      score: 0,
      findings: [],
      checked_at: "2026-06-01T00:00:00Z",
      dimensions: [],
      ...overrides,
    });
    const dims = aggregateDimensions([
      mk({
        dimensions: [
          { dimension: "vocabulary", score: 70, penalty: 10, issues: 2 },
          { dimension: "tone", score: 90, penalty: 0, issues: 0 },
        ],
      }),
      mk({
        dimensions: [
          { dimension: "vocabulary", score: 90, penalty: 4, issues: 1 },
          { dimension: "tone", score: 80, penalty: 2, issues: 1 },
        ],
      }),
    ]);
    expect(dims.map((d) => d.dimension)).toEqual(["tone", "vocabulary"]);
    const vocab = dims.find((d) => d.dimension === "vocabulary");
    expect(vocab).toMatchObject({ score: 80, issues: 3 });
  });
});

describe("change-set ordering", () => {
  const a = changeset("a", { status: "in_review", submitted_at: "2026-06-05T00:00:00Z" });
  const b = changeset("b", { status: "approved", submitted_at: "2026-06-02T00:00:00Z" });
  const c = changeset("c", { status: "draft", updated_at: "2026-06-09T00:00:00Z" });
  const d = changeset("d", { status: "merged", updated_at: "2026-06-10T00:00:00Z" });

  it("waitingSince prefers submitted_at", () => {
    expect(waitingSince(a)).toBe("2026-06-05T00:00:00Z");
    expect(waitingSince(c)).toBe("2026-06-09T00:00:00Z");
  });

  it("sortByWaiting is oldest-first", () => {
    expect(sortByWaiting([a, b]).map((x) => x.id)).toEqual(["b", "a"]);
  });

  it("sortByRecent is newest-first", () => {
    expect(sortByRecent([a, c, d]).map((x) => x.id)).toEqual(["d", "c", "a"]);
  });

  it("activeExperiments drops merged and abandoned", () => {
    expect(activeExperiments([a, b, c, d]).map((x) => x.id)).toEqual(["a", "b", "c"]);
  });

  it("pendingDecisions keeps in_review and approved, oldest-first", () => {
    expect(pendingDecisions([a, b, c, d]).map((x) => x.id)).toEqual(["b", "a"]);
  });
});
