import { describe, expect, it } from "vite-plus/test";

import {
  ageOf,
  foldFeed,
  groupFeedByDay,
  initialsOf,
  isFeedSourceEvent,
  mainCount,
  paletteIndex,
  type FeedEntry,
} from "../src/index.ts";

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

  it("drops quiet runs and folds one actor's runs into one card", () => {
    const t0 = new Date(2026, 10, 14, 12, 0);
    const run = (minutesAgo: number, produced: number, state = "converged") => ({
      ...entry("convergence_run", new Date(t0.getTime() - minutesAgo * 60_000), { produced }),
      id: `r${minutesAgo}`,
      state,
      actor: { id: "system", kind: "tool" as const, name: "convergence" },
    });
    const cards = foldFeed([
      run(0, 40),
      run(5, 0),
      run(10, 12),
      run(20, 0, "failed"),
      entry("push", new Date(t0.getTime() - 25 * 60_000)),
      run(90, 3),
    ]);
    expect(cards.map((c) => c.entries.map((e) => e.id))).toEqual([
      ["r0", "r10", "r20"],
      [`push-${t0.getTime() - 25 * 60_000}`],
      ["r90"],
    ]);
  });

  it("gives an actor initials and a stable colour", () => {
    expect(initialsOf("Ada Lindqvist")).toBe("AL");
    expect(initialsOf("claude-code")).toBe("CC");
    expect(initialsOf("asgeir")).toBe("AS");
    expect(paletteIndex("u-ada", 8)).toBe(paletteIndex("u-ada", 8));
  });

  it("states an age as now, minutes, hours, yesterday or a date", () => {
    const now = new Date(2026, 10, 14, 15, 0);
    const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();
    expect(ageOf(ago(10_000), now)).toEqual({ unit: "now" });
    expect(ageOf(ago(5 * 60_000), now)).toEqual({ unit: "minutes", n: 5 });
    expect(ageOf(ago(2 * 3_600_000), now)).toEqual({ unit: "hours", n: 2 });
    expect(ageOf(ago(20 * 3_600_000), now)).toEqual({ unit: "yesterday" });
    expect(ageOf(ago(4 * 86_400_000), now).unit).toBe("date");
  });
});
