import { describe, it, expect } from "vite-plus/test";
import type { TermInfo } from "../../types/api";
import type {
  ConceptRelation,
  ConceptStoryEntry,
  Market,
  RelationType,
} from "../../types/brand-graph";
import {
  deriveStoryEntry,
  dayKey,
  buildStoryTimeline,
  termsByMarket,
  groupRelations,
} from "./story-timeline";

describe("deriveStoryEntry", () => {
  it("marks the first revision as a creation", () => {
    const e = deriveStoryEntry(
      {
        kind: "revision",
        at: "2026-06-01T00:00:00Z",
        actor: "alex",
        ref: "1",
        summary: "Created concept",
      },
      0,
    );
    expect(e.tone).toBe("create");
    expect(e.title).toBe("Created concept");
  });

  it("treats a 'created' summary as a creation even without ref 1", () => {
    const e = deriveStoryEntry(
      { kind: "revision", at: "2026-06-01T00:00:00Z", summary: "Created with 3 terms" },
      0,
    );
    expect(e.tone).toBe("create");
  });

  it("treats later revisions as revisions", () => {
    const e = deriveStoryEntry(
      {
        kind: "revision",
        at: "2026-06-02T00:00:00Z",
        ref: "2",
        summary: "Status preferred → forbidden",
      },
      0,
    );
    expect(e.tone).toBe("revision");
    expect(e.title).toMatch(/forbidden/);
  });

  it("puts the comment body in the detail and a headline in the title", () => {
    const e = deriveStoryEntry(
      {
        kind: "comment",
        at: "2026-06-03T00:00:00Z",
        actor: "sam",
        summary: "Should this be preferred?",
      },
      0,
    );
    expect(e.tone).toBe("comment");
    expect(e.title).toBe("sam commented");
    expect(e.detail).toBe("Should this be preferred?");
  });

  it("derives observation and changeset tones", () => {
    expect(deriveStoryEntry({ kind: "observation", at: "x" }, 0).tone).toBe("observation");
    expect(deriveStoryEntry({ kind: "changeset", at: "x" }, 0).tone).toBe("changeset");
  });

  it("gives every entry a unique id", () => {
    const a = deriveStoryEntry({ kind: "comment", at: "2026-06-03T00:00:00Z" }, 0);
    const b = deriveStoryEntry({ kind: "comment", at: "2026-06-03T00:00:00Z" }, 1);
    expect(a.id).not.toBe(b.id);
  });
});

describe("dayKey", () => {
  it("slices the date from an ISO timestamp", () => {
    expect(dayKey("2026-06-13T10:00:00Z")).toBe("2026-06-13");
  });
  it("returns 'unknown' for an unparseable input", () => {
    expect(dayKey("not-a-date")).toBe("unknown");
  });
});

describe("buildStoryTimeline", () => {
  const entries: ConceptStoryEntry[] = [
    { kind: "revision", at: "2026-06-01T09:00:00Z", actor: "alex", ref: "1", summary: "Created" },
    { kind: "observation", at: "2026-06-01T15:00:00Z", actor: "alex", summary: "Saw rival" },
    { kind: "comment", at: "2026-06-03T11:00:00Z", actor: "sam", summary: "Discuss" },
  ];

  it("groups entries by calendar day", () => {
    const groups = buildStoryTimeline(entries, "asc");
    expect(groups.map((g) => g.key)).toEqual(["2026-06-01", "2026-06-03"]);
    expect(groups[0].entries).toHaveLength(2);
    expect(groups[1].entries).toHaveLength(1);
  });

  it("orders ascending by default (oldest day first)", () => {
    const groups = buildStoryTimeline(entries, "asc");
    expect(groups[0].entries[0].title).toBe("Created");
    expect(groups[0].entries[1].tone).toBe("observation");
  });

  it("orders descending when asked (newest first)", () => {
    const groups = buildStoryTimeline(entries, "desc");
    expect(groups[0].key).toBe("2026-06-03");
    expect(groups[1].entries[0].tone).toBe("observation");
    expect(groups[1].entries[1].title).toBe("Created");
  });

  it("handles an empty story", () => {
    expect(buildStoryTimeline([])).toEqual([]);
  });
});

describe("termsByMarket", () => {
  const markets: Market[] = [
    {
      id: "m-dach",
      workspace_id: "ws-1",
      name: "dach",
      locales: ["de-DE", "de-AT"],
      created_at: "",
      updated_at: "",
    },
    {
      id: "m-us",
      workspace_id: "ws-1",
      name: "us",
      locales: ["en-US"],
      created_at: "",
      updated_at: "",
    },
  ];
  const terms: TermInfo[] = [
    { text: "Checkout", locale: "en-US", status: "preferred" },
    { text: "Kasse", locale: "de-DE", status: "approved" },
    { text: "Caisse", locale: "fr-FR", status: "proposed" },
  ];

  it("groups terms under the markets that cover their locale", () => {
    const groups = termsByMarket(terms, markets);
    const dach = groups.find((g) => g.name === "dach");
    expect(dach?.locales.map((l) => l.locale)).toEqual(["de-DE"]);
    const us = groups.find((g) => g.name === "us");
    expect(us?.locales[0].terms[0].text).toBe("Checkout");
  });

  it("collects uncovered locales into an 'Other locales' bucket", () => {
    const groups = termsByMarket(terms, markets);
    const other = groups.find((g) => g.market === null);
    expect(other?.name).toBe("Other locales");
    expect(other?.locales.map((l) => l.locale)).toEqual(["fr-FR"]);
  });

  it("with no markets, everything lands in Other locales", () => {
    const groups = termsByMarket(terms, []);
    expect(groups).toHaveLength(1);
    expect(groups[0].market).toBeNull();
    expect(groups[0].locales).toHaveLength(3);
  });
});

describe("groupRelations", () => {
  function rel(id: string, source: string, target: string, type: RelationType): ConceptRelation {
    return { id, source_id: source, target_id: target, relation_type: type, created_at: "" };
  }
  const relations: ConceptRelation[] = [
    rel("r1", "c-a", "c-b", "RELATED"),
    rel("r2", "c-c", "c-a", "COMPETITOR"),
    rel("r3", "c-a", "c-d", "HAS_PART"),
  ];

  it("groups by type in canonical order and resolves the far end", () => {
    const groups = groupRelations(relations, "c-a");
    expect(groups.map((g) => g.type)).toEqual(["HAS_PART", "RELATED", "COMPETITOR"]);
    const related = groups.find((g) => g.type === "RELATED")!;
    expect(related.items[0]).toMatchObject({ otherId: "c-b", outgoing: true });
    const competitor = groups.find((g) => g.type === "COMPETITOR")!;
    expect(competitor.items[0]).toMatchObject({ otherId: "c-c", outgoing: false });
  });

  it("returns no groups for a concept with no relations", () => {
    expect(groupRelations([], "c-a")).toEqual([]);
  });
});
