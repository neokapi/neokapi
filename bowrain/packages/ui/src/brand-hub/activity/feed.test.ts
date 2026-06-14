import { describe, it, expect } from "vite-plus/test";
import type { ConceptInfo } from "../../types/api";
import type { ChangeSet, ChangeSetDetail } from "../../types/brand-graph";
import { buildFeed, conceptDisplayName, dayLabel, groupByDay, type StorySource } from "./feed";

function changeset(id: string, partial: Partial<ChangeSet>): ChangeSet {
  return {
    id,
    workspace_id: "ws-1",
    name: id,
    status: "draft",
    created_by: "alex",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
    ...partial,
  };
}

describe("buildFeed", () => {
  it("derives lifecycle events from change-set timestamps", () => {
    const cs = changeset("cs-1", {
      name: "Prefer Paiement",
      status: "merged",
      submitted_at: "2026-06-02T09:00:00Z",
      merged_at: "2026-06-03T10:00:00Z",
      merged_by: "sam",
    });
    const feed = buildFeed({ changesets: [cs] });
    const kinds = feed.map((f) => f.kind);
    expect(kinds).toContain("changeset.opened");
    expect(kinds).toContain("changeset.submitted");
    expect(kinds).toContain("changeset.merged");
    // Newest first → merged leads.
    expect(feed[0].kind).toBe("changeset.merged");
    expect(feed[0].actor).toBe("sam");
  });

  it("emits an abandoned event for abandoned change-sets", () => {
    const feed = buildFeed({
      changesets: [changeset("cs-x", { status: "abandoned", updated_at: "2026-06-05T00:00:00Z" })],
    });
    expect(feed.some((f) => f.kind === "changeset.abandoned")).toBe(true);
  });

  it("includes reviews and pilots from details", () => {
    const detail: ChangeSetDetail = {
      ...changeset("cs-1", { name: "Prefer Paiement" }),
      governed: true,
      ops: [],
      reviews: [
        {
          workspace_id: "ws-1",
          changeset_id: "cs-1",
          reviewer: "alex",
          verdict: "approve",
          created_at: "2026-06-02T12:00:00Z",
        },
      ],
      pilots: [
        {
          workspace_id: "ws-1",
          changeset_id: "cs-1",
          project_id: "p-web",
          stream: "main",
          created_by: "sam",
          created_at: "2026-06-02T13:00:00Z",
        },
      ],
    };
    const feed = buildFeed({ changesets: [], details: [detail] });
    expect(feed.find((f) => f.kind === "changeset.reviewed")?.detail).toMatch(/Approved/);
    expect(feed.find((f) => f.kind === "pilot.started")?.detail).toMatch(/p-web/);
  });

  it("maps story entries to concept/observation/comment, skipping changeset entries", () => {
    const story: StorySource = {
      conceptId: "c-checkout",
      entries: [
        { kind: "revision", at: "2026-06-01T00:00:00Z", actor: "alex", summary: "Created" },
        { kind: "observation", at: "2026-06-02T00:00:00Z", actor: "alex", summary: "Saw rival" },
        { kind: "comment", at: "2026-06-03T00:00:00Z", actor: "sam", summary: "Discuss fr-FR" },
        { kind: "changeset", at: "2026-06-04T00:00:00Z", actor: "sam", summary: "Opened exp" },
      ],
    };
    const feed = buildFeed({
      changesets: [],
      stories: [story],
      conceptNames: { "c-checkout": "Checkout" },
    });
    const byKind = feed.map((f) => f.kind);
    expect(byKind).toEqual(["comment", "observation", "concept.revision"]);
    expect(feed.every((f) => f.title === "Checkout")).toBe(true);
    expect(byKind).not.toContain("changeset");
  });

  it("de-duplicates identical events from overlapping sources", () => {
    const detail: ChangeSetDetail = {
      ...changeset("cs-1", {}),
      governed: false,
      ops: [],
      reviews: [
        {
          workspace_id: "ws-1",
          changeset_id: "cs-1",
          reviewer: "alex",
          verdict: "approve",
          created_at: "2026-06-02T12:00:00Z",
        },
      ],
      pilots: [],
    };
    const feed = buildFeed({ changesets: [], details: [detail, detail] });
    expect(feed.filter((f) => f.kind === "changeset.reviewed")).toHaveLength(1);
  });

  it("drops items with invalid timestamps and respects the limit", () => {
    const stories: StorySource[] = [
      {
        conceptId: "c1",
        entries: [
          { kind: "revision", at: "", summary: "no date" },
          { kind: "revision", at: "2026-06-01T00:00:00Z", summary: "ok" },
        ],
      },
    ];
    const feed = buildFeed({ changesets: [], stories, limit: 1 });
    expect(feed).toHaveLength(1);
    expect(feed[0].detail).toBe("ok");
  });
});

describe("conceptDisplayName", () => {
  const base: ConceptInfo = {
    id: "c1",
    domain: "commerce",
    definition: "",
    terms: [],
    created_at: "",
    updated_at: "",
  };

  it("prefers a preferred term", () => {
    expect(
      conceptDisplayName({
        ...base,
        terms: [
          { text: "Caisse", locale: "fr-FR", status: "proposed" },
          { text: "Checkout", locale: "en-US", status: "preferred" },
        ],
      }),
    ).toBe("Checkout");
  });

  it("falls back to an English term, then the first", () => {
    expect(
      conceptDisplayName({
        ...base,
        terms: [
          { text: "Kasse", locale: "de-DE", status: "approved" },
          { text: "Cart", locale: "en-GB", status: "admitted" },
        ],
      }),
    ).toBe("Cart");
    expect(conceptDisplayName({ ...base, terms: [] })).toBe("commerce");
  });
});

describe("dayLabel + groupByDay", () => {
  // Local-time (no trailing Z) so day boundaries are deterministic regardless of
  // the machine timezone — dayLabel/groupByDay intentionally bucket by local day.
  const now = new Date("2026-06-13T12:00:00").getTime();

  it("labels today and yesterday relatively", () => {
    expect(dayLabel("2026-06-13T08:00:00", now)).toBe("Today");
    expect(dayLabel("2026-06-12T23:00:00", now)).toBe("Yesterday");
    expect(dayLabel("2026-06-01T08:00:00", now)).toMatch(/Jun/);
  });

  it("buckets a sorted feed into ordered day groups", () => {
    const feed = buildFeed({
      changesets: [
        changeset("a", { created_at: "2026-06-13T09:00:00" }),
        changeset("b", { created_at: "2026-06-12T09:00:00" }),
        changeset("c", { created_at: "2026-06-12T08:00:00" }),
      ],
    });
    const groups = groupByDay(feed, now);
    expect(groups.map((g) => g.label)).toEqual(["Today", "Yesterday"]);
    expect(groups[1].items).toHaveLength(2);
  });
});
