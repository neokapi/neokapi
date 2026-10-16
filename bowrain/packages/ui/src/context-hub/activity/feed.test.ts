import { describe, it, expect } from "vite-plus/test";
import type { ActivityInfo, ConceptInfo } from "../../types/api";
import {
  ACTIVITY_CATEGORIES,
  CATEGORY_TYPE_PREFIXES,
  SESSION_GAP_MS,
  categoryCounts,
  categoryForType,
  conceptDisplayName,
  dayLabel,
  decisionsOf,
  groupByDay,
  groupSessionsByDay,
  sessionsOf,
  toFeedItem,
  typePrefixesFor,
} from "./feed";

function activity(partial: Partial<ActivityInfo>): ActivityInfo {
  return {
    id: "a-1",
    workspace_id: "acme",
    actor_id: "sam",
    actor_name: "Sam Okafor",
    type: "concept.updated",
    summary: "updated a concept",
    created_at: "2026-06-13T10:00:00Z",
    ...partial,
  };
}

describe("typePrefixesFor", () => {
  it("expresses a category as the type families the server matches by prefix", () => {
    expect(typePrefixesFor(["concept"])).toEqual(["concept."]);
    expect(typePrefixesFor(["experiment"])).toEqual(["changeset.", "review.", "pilot."]);
  });

  it("orders by the canonical category order and deduplicates", () => {
    expect(typePrefixesFor(["comment", "experiment"])).toEqual([
      "changeset.",
      "review.",
      "pilot.",
      "comment.",
    ]);
  });

  it("asks for nothing when no category is selected", () => {
    expect(typePrefixesFor([])).toEqual([]);
  });
});

describe("categoryForType", () => {
  it("claims each recorded knowledge family", () => {
    expect(categoryForType("concept.created")).toBe("concept");
    expect(categoryForType("concept.term.status_changed")).toBe("concept");
    expect(categoryForType("changeset.abandoned")).toBe("experiment");
    expect(categoryForType("review.decided")).toBe("experiment");
    expect(categoryForType("pilot.started")).toBe("experiment");
    expect(categoryForType("observation.added")).toBe("observation");
    expect(categoryForType("comment.added")).toBe("comment");
  });

  it("claims nothing outside the brand families", () => {
    expect(categoryForType("item.pushed")).toBeNull();
    expect(categoryForType("flow.completed")).toBeNull();
  });

  it("keeps the families disjoint, so no type lands in two categories", () => {
    const prefixes = ACTIVITY_CATEGORIES.flatMap((c) => CATEGORY_TYPE_PREFIXES[c]);
    for (const a of prefixes) {
      const overlapping = prefixes.filter((b) => a !== b && (a.startsWith(b) || b.startsWith(a)));
      expect(overlapping).toEqual([]);
    }
  });
});

describe("toFeedItem", () => {
  it("names the change-set when it can, keeping the server sentence as detail", () => {
    const item = toFeedItem(
      activity({
        type: "review.assigned",
        entity_type: "changeset",
        entity_id: "cs-1",
        summary: "submitted a change for review",
      }),
      { "cs-1": "Prefer ‘Paiement’ for fr-FR" },
    );
    expect(item.title).toBe("Prefer ‘Paiement’ for fr-FR");
    expect(item.detail).toBe("submitted a change for review");
    expect(item.changesetId).toBe("cs-1");
    expect(item.category).toBe("experiment");
  });

  it("falls back to the sentence rather than showing an id", () => {
    const item = toFeedItem(
      activity({ type: "concept.created", entity_type: "concept", entity_id: "c-9" }),
    );
    expect(item.title).toBe("updated a concept");
    expect(item.detail).toBeUndefined();
    expect(item.title).not.toContain("c-9");
    expect(item.conceptId).toBe("c-9");
  });

  it("reads the subject out of the payload when the entity ref is absent", () => {
    const item = toFeedItem(
      activity({ type: "pilot.started", data: { changeset_id: "cs-2", stream: "main" } }),
      { "cs-2": "Ban competitor name" },
    );
    expect(item.changesetId).toBe("cs-2");
    expect(item.title).toBe("Ban competitor name");
  });

  it("carries the actor and instant through unchanged", () => {
    const item = toFeedItem(activity({ id: "a-7", created_at: "2026-06-01T08:00:00Z" }));
    expect(item.id).toBe("a-7");
    expect(item.at).toBe("2026-06-01T08:00:00Z");
    expect(item.actor).toBe("sam");
  });
});

describe("conceptDisplayName", () => {
  const concept = (terms: ConceptInfo["terms"]): ConceptInfo => ({
    id: "c-1",
    domain: "commerce",
    definition: "",
    terms,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
  });

  it("prefers a preferred term, then English, then the first", () => {
    expect(
      conceptDisplayName(
        concept([
          { text: "Kasse", locale: "de-DE", status: "approved" },
          { text: "Checkout", locale: "en-US", status: "preferred" },
        ]),
      ),
    ).toBe("Checkout");
    expect(
      conceptDisplayName(
        concept([
          { text: "Kasse", locale: "de-DE", status: "approved" },
          { text: "Cart", locale: "en-GB", status: "admitted" },
        ]),
      ),
    ).toBe("Cart");
    expect(conceptDisplayName(concept([]))).toBe("commerce");
  });
});

describe("groupByDay", () => {
  const now = new Date("2026-06-13T12:00:00Z").getTime();

  it("labels today, yesterday, and older days", () => {
    expect(dayLabel("2026-06-13T09:00:00Z", now)).toBe("Today");
    expect(dayLabel("2026-06-12T09:00:00Z", now)).toBe("Yesterday");
    expect(dayLabel("2026-06-01T09:00:00Z", now)).toContain("2026");
    expect(dayLabel("not-a-date", now)).toBe("·");
  });

  it("buckets a sorted page into days, preserving order", () => {
    const items = [
      toFeedItem(activity({ id: "a", created_at: "2026-06-13T11:00:00Z" })),
      toFeedItem(activity({ id: "b", created_at: "2026-06-13T09:00:00Z" })),
      toFeedItem(activity({ id: "c", created_at: "2026-06-12T09:00:00Z" })),
    ];
    const groups = groupByDay(items, now);
    expect(groups.map((g) => g.label)).toEqual(["Today", "Yesterday"]);
    expect(groups[0].items.map((i) => i.id)).toEqual(["a", "b"]);
    expect(groups[1].items.map((i) => i.id)).toEqual(["c"]);
  });
});

describe("sessionsOf", () => {
  const at = (minutes: number) =>
    new Date(Date.UTC(2026, 5, 13, 12, 0) - minutes * 60_000).toISOString();
  const item = (id: string, actor: string, minutesAgo: number, type = "concept.updated") =>
    toFeedItem(
      activity({ id, actor_id: actor, actor_name: actor, type, created_at: at(minutesAgo) }),
    );

  it("keeps one actor's operations close together in one session, newest first", () => {
    const sessions = sessionsOf([item("a", "sam", 0), item("b", "sam", 10), item("c", "sam", 25)]);
    expect(sessions).toHaveLength(1);
    expect(sessions[0].id).toBe("a");
    expect(sessions[0].items.map((i) => i.id)).toEqual(["a", "b", "c"]);
    expect(sessions[0].last).toBe(at(0));
    expect(sessions[0].first).toBe(at(25));
  });

  it("starts a new session after a pause longer than the gap", () => {
    const gap = SESSION_GAP_MS / 60_000;
    const sessions = sessionsOf([item("a", "sam", 0), item("b", "sam", gap + 1)]);
    expect(sessions.map((s) => s.id)).toEqual(["a", "b"]);
  });

  it("measures the gap from the session's oldest operation, so a long steady run stays whole", () => {
    const sessions = sessionsOf([
      item("a", "sam", 0),
      item("b", "sam", 20),
      item("c", "sam", 40),
      item("d", "sam", 60),
    ]);
    expect(sessions).toHaveLength(1);
  });

  it("splits when another person acts in between", () => {
    const sessions = sessionsOf([item("a", "sam", 0), item("b", "alex", 2), item("c", "sam", 4)]);
    expect(sessions.map((s) => s.actor)).toEqual(["sam", "alex", "sam"]);
  });

  it("names the decisions a session holds and counts it by category", () => {
    const [session] = sessionsOf([
      item("a", "alex", 0, "review.decided"),
      item("b", "alex", 1, "concept.term.status_changed"),
      item("c", "alex", 2, "review.decided"),
      item("d", "alex", 3, "comment.added"),
    ]);
    expect(decisionsOf(session).map((i) => i.id)).toEqual(["a", "c"]);
    expect(categoryCounts(session)).toEqual([
      ["experiment", 2],
      ["concept", 1],
      ["comment", 1],
    ]);
  });

  it("returns nothing for an empty feed", () => {
    expect(sessionsOf([])).toEqual([]);
  });
});

describe("groupSessionsByDay", () => {
  it("buckets sessions by the day of their newest operation", () => {
    const now = new Date(2026, 5, 13, 18, 0).getTime();
    const sessions = sessionsOf([
      toFeedItem(
        activity({ id: "a", actor_id: "sam", created_at: new Date(2026, 5, 13, 9).toISOString() }),
      ),
      toFeedItem(
        activity({ id: "b", actor_id: "alex", created_at: new Date(2026, 5, 13, 8).toISOString() }),
      ),
      toFeedItem(
        activity({ id: "c", actor_id: "sam", created_at: new Date(2026, 5, 12, 9).toISOString() }),
      ),
    ]);
    const groups = groupSessionsByDay(sessions, now);
    expect(groups.map((g) => [g.label, g.sessions.map((s) => s.id)])).toEqual([
      ["Today", ["a", "b"]],
      ["Yesterday", ["c"]],
    ]);
  });
});
