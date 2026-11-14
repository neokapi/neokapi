import type { FeedEntry } from "@neokapi/bowrain-client";
import i18n from "i18next";

import "../lib/i18n";
import { dayLabel, deckLocaleOf, entryMeta, entryTitle } from "../lib/feed";
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
