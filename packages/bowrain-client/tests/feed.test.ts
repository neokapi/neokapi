import { describe, expect, it } from "vite-plus/test";

import { groupFeedByDay, isFeedSourceEvent, mainCount, type FeedEntry } from "../src/index.ts";

function entry(kind: string, at: Date, counts: Record<string, number> = {}): FeedEntry {
  return {
    id: `${kind}-${at.getTime()}`,
    kind,
    at: at.toISOString(),
    actor: { id: "u1", kind: "person" },
    counts,
    link: { screen: "stream" },
  };
}

describe("feed", () => {
  it("groups entries by local day, newest first", () => {
    const now = new Date(2026, 10, 14, 15, 0);
    const days = groupFeedByDay(
      [
        entry("push", new Date(2026, 10, 14, 9, 0)),
        entry("push", new Date(2026, 10, 14, 0, 5)),
        entry("convergence_run", new Date(2026, 10, 13, 23, 55)),
        entry("change_set", new Date(2026, 10, 10, 12, 0)),
      ],
      now,
    );
    expect(days.map((d) => [d.key, d.daysAgo, d.entries.length])).toEqual([
      ["2026-11-14", 0, 2],
      ["2026-11-13", 1, 1],
      ["2026-11-10", 4, 1],
    ]);
  });

  it("leads each kind with the count that matters", () => {
    const at = new Date();
    expect(mainCount(entry("push", at, { files: 3, blocks: 40 }))).toEqual({
      key: "blocks",
      n: 40,
    });
    expect(
      mainCount(entry("review_summary", at, { approved: 4, rejected: 1, bulk_approved: 7 })),
    ).toEqual({
      key: "decisions",
      n: 12,
    });
    expect(mainCount(entry("context_change", at, { term_rule: 6, add: 6 }))).toEqual({
      key: "term_rule",
      n: 6,
    });
    expect(mainCount(entry("context_change", at, { term_rule: 2, note: 1, keep: 3 }))).toEqual({
      key: "items",
      n: 3,
    });
    expect(mainCount(entry("recipe_change", at, { targets_added: 2 }))).toEqual({
      key: "targets_added",
      n: 2,
    });
    expect(mainCount(entry("change_set", at))).toBeUndefined();
  });

  it("knows which relayed events file feed entries", () => {
    expect(isFeedSourceEvent("connector.push.completed")).toBe(true);
    expect(isFeedSourceEvent("changeset.merged")).toBe(true);
    expect(isFeedSourceEvent("block.updated")).toBe(false);
  });
});
