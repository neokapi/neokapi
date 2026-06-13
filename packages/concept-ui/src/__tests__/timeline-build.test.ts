import { describe, expect, it } from "vitest";
import {
  TIMELINE_KIND_META,
  buildDisplayTimeline,
  resolveTimelineEvents,
  synthesizeCoreTimeline,
  timelineKindMeta,
} from "../timeline-build";
import type { Concept, Relation, TimelineEvent } from "../types";

const baseConcept: Concept = {
  id: "checkout",
  createdAt: "2025-09-01T09:00:00Z",
  updatedAt: "2026-03-05T14:20:00Z",
  terms: [
    {
      text: "Kasse",
      locale: "de-DE",
      status: "preferred",
      validity: { validFrom: "2025-10-01T00:00:00Z", tags: { market: "dach" } },
    },
    {
      text: "Validation de commande",
      locale: "fr-FR",
      status: "deprecated",
      validity: { validTo: "2025-11-03T00:00:00Z", tags: { market: "france" } },
    },
    { text: "Checkout", locale: "en-US", status: "preferred" }, // no window → no event
  ],
};

describe("timelineKindMeta", () => {
  it("maps each kind to a tone and falls back safely", () => {
    expect(timelineKindMeta("create").tone).toBe("genesis");
    expect(timelineKindMeta("relation").tone).toBe("relation");
    expect(TIMELINE_KIND_META.comment.tone).toBe("discussion");
    // Unknown kind (defensive cast) still resolves.
    expect(timelineKindMeta("mystery" as never).tone).toBe("edit");
  });
});

describe("synthesizeCoreTimeline", () => {
  it("derives create, update, and term valid/retire events from windows", () => {
    const events = synthesizeCoreTimeline(baseConcept);
    const kinds = events.map((e) => e.kind);
    expect(kinds).toContain("create");
    expect(kinds).toContain("revision");
    // Two windowed terms → one valid-from + one valid-to.
    const status = events.filter((e) => e.kind === "status");
    expect(status.map((e) => e.summary)).toEqual([
      "Kasse valid in de-DE · dach",
      "Validation de commande retired in fr-FR · france",
    ]);
  });

  it("derives relation gained/lost from relation validity and labels the neighbour", () => {
    const relations: Relation[] = [
      {
        id: "r1",
        sourceId: "checkout",
        targetId: "promo",
        type: "USE_INSTEAD",
        validity: { validFrom: "2025-12-01T00:00:00Z", validTo: "2026-02-01T00:00:00Z" },
      },
      // Untouching relation is ignored.
      {
        id: "r2",
        sourceId: "x",
        targetId: "y",
        type: "RELATED",
        validity: { validFrom: "2025-12-01T00:00:00Z" },
      },
    ];
    const events = synthesizeCoreTimeline(baseConcept, relations, {
      labelFor: (id) => (id === "promo" ? "Promo code" : id),
    });
    const rel = events.filter((e) => e.kind === "relation").map((e) => e.summary);
    expect(rel).toEqual(["Linked use instead Promo code", "Unlinked use instead Promo code"]);
  });

  it("returns nothing for a concept with no timestamps or windows", () => {
    expect(synthesizeCoreTimeline({ id: "x", terms: [] })).toEqual([]);
  });
});

describe("resolveTimelineEvents", () => {
  it("prefers a non-empty remote revision log", () => {
    const remote: TimelineEvent[] = [
      { kind: "changeset", at: "2026-01-01T00:00:00Z", summary: "Shipped" },
    ];
    const events = resolveTimelineEvents(baseConcept, { remote });
    expect(events).toBe(remote);
  });

  it("falls back to the synthesised core when remote is empty/absent", () => {
    const events = resolveTimelineEvents(baseConcept, { remote: [] });
    expect(events.some((e) => e.kind === "create")).toBe(true);
  });
});

describe("buildDisplayTimeline", () => {
  const events: TimelineEvent[] = [
    { kind: "create", at: "2025-09-01T09:00:00Z", summary: "Created" },
    { kind: "status", at: "2025-09-01T17:00:00Z", summary: "Same day, later" },
    { kind: "comment", at: "2026-03-05T14:20:00Z", summary: "Discussed" },
  ];

  it("buckets by day newest-first and attaches tone + stable ids", () => {
    const days = buildDisplayTimeline(events, "desc");
    expect(days.map((d) => d.key)).toEqual(["2026-03-05", "2025-09-01"]);
    expect(days[0].events[0].tone).toBe("discussion");
    // Same-day group keeps both events, latest first.
    expect(days[1].events.map((e) => e.summary)).toEqual(["Same day, later", "Created"]);
    expect(days[1].events.map((e) => e.tone)).toEqual(["status", "genesis"]);
  });

  it("produces unique keys across the whole timeline", () => {
    const days = buildDisplayTimeline([...events, ...events], "asc");
    const ids = days.flatMap((d) => d.events.map((e) => e.id));
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("honours an explicit event id", () => {
    const days = buildDisplayTimeline([
      { id: "fixed", kind: "create", at: "2025-09-01T09:00:00Z", summary: "x" },
    ]);
    expect(days[0].events[0].id).toBe("fixed");
  });
});
