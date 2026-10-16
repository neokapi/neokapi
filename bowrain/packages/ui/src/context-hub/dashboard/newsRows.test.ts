import { describe, expect, it } from "vite-plus/test";
import type { ChangeSet } from "../../types/brand-graph";
import type { CandidateRule } from "../../voice/types";
import { RECENT_CHANGESETS, profileNews, suggestionTotals, whatsNewRows } from "./newsRows";

const cs = (over: Partial<ChangeSet>): ChangeSet => ({
  id: "cs",
  workspace_id: "ws",
  name: "A change",
  status: "draft",
  created_by: "u-1",
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  ...over,
});

const rule = (over: Partial<CandidateRule>): CandidateRule => ({
  term: "utilize",
  replacement: "use",
  correction_count: 3,
  dimension: "vocabulary",
  status: "pending",
  ...over,
});

describe("whatsNewRows", () => {
  it("leads with what waits, longest-waiting first, then profiles, then what moved", () => {
    const rows = whatsNewRows(
      [
        cs({ id: "merged", status: "merged", updated_at: "2026-10-10T00:00:00Z" }),
        cs({ id: "late", status: "in_review", submitted_at: "2026-10-09T00:00:00Z" }),
        cs({ id: "early", status: "approved", submitted_at: "2026-10-02T00:00:00Z" }),
      ],
      [
        { id: "quiet", name: "Quiet", candidates: [] },
        { id: "busy", name: "Busy", candidates: [rule({})] },
        {
          id: "split",
          name: "Split",
          candidates: [rule({ replacement: "use" }), rule({ replacement: "employ" })],
        },
      ],
    );
    expect(rows.map((r) => r.key)).toEqual([
      "changeset:early",
      "changeset:late",
      "profile:split",
      "profile:busy",
      "changeset:merged",
    ]);
  });

  it("dates a waiting change-set by when it was submitted, and the rest by their last move", () => {
    const [waiting, moved] = whatsNewRows(
      [
        cs({ id: "a", status: "in_review", submitted_at: "2026-10-05T00:00:00Z" }),
        cs({ id: "b", status: "merged", updated_at: "2026-10-07T00:00:00Z" }),
      ],
      [],
    );
    expect(waiting.type === "changeset" && waiting.at).toBe("2026-10-05T00:00:00Z");
    expect(moved.type === "changeset" && moved.at).toBe("2026-10-07T00:00:00Z");
  });

  it("caps what moved recently, never what waits", () => {
    const moved = Array.from({ length: RECENT_CHANGESETS + 3 }, (_, i) =>
      cs({ id: `m${i}`, status: "merged" }),
    );
    const waiting = Array.from({ length: 8 }, (_, i) => cs({ id: `w${i}`, status: "in_review" }));
    const rows = whatsNewRows([...moved, ...waiting], []);
    expect(rows.filter((r) => r.type === "changeset" && r.waiting)).toHaveLength(8);
    expect(rows.filter((r) => r.type === "changeset" && !r.waiting)).toHaveLength(
      RECENT_CHANGESETS,
    );
  });
});

describe("profileNews", () => {
  it("counts a profile's suggestions by kind, the approved ones, and the disagreements", () => {
    const news = profileNews({
      id: "p",
      name: "Core voice",
      candidates: [
        rule({ term: "synergy", replacement: "teamwork" }),
        rule({ term: "Dear customer", replacement: "Hello", dimension: "tone" }),
        rule({ term: "Globex", replacement: "", status: "approved" }),
        rule({ term: "log in", replacement: "sign in" }),
        rule({ term: "log in", replacement: "log on" }),
      ],
    });
    expect(news).toMatchObject({ words: 1, notes: 1, approved: 1, conflicts: 1 });
  });

  it("leaves out a profile with nothing to decide", () => {
    expect(
      profileNews({ id: "p", name: "Done", candidates: [rule({ status: "promoted" })] }),
    ).toBeNull();
  });
});

describe("suggestionTotals", () => {
  it("counts each phrasing and form once, however many profiles list it", () => {
    const shared = [rule({}), rule({ term: "synergy", replacement: "teamwork" })];
    const totals = suggestionTotals([
      { id: "a", name: "A", candidates: shared },
      { id: "b", name: "B", candidates: shared },
      {
        id: "c",
        name: "C",
        candidates: [
          rule({ term: "log in", replacement: "sign in" }),
          rule({ term: "log in", replacement: "log on" }),
        ],
      },
    ]);
    expect(totals).toEqual({ suggestions: 4, conflicts: 1 });
  });
});
