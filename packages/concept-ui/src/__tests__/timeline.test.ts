import { describe, expect, it } from "vitest";
import { buildTimeline, dayKey, sortTimeline, synthesizeTimeline } from "../timeline";
import type { Concept, TimelineEvent } from "../types";

const ev = (at: string, summary: string): TimelineEvent => ({ kind: "revision", at, summary });

describe("dayKey", () => {
  it("slices ISO dates and flags unparseable input", () => {
    expect(dayKey("2026-03-05T14:20:00Z")).toBe("2026-03-05");
    expect(dayKey("not-a-date")).toBe("undated");
  });
});

describe("sortTimeline", () => {
  const events = [ev("2025-01-01T00:00:00Z", "a"), ev("2026-01-01T00:00:00Z", "b")];

  it("defaults to newest first", () => {
    expect(sortTimeline(events).map((e) => e.summary)).toEqual(["b", "a"]);
  });

  it("can tell the story from the beginning", () => {
    expect(sortTimeline(events, "asc").map((e) => e.summary)).toEqual(["a", "b"]);
  });

  it("is stable for equal timestamps", () => {
    const same = [ev("2026-01-01T00:00:00Z", "x"), ev("2026-01-01T00:00:00Z", "y")];
    expect(sortTimeline(same, "asc").map((e) => e.summary)).toEqual(["x", "y"]);
  });
});

describe("buildTimeline", () => {
  it("buckets events into per-day groups in the chosen order", () => {
    const days = buildTimeline(
      [
        ev("2026-03-05T09:00:00Z", "morning"),
        ev("2026-03-05T17:00:00Z", "evening"),
        ev("2026-01-10T12:00:00Z", "january"),
      ],
      "desc",
    );
    expect(days.map((d) => d.key)).toEqual(["2026-03-05", "2026-01-10"]);
    expect(days[0].events.map((e) => e.summary)).toEqual(["evening", "morning"]);
  });
});

describe("synthesizeTimeline", () => {
  const concept: Concept = {
    id: "c",
    createdAt: "2025-09-01T09:00:00Z",
    updatedAt: "2025-10-01T09:00:00Z",
    terms: [
      {
        text: "Bon",
        locale: "fr-FR",
        status: "admitted",
        validity: { validFrom: "2025-11-01T00:00:00Z" },
      },
      { text: "Checkout", locale: "en-US", status: "preferred" },
    ],
  };

  it("emits create, update, and per-window events", () => {
    const events = synthesizeTimeline(concept);
    expect(events.map((e) => e.kind)).toEqual(["create", "revision", "status"]);
    expect(events[2].summary).toContain("Bon");
  });

  it("omits the update event when it equals creation", () => {
    const events = synthesizeTimeline({ ...concept, updatedAt: concept.createdAt });
    expect(events.map((e) => e.kind)).toEqual(["create", "status"]);
  });

  it("returns nothing when the concept tracks no timestamps", () => {
    expect(synthesizeTimeline({ id: "x", terms: [] })).toEqual([]);
  });
});
