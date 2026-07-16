import { describe, it, expect } from "vite-plus/test";
import type { ConceptInfo } from "../../types/api";
import type { ChangeSet } from "../../types/brand-graph";
import type { DriftResult, StoredScore } from "../../brand/types";
import {
  computeLocaleCoverage,
  averageScore,
  aggregateDimensions,
  trendDirection,
  latestChecked,
  waitingSince,
  sortByWaiting,
  sortByRecent,
  activeExperiments,
  pendingDecisions,
} from "./metrics";

function concept(id: string, locales: string[]): ConceptInfo {
  return {
    id,
    domain: "commerce",
    definition: "",
    terms: locales.map((locale) => ({ text: `${id}-${locale}`, locale, status: "preferred" })),
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
  };
}

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

describe("computeLocaleCoverage", () => {
  it("returns empty for no concepts", () => {
    expect(computeLocaleCoverage([])).toEqual([]);
  });

  it("counts each concept once per locale and sorts by coverage", () => {
    const cov = computeLocaleCoverage([
      concept("a", ["en-US", "de-DE"]),
      concept("b", ["en-US"]),
      concept("c", ["en-US", "fr-FR"]),
    ]);
    // en-US in all 3, de-DE + fr-FR in 1 each.
    expect(cov[0]).toEqual({ locale: "en-US", present: 3, total: 3, pct: 100 });
    expect(cov.map((c) => c.locale)).toEqual(["en-US", "de-DE", "fr-FR"]);
    const de = cov.find((c) => c.locale === "de-DE");
    expect(de).toEqual({ locale: "de-DE", present: 1, total: 3, pct: 33 });
  });

  it("does not double-count duplicate locales within one concept", () => {
    const c: ConceptInfo = {
      ...concept("dup", []),
      terms: [
        { text: "a", locale: "en-US", status: "preferred" },
        { text: "b", locale: "en-US", status: "admitted" },
      ],
    };
    const cov = computeLocaleCoverage([c]);
    expect(cov).toEqual([{ locale: "en-US", present: 1, total: 1, pct: 100 }]);
  });
});

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

describe("trendDirection", () => {
  const drift = (partial: Partial<DriftResult>): DriftResult => ({
    drifted: false,
    recent_avg: 80,
    baseline_avg: 80,
    drop: 0,
    recent_days: 7,
    recent_count: 5,
    ...partial,
  });

  it("is unknown when there is no drift data", () => {
    expect(trendDirection(undefined)).toBe("");
  });

  it("is flat when nothing was scored recently", () => {
    expect(trendDirection(drift({ recent_count: 0, recent_avg: 0, baseline_avg: 90 }))).toBe(
      "flat",
    );
  });

  it("rises, falls, and holds around a one-point band", () => {
    expect(trendDirection(drift({ recent_avg: 85, baseline_avg: 80 }))).toBe("up");
    expect(trendDirection(drift({ recent_avg: 74, baseline_avg: 82 }))).toBe("down");
    expect(trendDirection(drift({ recent_avg: 80.4, baseline_avg: 80 }))).toBe("flat");
  });
});

describe("latestChecked", () => {
  const at = (iso: string): StoredScore => ({
    id: iso,
    project_id: "p",
    stream: "main",
    block_id: "b",
    profile_id: "pr",
    locale: "en-US",
    score: 80,
    dimensions: [],
    findings: [],
    checked_at: iso,
  });

  it("returns null when empty", () => {
    expect(latestChecked([])).toBeNull();
  });

  it("returns the most recent timestamp regardless of order", () => {
    expect(
      latestChecked([
        at("2026-06-01T00:00:00Z"),
        at("2026-06-09T00:00:00Z"),
        at("2026-06-05T00:00:00Z"),
      ]),
    ).toBe("2026-06-09T00:00:00Z");
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
