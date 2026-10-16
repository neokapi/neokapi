import { describe, expect, it } from "vite-plus/test";
import {
  candidateKind,
  candidateRows,
  defaultRowKey,
  nextRowKey,
  rowCorrections,
} from "./candidateRows";
import type { CandidateRule } from "./types";

const rule = (over: Partial<CandidateRule>): CandidateRule => ({
  term: "utilize",
  replacement: "use",
  correction_count: 3,
  dimension: "vocabulary",
  status: "pending",
  ...over,
});

describe("candidateRows", () => {
  it("puts each status in its group, in the list's order", () => {
    const rows = candidateRows([
      rule({ term: "leverage", status: "promoted", decided_at: "2026-10-01T00:00:00Z" }),
      rule({ term: "disrupt", status: "rejected", decided_at: "2026-10-02T00:00:00Z" }),
      rule({ term: "Globex", replacement: "", status: "approved" }),
      rule({ term: "synergy", replacement: "teamwork" }),
    ]);
    expect(rows.map((r) => r.group)).toEqual(["suggested", "approved", "promoted", "rejected"]);
  });

  it("makes a phrasing corrected to two forms one disagreement, most corrected form first", () => {
    const rows = candidateRows([
      rule({ replacement: "employ", correction_count: 3 }),
      rule({ replacement: "use", correction_count: 5 }),
      rule({ term: "synergy", replacement: "teamwork" }),
    ]);
    expect(rows[0].group).toBe("conflict");
    expect(rows[0].sides?.map((s) => s.replacement)).toEqual(["use", "employ"]);
    expect(rows[0].candidate.replacement).toBe("use");
    expect(rowCorrections(rows[0])).toBe(8);
    expect(rows).toHaveLength(2);
  });

  it("matches phrasings the way the server matches decisions, ignoring case", () => {
    const rows = candidateRows([
      rule({ term: "Utilize", replacement: "use" }),
      rule({ term: "utilize ", replacement: "employ" }),
    ]);
    expect(rows).toHaveLength(1);
    expect(rows[0].group).toBe("conflict");
  });

  it("folds one correction counted under two dimensions into one rule", () => {
    const rows = candidateRows([
      rule({ dimension: "vocabulary", correction_count: 4 }),
      rule({ dimension: "style", correction_count: 2 }),
    ]);
    expect(rows).toHaveLength(1);
    expect(rows[0].group).toBe("suggested");
    expect(rows[0].candidate.correction_count).toBe(6);
    expect(rows[0].candidate.dimension).toBe("vocabulary");
  });

  it("keeps a decided phrasing's forms apart, since the decision names none of them", () => {
    const rows = candidateRows([
      rule({ replacement: "use", status: "promoted" }),
      rule({ replacement: "employ", status: "promoted" }),
    ]);
    expect(rows.map((r) => r.group)).toEqual(["promoted", "promoted"]);
    expect(new Set(rows.map((r) => r.key)).size).toBe(2);
  });

  it("orders decided rows newest decision first", () => {
    const rows = candidateRows([
      rule({ term: "a", status: "promoted", decided_at: "2026-01-01T00:00:00Z" }),
      rule({ term: "b", status: "promoted", decided_at: "2026-03-01T00:00:00Z" }),
    ]);
    expect(rows.map((r) => r.candidate.term)).toEqual(["b", "a"]);
  });
});

describe("selection", () => {
  const rows = candidateRows([
    rule({ term: "a", correction_count: 9 }),
    rule({ term: "b", correction_count: 5 }),
    rule({ term: "c", status: "promoted" }),
  ]);
  const [a, b, c] = rows.map((r) => r.key);

  it("opens on the first row that waits for a decision", () => {
    expect(defaultRowKey(rows, new Set())).toBe(a);
    expect(defaultRowKey(rows, new Set([a]))).toBe(b);
    expect(defaultRowKey(rows, new Set([a, b]))).toBeUndefined();
  });

  it("moves past a decision to the next row that waits, never to a decided rule", () => {
    expect(nextRowKey(rows, a, new Set([a]))).toBe(b);
    expect(nextRowKey(rows, b, new Set([a, b]))).toBeNull();
    expect(nextRowKey(rows, b, new Set([b]))).toBe(a);
    expect(nextRowKey(rows, c, new Set())).toBe(a);
  });
});

describe("candidateKind", () => {
  it("draws a disagreement, an established rule, and each dimension", () => {
    const [conflict] = candidateRows([
      rule({ replacement: "use" }),
      rule({ replacement: "employ" }),
    ]);
    expect(candidateKind(conflict)).toBe("conflict");
    expect(candidateKind(candidateRows([rule({ status: "promoted" })])[0])).toBe("established");
    expect(candidateKind(candidateRows([rule({ dimension: "vocabulary" })])[0])).toBe("word");
    expect(candidateKind(candidateRows([rule({ dimension: "tone" })])[0])).toBe("note");
  });
});
