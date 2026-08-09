import { describe, it, expect } from "vite-plus/test";
import type { BlockInfo, QAIssue } from "../types/api";
import {
  entryKey,
  entryCheckStatus,
  entryHasErrors,
  isPendingReview,
  isEntryClean,
  matchesFilter,
  filterEntries,
  groupEntries,
  queueCounts,
  noFailingChecksCount,
  nextIndex,
  indexAfterRemoval,
  type ReviewEntry,
} from "../components/review/reviewQueue";

function block(id: string, locale: string, text: string, status = "translated"): BlockInfo {
  return {
    id,
    source: `source ${id}`,
    targets: { [locale]: { text, status: status as never } },
    translatable: true,
    has_spans: false,
    properties: {},
  };
}

function entry(overrides: Partial<ReviewEntry> & { locale: string; itemId: string }): ReviewEntry {
  const blockId = overrides.block?.id ?? "b1";
  return {
    id: entryKey(overrides.itemId, blockId, overrides.locale),
    itemName: overrides.itemName ?? `${overrides.itemId}.json`,
    issues: [],
    block: block(blockId, overrides.locale, "draft target"),
    ...overrides,
  };
}

const error: QAIssue = { type: "tag-mismatch", severity: "error", message: "missing tag" };
const warning: QAIssue = { type: "length", severity: "warning", message: "too long" };

describe("isPendingReview", () => {
  it("counts a translated block with text as pending", () => {
    expect(isPendingReview(block("b1", "fr", "bonjour", "translated"), "fr")).toBe(true);
  });
  it("counts a draft block with text as pending", () => {
    expect(isPendingReview(block("b1", "fr", "bonjour", "draft"), "fr")).toBe(true);
  });
  it("excludes a reviewed block", () => {
    expect(isPendingReview(block("b1", "fr", "bonjour", "reviewed"), "fr")).toBe(false);
  });
  it("excludes a signed-off block", () => {
    expect(isPendingReview(block("b1", "fr", "bonjour", "signed-off"), "fr")).toBe(false);
  });
  it("excludes an empty target", () => {
    expect(isPendingReview(block("b1", "fr", "", "translated"), "fr")).toBe(false);
  });
  it("excludes a non-translatable block", () => {
    const b = { ...block("b1", "fr", "x", "translated"), translatable: false };
    expect(isPendingReview(b, "fr")).toBe(false);
  });
});

describe("entryCheckStatus", () => {
  it("is failing when an error finding is present", () => {
    expect(entryCheckStatus(entry({ itemId: "i1", locale: "fr", issues: [error, warning] }))).toBe(
      "failing",
    );
  });
  it("is clean with only warnings", () => {
    expect(entryCheckStatus(entry({ itemId: "i1", locale: "fr", issues: [warning] }))).toBe(
      "clean",
    );
    expect(entryHasErrors(entry({ itemId: "i1", locale: "fr", issues: [warning] }))).toBe(false);
  });
  it("isEntryClean only for clean", () => {
    expect(isEntryClean(entry({ itemId: "i1", locale: "fr" }))).toBe(true);
    expect(isEntryClean(entry({ itemId: "i1", locale: "fr", issues: [error] }))).toBe(false);
  });
});

describe("filtering", () => {
  const entries = [
    entry({ itemId: "i1", locale: "fr", block: block("a", "fr", "x"), issues: [error] }),
    entry({ itemId: "i1", locale: "de", block: block("b", "de", "x") }),
    entry({ itemId: "i2", locale: "fr", block: block("c", "fr", "x") }),
  ];
  it("matchesFilter respects every set field", () => {
    expect(matchesFilter(entries[0], { itemId: "i1" })).toBe(true);
    expect(matchesFilter(entries[0], { itemId: "i2" })).toBe(false);
    expect(matchesFilter(entries[0], { locale: "fr", check: "failing" })).toBe(true);
    expect(matchesFilter(entries[0], { locale: "fr", check: "clean" })).toBe(false);
  });
  it("filterEntries narrows and preserves order", () => {
    expect(filterEntries(entries, { locale: "fr" }).map((e) => e.block.id)).toEqual(["a", "c"]);
    expect(filterEntries(entries, { check: "clean" }).map((e) => e.block.id)).toEqual(["b", "c"]);
    expect(filterEntries(entries, {})).toHaveLength(3);
  });
});

describe("groupEntries", () => {
  const entries = [
    entry({ itemId: "i2", locale: "fr", block: block("a", "fr", "x") }),
    entry({ itemId: "i1", locale: "de", block: block("b", "de", "x"), issues: [error] }),
    entry({ itemId: "i1", locale: "fr", block: block("c", "fr", "x") }),
  ];
  it("groups by item, first-appearance order", () => {
    const groups = groupEntries(entries, "item");
    expect(groups.map((g) => g.key)).toEqual(["i2", "i1"]);
    expect(groups[1].entries).toHaveLength(2);
  });
  it("groups by locale", () => {
    const groups = groupEntries(entries, "locale");
    expect(groups.map((g) => g.key).sort()).toEqual(["de", "fr"]);
  });
  it("groups by check-status in severity order", () => {
    const groups = groupEntries(entries, "check");
    expect(groups.map((g) => g.key)).toEqual(["failing", "clean"]);
    expect(groups[0].label).toBe("Failing checks");
  });
});

describe("queueCounts + noFailingChecksCount", () => {
  const entries = [
    entry({ itemId: "i1", locale: "fr", block: block("a", "fr", "x"), issues: [error] }),
    entry({ itemId: "i1", locale: "de", block: block("b", "de", "x"), issues: [error] }),
    entry({ itemId: "i2", locale: "fr", block: block("c", "fr", "x") }),
    entry({ itemId: "i2", locale: "fr", block: block("d", "fr", "x") }),
  ];
  it("tallies by status, locale, and item", () => {
    const counts = queueCounts(entries);
    expect(counts.total).toBe(4);
    expect(counts.failing).toBe(2);
    expect(counts.clean).toBe(2);
    expect(counts.byLocale).toEqual({ fr: 3, de: 1 });
    expect(counts.byItem).toEqual({ i1: 2, i2: 2 });
  });
  it("noFailingChecksCount counts only entries without a failing check", () => {
    expect(noFailingChecksCount(entries)).toBe(2);
  });
});

describe("keyboard navigation", () => {
  it("nextIndex clamps without wrapping", () => {
    expect(nextIndex(5, 0, -1)).toBe(0);
    expect(nextIndex(5, 0, 1)).toBe(1);
    expect(nextIndex(5, 4, 1)).toBe(4);
    expect(nextIndex(0, 0, 1)).toBe(0);
  });
  it("indexAfterRemoval keeps focus in place, clamped", () => {
    // Remove middle of 5 → 4 remain, focus stays at 2.
    expect(indexAfterRemoval(5, 2)).toBe(2);
    // Remove last of 5 → focus clamps to new last (3).
    expect(indexAfterRemoval(5, 4)).toBe(3);
    // Remove the only entry → 0 (queue empty).
    expect(indexAfterRemoval(1, 0)).toBe(0);
  });
});
