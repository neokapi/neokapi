// Each fixture is checked here for the fields the views read, including the
// values a union type allows. src/fixtures.ts checks the same files against
// the types at the type level.
import { describe, it, expect } from "vite-plus/test";

import {
  mapSummaryFixture as map,
  readerFileFixture as reader,
  rulesDecideViewFixture as decide,
} from "../fixtures";
import type { ContextActor, DigestItem, ReaderFinding, RuleDecision } from "../types";

const ACTOR_KINDS = ["person", "agent", "tool"];
const DECISION_KINDS = ["name", "word", "note", "memory", "disagreement"];
const VERDICTS = ["passed", "failed", "did_not_run"];

function expectActor(a: ContextActor) {
  expect(ACTOR_KINDS).toContain(a.kind);
}

function expectDigestItem(it: DigestItem) {
  expect(it.id).not.toBe("");
  expect(it.short).not.toBe("");
  expect(it.sentence).not.toBe("");
  expect(["names", "words", "writing", "memory"]).toContain(it.theme);
  expect(ACTOR_KINDS).toContain(it.noticed_by.kind);
  expect(Number.isNaN(Date.parse(it.at))).toBe(false);
  expect(typeof it.keepable).toBe("boolean");
  expect(typeof it.droppable).toBe("boolean");
}

function expectFinding(f: ReaderFinding) {
  expect(f.id).not.toBe("");
  expect(f.kind).not.toBe("");
  expect(f.label).not.toBe("");
  expect(typeof f.fails).toBe("boolean");
  expect(typeof f.fixable).toBe("boolean");
  if (f.span) expect(f.span.end).toBeGreaterThan(f.span.start);
}

describe("map-summary.json", () => {
  it("holds the counts the Map shows", () => {
    for (const n of [map.files, map.blocks, map.rules, map.read_files]) {
      expect(Number.isInteger(n)).toBe(true);
    }
    expect(map.findings === null || Number.isInteger(map.findings)).toBe(true);
    expect(map.awaiting).toBe(map.suggested + map.conflicts);
    expect(map.target_languages.length).toBeGreaterThan(0);
  });

  it("has one row per file with its blocks and findings", () => {
    expect(map.file_rows.length).toBeGreaterThan(1);
    expect(map.file_rows.length).toBe(map.files);
    for (const row of map.file_rows) {
      expect(row.path).toMatch(/^\//);
      expect(row.relative).not.toMatch(/^\//);
      expect(Number.isInteger(row.blocks)).toBe(true);
      expect(Number.isInteger(row.findings)).toBe(true);
    }
  });

  it("lists each suggestion with its item, and each conflict with its sides", () => {
    expect(map.suggestions.length).toBe(map.awaiting);
    for (const row of map.suggestions) {
      expect(["conflict", "suggested"]).toContain(row.group);
      expectDigestItem(row.item);
      if (row.group === "conflict") {
        expect(row.conflict?.sides.length).toBeGreaterThan(1);
        expect(row.conflict?.sides[0].id).toBe(row.item.id);
        row.conflict?.sides.forEach(expectDigestItem);
      } else {
        expect(row.conflict).toBeUndefined();
      }
    }
  });
});

describe("reader-file.json", () => {
  it("draws the file's blocks with the findings on them", () => {
    expect(VERDICTS).toContain(reader.verdict);
    expect(reader.blocks.length).toBeGreaterThan(1);
    for (const block of reader.blocks) {
      expect(block.id).not.toBe("");
      expect(block.runs.length).toBeGreaterThan(0);
      for (const f of block.findings) {
        expectFinding(f);
        expect(f.block_id).toBe(block.id);
      }
    }
    reader.document.forEach(expectFinding);
  });

  it("counts every finding and the fixable ones", () => {
    const all = [...reader.blocks.flatMap((b) => b.findings), ...reader.document];
    expect(reader.findings).toBe(all.length);
    expect(reader.fixable).toBe(all.filter((f) => f.fixable).length);
    expect(reader.fixable).toBeGreaterThan(0);
  });

  it("carries a plural block as a plural run", () => {
    const plural = reader.blocks.flatMap((b) => b.runs).find((r) => "plural" in r);
    expect(plural && "plural" in plural ? Object.keys(plural.plural.forms) : []).toContain("other");
  });

  it("lists the rules, the attention lines and the refused fixes", () => {
    expect(reader.rules.length).toBeLessThanOrEqual(reader.rules_total);
    for (const rule of reader.rules) {
      expect(rule.kind).not.toBe("");
      expect(rule.rule).not.toBe("");
      expect(["established", "suggested", "contested"]).toContain(rule.status);
    }
    expect(Array.isArray(reader.attention)).toBe(true);
    for (const r of reader.refused) {
      expect(r.finding_id).not.toBe("");
      expect(r.reason).not.toBe("");
    }
  });
});

describe("rules-decide-view.json", () => {
  it("names the project and its counts", () => {
    expect(decide.project).not.toBe("");
    expect(decide.project_name).not.toBe("");
    for (const n of [decide.rules, decide.suggested, decide.narrowed]) {
      expect(Number.isInteger(n)).toBe(true);
    }
  });

  it("asks each question with its options and who noticed it", () => {
    expect(decide.decisions.length).toBeGreaterThan(0);
    for (const d of decide.decisions as RuleDecision[]) {
      expect(DECISION_KINDS).toContain(d.kind);
      expect(d.question).not.toBe("");
      expect(d.label).not.toBe("");
      expect(Array.isArray(d.options)).toBe(true);
      expectActor(d.noticed_by);
      if (d.options.length > 0) {
        expect(d.chosen).toBeGreaterThanOrEqual(0);
        expect(d.chosen).toBeLessThan(d.options.length);
      } else {
        expect(d.consequence).toBeTruthy();
      }
      for (const o of d.options) {
        expect(o.consequence).not.toBe("");
        expect(Number.isInteger(o.blocks) && Number.isInteger(o.files)).toBe(true);
        if (d.kind === "disagreement") {
          expect(o.id).toBeTruthy();
          expectActor(o.noticed_by as ContextActor);
        }
      }
    }
  });

  it("holds an actor with no name", () => {
    const actors = decide.decisions.flatMap((d) => [
      d.noticed_by,
      ...d.options.flatMap((o) => (o.noticed_by ? [o.noticed_by] : [])),
    ]);
    expect(actors.some((a) => a.name === undefined)).toBe(true);
  });
});
