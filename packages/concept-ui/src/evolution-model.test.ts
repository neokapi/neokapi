// Tests for the pure evolution-model builder (Apache-2.0). They pin the
// behaviours the renderers depend on: language lanes from term validity, the
// `directory → folder` rename, a sibling locale branching after genesis, the
// shared context track, density clustering into "clouds", language focus, span
// end-caps, and graceful degradation for a bare concept.

import { describe, expect, it } from "vitest";
import { buildEvolutionModel, clusterMilestones } from "./evolution-model";
import type { EvolutionMilestone } from "./evolution-types";
import { SIGNAL_IMPORTANCE } from "./evolution-types";
import type { Concept, Relation, TermStatus, TimelineEvent } from "./types";

const NOW = "2026-06-14T00:00:00.000Z";

function concept(partial: Partial<Concept> & Pick<Concept, "id">): Concept {
  return { domain: "product", terms: [], ...partial };
}

function term(
  text: string,
  locale: string,
  status: TermStatus,
  validity?: { validFrom?: string; validTo?: string; market?: string },
) {
  return {
    text,
    locale,
    status,
    validity: validity
      ? {
          validFrom: validity.validFrom,
          validTo: validity.validTo,
          tags: validity.market ? { market: validity.market } : undefined,
        }
      : undefined,
  };
}

/** The running example: directory→folder (en), a Norwegian sibling, German. */
function evolutionConcept(): Concept {
  return concept({
    id: "c-folder",
    createdAt: "2023-01-01T00:00:00.000Z",
    updatedAt: "2024-06-01T00:00:00.000Z",
    terms: [
      term("directory", "en", "deprecated", {
        validFrom: "2023-01-01T00:00:00.000Z",
        validTo: "2024-06-01T00:00:00.000Z",
      }),
      term("folder", "en", "preferred", { validFrom: "2024-06-01T00:00:00.000Z" }),
      term("Ordner", "de", "approved", { validFrom: "2023-01-01T00:00:00.000Z" }),
      term("mappe", "nb", "preferred", {
        validFrom: "2024-01-01T00:00:00.000Z",
        market: "nordics",
      }),
    ],
  });
}

describe("buildEvolutionModel — lanes & spans", () => {
  it("derives one lane per locale with validity spans", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    const locales = m.lanes.map((l) => l.locale).sort();
    expect(locales).toEqual(["de", "en", "nb"]);
    const en = m.lanes.find((l) => l.locale === "en")!;
    expect(en.spans.map((s) => s.termText)).toEqual(["directory", "folder"]);
  });

  it("caps a deprecated term as banned and an open term as open", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    const en = m.lanes.find((l) => l.locale === "en")!;
    expect(en.spans.find((s) => s.termText === "directory")!.cap).toBe("banned");
    expect(en.spans.find((s) => s.termText === "folder")!.cap).toBe("open");
  });

  it("classifies bounded vs expired vs banned end-caps", () => {
    const c = concept({
      id: "c",
      createdAt: "2020-01-01T00:00:00.000Z",
      terms: [
        term("future", "en", "approved", {
          validFrom: "2020-01-01T00:00:00.000Z",
          validTo: "2030-01-01T00:00:00.000Z",
        }),
        term("past", "fr", "approved", {
          validFrom: "2020-01-01T00:00:00.000Z",
          validTo: "2022-01-01T00:00:00.000Z",
        }),
        term("gone", "de", "forbidden", {
          validFrom: "2020-01-01T00:00:00.000Z",
          validTo: "2021-01-01T00:00:00.000Z",
        }),
      ],
    });
    const m = buildEvolutionModel({ concept: c }, { now: NOW });
    const cap = (loc: string) => m.lanes.find((l) => l.locale === loc)!.spans[0].cap;
    expect(cap("en")).toBe("bounded");
    expect(cap("fr")).toBe("expired");
    expect(cap("de")).toBe("banned");
  });

  it("infers a start for a term with no validFrom", () => {
    const c = concept({
      id: "c",
      createdAt: "2021-03-01T00:00:00.000Z",
      terms: [term("thing", "en", "approved")],
    });
    const m = buildEvolutionModel({ concept: c }, { now: NOW });
    const span = m.lanes[0].spans[0];
    expect(span.startInferred).toBe(true);
    expect(span.start).toBe("2021-03-01T00:00:00.000Z");
    expect(span.cap).toBe("open");
  });
});

describe("buildEvolutionModel — branches", () => {
  it("detects a within-lane rename (directory → folder)", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    const en = m.lanes.find((l) => l.locale === "en")!;
    const rename = en.milestones.find((x) => x.kind === "rename");
    expect(rename).toBeDefined();
    expect(rename!.summary).toBe("directory → folder");
    expect(rename!.importance).toBeGreaterThanOrEqual(SIGNAL_IMPORTANCE);
  });

  it("branches a sibling locale that appears after genesis (nb), not the origin", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    const sibling = m.branches.find((b) => b.kind === "sibling");
    expect(sibling).toBeDefined();
    expect(sibling!.toLaneKey).toBe("nb");
    expect(sibling!.fromLaneKey).toBe("en"); // origin = English
    // de starts at genesis → it is NOT a sibling.
    expect(m.branches.filter((b) => b.kind === "sibling")).toHaveLength(1);
    const nb = m.lanes.find((l) => l.locale === "nb")!;
    expect(nb.milestones.some((x) => x.kind === "sibling")).toBe(true);
  });

  it("turns an outgoing REPLACED_BY into a rename branch that navigates", () => {
    const rel: Relation = {
      id: "r1",
      sourceId: "c-folder",
      targetId: "c-archive",
      type: "REPLACED_BY",
      validity: { validFrom: "2025-02-01T00:00:00.000Z" },
    };
    const m = buildEvolutionModel(
      {
        concept: evolutionConcept(),
        relations: [rel],
        neighbourLabels: { "c-archive": "archive" },
      },
      { now: NOW },
    );
    const branch = m.branches.find((b) => b.kind === "rename");
    expect(branch).toBeDefined();
    expect(branch!.navigateId).toBe("c-archive");
    expect(branch!.label).toContain("archive");
  });

  it("emits a dated relation milestone for a competitor", () => {
    const rel: Relation = {
      id: "r2",
      sourceId: "c-folder",
      targetId: "c-explorer",
      type: "COMPETITOR",
      validity: { validFrom: "2025-01-01T00:00:00.000Z" },
    };
    const m = buildEvolutionModel(
      {
        concept: evolutionConcept(),
        relations: [rel],
        neighbourLabels: { "c-explorer": "Explorer" },
      },
      { now: NOW },
    );
    expect(m.milestones.some((x) => x.kind === "relation" && x.summary.includes("Explorer"))).toBe(
      true,
    );
  });

  it("ignores an undated relation (it lives in the Relations panel, not the timeline)", () => {
    const rel: Relation = { id: "r3", sourceId: "c-folder", targetId: "c-x", type: "RELATED" };
    const m = buildEvolutionModel({ concept: evolutionConcept(), relations: [rel] }, { now: NOW });
    expect(m.milestones.some((x) => x.ref === "r3")).toBe(false);
    expect(m.branches.some((b) => b.id.includes("r3"))).toBe(false);
  });
});

describe("buildEvolutionModel — context track", () => {
  it("marks a locale introduced after genesis (nb)", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    const intro = m.context.find((x) => x.kind === "market");
    expect(intro).toBeDefined();
    expect(intro!.at).toBe("2024-01-01T00:00:00.000Z");
  });

  it("uses a named market label when one covers the locale", () => {
    const m = buildEvolutionModel(
      { concept: evolutionConcept(), markets: [{ name: "Nordics", locales: ["nb-NO", "nb"] }] },
      { now: NOW },
    );
    expect(m.context.find((x) => x.kind === "market")!.label).toContain("Nordics");
  });

  it("adds governed change-sets from the revision log to the context track", () => {
    const timeline: TimelineEvent[] = [
      {
        kind: "changeset",
        at: "2024-06-01T00:00:00.000Z",
        summary: "Rename directory→folder merged",
        ref: "cs1",
      },
    ];
    const m = buildEvolutionModel({ concept: evolutionConcept(), timeline }, { now: NOW });
    expect(m.context.some((x) => x.kind === "changeset" && x.ref === "cs1")).toBe(true);
  });
});

describe("clusterMilestones — clouds", () => {
  const ms = (at: string, importance: number, id: string): EvolutionMilestone => ({
    id,
    at,
    tone: importance >= SIGNAL_IMPORTANCE ? "genesis" : "edit",
    kind: "revision",
    importance,
    summary: id,
  });

  it("folds a run of low-importance, time-close events into one cluster", () => {
    const window = 7 * 86_400_000; // 7 days
    const items = [
      ms("2024-03-01T00:00:00.000Z", 20, "a"),
      ms("2024-03-02T00:00:00.000Z", 20, "b"),
      ms("2024-03-03T00:00:00.000Z", 20, "c"),
    ];
    const { kept, clusters } = clusterMilestones(items, window, "en");
    expect(kept).toHaveLength(0);
    expect(clusters).toHaveLength(1);
    expect(clusters[0].count).toBe(3);
    expect(clusters[0].laneKey).toBe("en");
  });

  it("keeps high-importance milestones discrete, even amid noise", () => {
    const window = 30 * 86_400_000;
    const items = [
      ms("2024-03-01T00:00:00.000Z", 20, "edit1"),
      ms("2024-03-02T00:00:00.000Z", 90, "GENESIS"),
      ms("2024-03-03T00:00:00.000Z", 20, "edit2"),
    ];
    const { kept, clusters } = clusterMilestones(items, window, undefined);
    expect(kept.map((k) => k.id)).toContain("GENESIS");
    // The two edits are split by the signal event, so neither run reaches 2.
    expect(clusters).toHaveLength(0);
    expect(kept).toHaveLength(3);
  });

  it("never clusters a lone low-importance event", () => {
    const { kept, clusters } = clusterMilestones(
      [ms("2024-03-01T00:00:00.000Z", 20, "solo")],
      86_400_000,
      undefined,
    );
    expect(kept).toHaveLength(1);
    expect(clusters).toHaveLength(0);
  });

  it("clusters dense routine edits in a full model while signal survives", () => {
    const timeline: TimelineEvent[] = [
      { kind: "create", at: "2023-01-01T00:00:00.000Z", summary: "Created" },
      { kind: "revision", at: "2024-03-01T00:00:00.000Z", summary: "tweak" },
      { kind: "revision", at: "2024-03-02T00:00:00.000Z", summary: "tweak" },
      { kind: "revision", at: "2024-03-03T00:00:00.000Z", summary: "tweak" },
      { kind: "revision", at: "2024-03-04T00:00:00.000Z", summary: "tweak" },
      { kind: "changeset", at: "2024-06-01T00:00:00.000Z", summary: "merged" },
    ];
    const m = buildEvolutionModel({ concept: evolutionConcept(), timeline }, { now: NOW });
    expect(m.derived).toBe(false);
    expect(m.globalClusters.length).toBeGreaterThanOrEqual(1);
    expect(m.milestones.some((x) => x.tone === "genesis")).toBe(true);
    expect(m.milestones.some((x) => x.tone === "governed")).toBe(true);
  });
});

describe("buildEvolutionModel — language focus", () => {
  function manyLocales(): Concept {
    const locales = ["en", "de", "fr", "es", "nb", "ja"];
    return concept({
      id: "c-many",
      createdAt: "2023-01-01T00:00:00.000Z",
      terms: locales.map((l, i) =>
        term(`t-${l}`, l, l === "en" ? "preferred" : "approved", {
          validFrom: `2023-0${(i % 8) + 1}-01T00:00:00.000Z`,
        }),
      ),
    });
  }

  it("focuses the top N and folds the rest into 'more'", () => {
    const m = buildEvolutionModel({ concept: manyLocales() }, { now: NOW, maxFocusLanes: 3 });
    expect(m.focusedLanes).toHaveLength(3);
    expect(m.moreLanes).toHaveLength(3);
    // Focused lanes render first.
    expect(m.lanes.slice(0, 3).every((l) => l.focused)).toBe(true);
  });

  it("honours an explicit focusLocales list", () => {
    const m = buildEvolutionModel(
      { concept: manyLocales() },
      { now: NOW, maxFocusLanes: 3, focusLocales: ["fr", "ja"] },
    );
    expect(m.focusedLanes.map((l) => l.locale).sort()).toEqual(["fr", "ja"]);
  });

  it("focuses every lane when there are few languages", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW, maxFocusLanes: 3 });
    expect(m.moreLanes).toHaveLength(0);
    expect(m.focusedLanes).toHaveLength(3);
  });
});

describe("buildEvolutionModel — extent & degradation", () => {
  it("spans genesis → now with year ticks for a multi-year history", () => {
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: NOW });
    expect(m.extent.unit).toBe("year");
    expect(m.extent.ticks.length).toBeGreaterThanOrEqual(3);
    expect(m.extent.end).toBe(NOW);
  });

  it("degrades to a single genesis milestone for a bare concept", () => {
    const c = concept({
      id: "bare",
      createdAt: "2026-01-01T00:00:00.000Z",
      terms: [term("thing", "en", "approved")],
    });
    const m = buildEvolutionModel({ concept: c }, { now: NOW });
    expect(m.derived).toBe(true);
    expect(m.lanes).toHaveLength(1);
    expect(m.milestones.filter((x) => x.kind === "create")).toHaveLength(1);
    expect(m.branches).toHaveLength(0);
  });

  it("treats an undated concept (terms but no dates anywhere) as empty history", () => {
    const c = concept({
      id: "undated",
      terms: [
        term("Wishlist", "en", "preferred"),
        // A legacy forbidden label beside the preferred one must NOT be read as
        // a dated rename — with no validity there is no moment to chart.
        term("Saved items", "en", "forbidden"),
        term("Merkliste", "de", "preferred"),
      ],
    });
    const m = buildEvolutionModel({ concept: c }, { now: NOW });
    // Lanes still exist (the terms do), but there is no dated history to chart.
    expect(m.lanes.length).toBe(2);
    expect(m.eventCount).toBe(0);
    expect(m.milestones).toHaveLength(0);
    expect(m.lanes.flatMap((l) => l.milestones)).toHaveLength(0);
  });

  it("clamps a future-dated revision to now", () => {
    const timeline: TimelineEvent[] = [
      { kind: "create", at: "2023-01-01T00:00:00.000Z", summary: "Created" },
      { kind: "revision", at: "2999-01-01T00:00:00.000Z", summary: "from the future" },
    ];
    const m = buildEvolutionModel({ concept: evolutionConcept(), timeline }, { now: NOW });
    expect(m.milestones.every((x) => Date.parse(x.at) <= Date.parse(NOW))).toBe(true);
  });

  it("can exclude discussion noise", () => {
    const timeline: TimelineEvent[] = [
      { kind: "create", at: "2023-01-01T00:00:00.000Z", summary: "Created" },
      { kind: "comment", at: "2024-01-01T00:00:00.000Z", summary: "chatter" },
      { kind: "observation", at: "2024-02-01T00:00:00.000Z", summary: "seen" },
    ];
    const without = buildEvolutionModel(
      { concept: evolutionConcept(), timeline },
      { now: NOW, includeDiscussion: false },
    );
    expect(without.milestones.some((x) => x.tone === "discussion")).toBe(false);
    expect(without.milestones.some((x) => x.tone === "evidence")).toBe(false);
  });
});

// Regressions pinned by the adversarial review.
describe("buildEvolutionModel — review regressions", () => {
  it("detects a rename when the successor carries a future validTo (bounded, not open)", () => {
    const c = concept({
      id: "c",
      createdAt: "2023-01-01T00:00:00.000Z",
      terms: [
        term("directory", "en", "deprecated", {
          validFrom: "2023-01-01T00:00:00.000Z",
          validTo: "2024-06-01T00:00:00.000Z",
        }),
        term("folder", "en", "preferred", {
          validFrom: "2024-06-01T00:00:00.000Z",
          validTo: "2030-01-01T00:00:00.000Z",
        }),
      ],
    });
    const en = buildEvolutionModel({ concept: c }, { now: NOW }).lanes.find(
      (l) => l.locale === "en",
    )!;
    expect(en.spans.find((s) => s.termText === "folder")!.cap).toBe("bounded");
    expect(en.milestones.find((x) => x.kind === "rename")!.summary).toBe("directory → folder");
  });

  it("does not emit a backwards rename when the open term is older than the retired one", () => {
    const c = concept({
      id: "c",
      createdAt: "2023-01-01T00:00:00.000Z",
      terms: [
        term("folder", "en", "preferred", { validFrom: "2023-01-01T00:00:00.000Z" }),
        term("directory", "en", "deprecated", {
          validFrom: "2024-01-01T00:00:00.000Z",
          validTo: "2025-01-01T00:00:00.000Z",
        }),
      ],
    });
    const en = buildEvolutionModel({ concept: c }, { now: NOW }).lanes.find(
      (l) => l.locale === "en",
    )!;
    expect(en.milestones.some((x) => x.kind === "rename")).toBe(false);
  });

  it("does not branch a locale present at genesis via an undated term, even once it gains a dated term", () => {
    const c = concept({
      id: "c",
      createdAt: "2023-01-01T00:00:00.000Z",
      terms: [
        term("folder", "en", "preferred", { validFrom: "2023-01-01T00:00:00.000Z" }),
        term("Ordner", "de", "approved"), // undated → de existed at genesis
        term("Verzeichnis", "de", "deprecated", {
          validFrom: "2024-01-01T00:00:00.000Z",
          validTo: "2024-06-01T00:00:00.000Z",
        }),
      ],
    });
    const m = buildEvolutionModel({ concept: c }, { now: NOW });
    expect(m.branches.some((b) => b.kind === "sibling" && b.toLaneKey === "de")).toBe(false);
    expect(m.context.some((x) => x.kind === "market")).toBe(false);
  });

  it("ignores a future-dated relation and does not stretch the axis past now", () => {
    const rel: Relation = {
      id: "rf",
      sourceId: "c-folder",
      targetId: "c-z",
      type: "COMPETITOR",
      validity: { validFrom: "2030-01-01T00:00:00.000Z" },
    };
    const m = buildEvolutionModel({ concept: evolutionConcept(), relations: [rel] }, { now: NOW });
    expect(m.milestones.some((x) => x.ref === "rf")).toBe(false);
    expect(Date.parse(m.extent.end)).toBeLessThanOrEqual(Date.parse(NOW));
  });

  it("does not throw on an unparseable now and still produces a usable extent", () => {
    expect(() =>
      buildEvolutionModel({ concept: evolutionConcept() }, { now: "not-a-date" }),
    ).not.toThrow();
    const m = buildEvolutionModel({ concept: evolutionConcept() }, { now: "not-a-date" });
    expect(m.lanes.length).toBeGreaterThan(0);
    expect(m.extent.end).toMatch(/^\d{4}-\d{2}-\d{2}/);
  });
});
