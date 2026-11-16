import type { FeedEntry } from "@neokapi/bowrain-client";
import i18n from "i18next";

import "../lib/i18n";
import {
  actionOf,
  actorName,
  ageLabel,
  dayLabel,
  deckLocaleOf,
  entryMeta,
  entryTitle,
  foldedActionOf,
} from "../lib/feed";
import { changeTypeOf, createSseParser } from "../lib/sse";

const t = i18n.getFixedT("en");

function entry(over: Partial<FeedEntry>): FeedEntry {
  return {
    id: "a1",
    kind: "push",
    at: "2026-11-14T09:10:00Z",
    actor: { id: "u1", kind: "person", name: "Ada" },
    counts: {},
    link: { screen: "stream" },
    ...over,
  };
}

describe("feed card sentences", () => {
  it("says what the actor did, with language names", () => {
    expect(
      actionOf(
        entry({
          kind: "recipe_change",
          diff: { targets: { before: ["nb"], after: ["nb", "ja", "ko"] } },
          counts: { targets_added: 2 },
        }),
        t,
        "en",
      ),
    ).toBe("added Japanese and Korean");
    expect(
      actionOf(
        entry({
          kind: "context_change",
          actor: { id: "u1", kind: "agent", name: "claude-code" },
          counts: { term_rule: 6, add: 6 },
        }),
        t,
        "en",
      ),
    ).toBe("proposed 6 term rules");
    expect(
      actionOf(
        entry({
          kind: "convergence_run",
          state: "converged",
          locales: ["nb", "de", "fr"],
          counts: { produced: 40 },
        }),
        t,
        "en",
      ),
    ).toBe("drafted 40 translations in 3 languages");
    expect(
      actionOf(
        entry({
          kind: "review_summary",
          locales: ["nb"],
          counts: { approved: 120, bulk_approved: 1000, rejected: 14 },
        }),
        t,
        "en",
      ),
    ).toBe("approved 1,120 and rejected 14 in Norwegian Bokmål");
    expect(
      actionOf(entry({ kind: "change_set", state: "merged", name: "Checkout terms" }), t, "en"),
    ).toBe("merged Checkout terms");
  });

  it("words a parked run from why it stopped", () => {
    const parked = (stall_reason?: string) =>
      actionOf(
        entry({
          kind: "convergence_run",
          state: "parked",
          stall_reason,
          locales: ["nb"],
          counts: { produced: 34231 },
        }),
        t,
        "en",
      );
    expect(parked()).toBe("drafted 34,231 translations in Norwegian Bokmål for review");
    expect(parked("no_progress")).toBe(
      "drafted 34,231 translations in Norwegian Bokmål for review",
    );
    expect(parked("needs_credits")).toBe("stopped because the workspace is out of AI credits");
    expect(parked("checks_failing")).toBe(
      "drafted 34,231 translations in Norwegian Bokmål, and some fail checks",
    );
    expect(parked("something_new")).toBe("stopped before finishing");
  });

  it("names the convergence loop as Bowrain", () => {
    expect(actorName(entry({ actor: { id: "", kind: "tool", name: "convergence" } }), t)).toBe(
      "Bowrain",
    );
  });

  it("names a tool by its name in words, apart from the member who issued it", () => {
    const tool = entry({
      actor: { id: "u1", kind: "tool", name: "dogfood-sync" },
      on_behalf_of: { id: "u1", name: "Asgeir" },
    });
    expect(actorName(tool, t)).toBe("Dogfood sync");
  });

  it("counts collections past two instead of naming each", () => {
    const added = (n: number) =>
      actionOf(
        entry({
          kind: "recipe_change",
          diff: {
            collections: Array.from({ length: n }, (_, i) => ({
              name: `c${i}`,
              change: "added",
              after: {},
            })),
          },
        }),
        t,
        "en",
      );
    expect(added(2)).toBe("added the c0 collection and added the c1 collection");
    expect(added(20)).toBe("added 20 collections");
  });

  it("says how many times a folded card's actor acted", () => {
    const run = entry({ kind: "convergence_run", counts: { produced: 3 } });
    expect(foldedActionOf({ key: "k", entries: [run, run, run] }, t, "en")).toBe("ran 3 times");
  });

  it("states a compact age", () => {
    const now = new Date(2026, 10, 14, 15, 0);
    expect(ageLabel(new Date(now.getTime() - 2 * 3_600_000).toISOString(), t, "en", now)).toBe(
      "2h",
    );
  });
});

describe("feed rows", () => {
  it("names the kind and its object on the primary line", () => {
    expect(entryTitle(entry({ stream: "main" }), t, "en")).toBe("Push · main");
    expect(entryTitle(entry({ kind: "convergence_run", state: "converged" }), t, "en")).toBe(
      "Run · converged",
    );
    expect(entryTitle(entry({ kind: "review_summary", locales: ["fr"] }), t, "en")).toBe(
      "Review · French",
    );
    expect(
      entryTitle(
        entry({ kind: "coverage_milestone", locales: ["fr"], counts: { percent: 90 } }),
        t,
        "en",
      ),
    ).toBe("French · 90%");
  });

  it("keeps the meta line to the actor, one count and the time", () => {
    const meta = entryMeta(entry({ counts: { files: 3, blocks: 40 } }), t, "en");
    const parts = meta.split(" · ");
    expect(parts).toHaveLength(3);
    expect(parts.slice(0, 2)).toEqual(["Ada", "40 blocks"]);
    expect(
      entryMeta(entry({ kind: "context_change", counts: { term_rule: 1, add: 1 } }), t, "en"),
    ).toContain("1 term rule");
  });

  it("opens the review deck for a review summary and a milestone", () => {
    expect(deckLocaleOf(entry({ link: { screen: "review_deck", locale: "nb" } }))).toBe("nb");
    expect(deckLocaleOf(entry({ link: { screen: "run", id: "r1" } }))).toBeUndefined();
  });

  it("heads days as Today, Yesterday, then the date", () => {
    expect(dayLabel(0, new Date(), t, "en")).toBe("Today");
    expect(dayLabel(1, new Date(), t, "en")).toBe("Yesterday");
    expect(dayLabel(5, new Date(2026, 10, 9), t, "en")).toContain("November");
  });
});

describe("event stream", () => {
  it("reads change events across chunk boundaries and skips comments", () => {
    const parse = createSseParser();
    expect(parse(': connected\n\nevent: change\ndata: {"type":"rec')).toEqual([]);
    const out = parse('ipe.changed"}\n\n: heartbeat\n\nevent: other\ndata: x\n\n');
    expect(out).toEqual(['{"type":"recipe.changed"}']);
    expect(changeTypeOf(out[0])).toBe("recipe.changed");
    expect(changeTypeOf("not json")).toBe("");
  });
});
