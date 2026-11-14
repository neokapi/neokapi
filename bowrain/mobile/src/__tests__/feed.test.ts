import type { FeedEntry } from "@neokapi/bowrain-client";
import i18n from "i18next";

import "../lib/i18n";
import {
  actionOf,
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
          counts: { approved: 120, rejected: 14 },
        }),
        t,
        "en",
      ),
    ).toBe("approved 120 and rejected 14 in Norwegian Bokmål");
    expect(
      actionOf(entry({ kind: "change_set", state: "merged", name: "Checkout terms" }), t, "en"),
    ).toBe("merged Checkout terms");
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
