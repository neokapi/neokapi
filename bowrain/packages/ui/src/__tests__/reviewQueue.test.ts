import { describe, it, expect } from "vite-plus/test";
import type { BlockInfo, CheckIssue } from "../types/api";
import {
  entryKey,
  entryBlockers,
  entryVerdict,
  entryHasErrors,
  isPendingReview,
  isBelowVoiceBar,
  isEntryPassing,
  matchesFilter,
  filterEntries,
  groupEntries,
  queueCounts,
  passingCount,
  nextIndex,
  indexAfterRemoval,
  VERDICT_LABELS,
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
    // The server names every entry's collection, "" for an item in none.
    collectionId: "",
    // Unchecked terminology and no voice score: the neutral default, so a case
    // that says nothing about those bars is not silently asserting them.
    termCompliance: "",
    issues: [],
    block: block(blockId, overrides.locale, "draft target"),
    ...overrides,
  };
}

const error: CheckIssue = { type: "tag-mismatch", severity: "error", message: "missing tag" };
const warning: CheckIssue = { type: "length", severity: "warning", message: "too long" };

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

// The verdict mirrors the server's approve-passing predicate over the three
// bars the queue payload carries. It bucketed on check findings alone while
// they were the only evidence, so "no failing checks" over-counted by exactly
// the blocks the server then refused — a term violation or a below-bar voice
// score sat in the passing bucket.
describe("entryVerdict", () => {
  const cases: {
    name: string;
    entry: Partial<ReviewEntry>;
    verdict: "failing" | "passing";
    blockers: string[];
  }[] = [
    { name: "nothing against it", entry: {}, verdict: "passing", blockers: [] },
    {
      name: "an error finding",
      entry: { issues: [error, warning] },
      verdict: "failing",
      blockers: ["checks"],
    },
    { name: "only warnings", entry: { issues: [warning] }, verdict: "passing", blockers: [] },
    {
      name: "a terminology violation, checks clean",
      entry: { termCompliance: "violation" },
      verdict: "failing",
      blockers: ["terms"],
    },
    {
      name: "term-compliant",
      entry: { termCompliance: "compliant" },
      verdict: "passing",
      blockers: [],
    },
    {
      name: "a voice score below its profile's bar",
      entry: { voiceScore: 62, voiceBar: 90 },
      verdict: "failing",
      blockers: ["voice"],
    },
    {
      name: "a voice score at the bar",
      entry: { voiceScore: 90, voiceBar: 90 },
      verdict: "passing",
      blockers: [],
    },
    {
      name: "unscored — no voice bar is applied to it either",
      entry: {},
      verdict: "passing",
      blockers: [],
    },
    {
      name: "every bar missed at once, named in the server's order",
      entry: { issues: [error], termCompliance: "violation", voiceScore: 10, voiceBar: 80 },
      verdict: "failing",
      blockers: ["checks", "terms", "voice"],
    },
  ];
  for (const c of cases) {
    it(c.name, () => {
      const e = entry({ itemId: "i1", locale: "fr", ...c.entry });
      expect(entryVerdict(e)).toBe(c.verdict);
      expect(entryBlockers(e)).toEqual(c.blockers);
      expect(isEntryPassing(e)).toBe(c.verdict === "passing");
    });
  }

  it("an unchecked terminology verdict is not a violation", () => {
    // "" means no governance was active — nothing to violate, nothing claimed.
    expect(entryVerdict(entry({ itemId: "i1", locale: "fr", termCompliance: "" }))).toBe("passing");
  });

  it("entryHasErrors and isBelowVoiceBar answer their own axis only", () => {
    expect(entryHasErrors(entry({ itemId: "i1", locale: "fr", issues: [warning] }))).toBe(false);
    expect(isBelowVoiceBar(entry({ itemId: "i1", locale: "fr" }))).toBe(false);
    expect(
      isBelowVoiceBar(entry({ itemId: "i1", locale: "fr", voiceScore: 79, voiceBar: 80 })),
    ).toBe(true);
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
    expect(matchesFilter(entries[0], { locale: "fr", verdict: "failing" })).toBe(true);
    expect(matchesFilter(entries[0], { locale: "fr", verdict: "passing" })).toBe(false);
  });
  it("filterEntries narrows and preserves order", () => {
    expect(filterEntries(entries, { locale: "fr" }).map((e) => e.block.id)).toEqual(["a", "c"]);
    expect(filterEntries(entries, { verdict: "passing" }).map((e) => e.block.id)).toEqual([
      "b",
      "c",
    ]);
    expect(filterEntries(entries, {})).toHaveLength(3);
  });
  it("the verdict filter sees the terminology and voice bars, not checks alone", () => {
    const mixed = [
      entry({ itemId: "i1", locale: "fr", block: block("clean", "fr", "x") }),
      entry({
        itemId: "i1",
        locale: "fr",
        block: block("term", "fr", "x"),
        termCompliance: "violation",
      }),
      entry({
        itemId: "i1",
        locale: "fr",
        block: block("voice", "fr", "x"),
        voiceScore: 40,
        voiceBar: 80,
      }),
    ];
    expect(filterEntries(mixed, { verdict: "passing" }).map((e) => e.block.id)).toEqual(["clean"]);
    expect(filterEntries(mixed, { verdict: "failing" }).map((e) => e.block.id)).toEqual([
      "term",
      "voice",
    ]);
  });

  // How a collection carries from the project overview into review. The server
  // applies this scope when it pages the queue and names each entry's
  // collection on the payload, so the predicate here answers the same question
  // over a set a caller may have gathered from several scopes.
  //
  // Two states: "col-app" is a named collection and "" is an item in none. The
  // empty id is a scope of its own, which is why the filter is matched on
  // presence rather than truthiness.
  describe("by collection", () => {
    const scoped = [
      entry({ itemId: "i1", locale: "fr", collectionId: "col-app", block: block("a", "fr", "x") }),
      entry({ itemId: "i2", locale: "fr", collectionId: "col-docs", block: block("b", "fr", "x") }),
      entry({ itemId: "i3", locale: "fr", collectionId: "", block: block("c", "fr", "x") }),
    ];

    it("narrows to one collection", () => {
      expect(filterEntries(scoped, { collectionId: "col-app" }).map((e) => e.block.id)).toEqual([
        "a",
      ]);
    });

    it("selects the collection-less entries with the empty id, not with no filter", () => {
      expect(filterEntries(scoped, { collectionId: "" }).map((e) => e.block.id)).toEqual(["c"]);
      expect(filterEntries(scoped, {})).toHaveLength(3);
    });

    it("composes with the other filters", () => {
      expect(matchesFilter(scoped[0], { collectionId: "col-app", locale: "fr" })).toBe(true);
      expect(matchesFilter(scoped[0], { collectionId: "col-app", locale: "de" })).toBe(false);
    });
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
  it("groups by verdict in severity order", () => {
    const groups = groupEntries(entries, "verdict");
    expect(groups.map((g) => g.key)).toEqual(["failing", "passing"]);
    expect(groups[0].label).toBe(VERDICT_LABELS.failing);
  });
});

describe("queueCounts + passingCount", () => {
  const entries = [
    entry({ itemId: "i1", locale: "fr", block: block("a", "fr", "x"), issues: [error] }),
    entry({ itemId: "i1", locale: "de", block: block("b", "de", "x"), issues: [error] }),
    entry({ itemId: "i2", locale: "fr", block: block("c", "fr", "x") }),
    entry({ itemId: "i2", locale: "fr", block: block("d", "fr", "x") }),
  ];
  it("tallies by verdict, locale, and item", () => {
    const counts = queueCounts(entries);
    expect(counts.total).toBe(4);
    expect(counts.failing).toBe(2);
    expect(counts.passing).toBe(2);
    expect(counts.byLocale).toEqual({ fr: 3, de: 1 });
    expect(counts.byItem).toEqual({ i1: 2, i2: 2 });
  });
  it("passingCount counts the entries approve-passing will take", () => {
    expect(passingCount(entries)).toBe(2);
  });
  it("a term violation and a below-bar score leave the passing count, not just failing checks", () => {
    const withBars = [
      ...entries,
      entry({
        itemId: "i3",
        locale: "fr",
        block: block("e", "fr", "x"),
        termCompliance: "violation",
      }),
      entry({
        itemId: "i3",
        locale: "fr",
        block: block("f", "fr", "x"),
        voiceScore: 20,
        voiceBar: 80,
      }),
    ];
    // Six entries, four of which the server will refuse — the old
    // check-status-only count said four would pass.
    expect(passingCount(withBars)).toBe(2);
    expect(queueCounts(withBars).failing).toBe(4);
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
