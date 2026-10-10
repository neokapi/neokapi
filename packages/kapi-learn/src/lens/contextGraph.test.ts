import { describe, expect, it } from "vitest";
import { buildContextGraph, parseJson } from "./contextGraph.ts";
import { layoutGraph } from "./layout.ts";
import type { LineRunner } from "./types.ts";

// A fake engine: each command answers with a fixed JSON, the way the Compass
// sample answers after `kapi store import ./context` and `kapi up`.
const answers: Record<string, unknown> = {
  "kapi ls --stats --json": {
    files: [{ path: "site/locales/en-GB.json", format: "json", blocks: 38, words: 171 }],
    total: 1,
    blocks: 38,
    words: 171,
  },
  "kapi status --json": {
    project: "compass",
    source: { total: 38 },
    locales: [
      {
        locale: "de",
        collection: "compass-app",
        total: 38,
        pct: { draft: 100, established: 53, translated: 100 },
        shipState: "translated",
        shippable: true,
      },
      {
        locale: "nb",
        collection: "compass-app",
        total: 38,
        pct: { draft: 100, established: 53, translated: 100 },
        shipState: "translated",
        shippable: true,
      },
      {
        locale: "nl",
        collection: "compass-app",
        total: 38,
        pct: { draft: 53, established: 0, translated: 53 },
        shipState: "withheld",
        blocking: "translated",
        failingChecks: 1,
      },
    ],
  },
  "kapi context 'site/locales/en-GB.json' --json": {
    point: {
      path: "site/locales/en-GB.json",
      profile: "northsea",
      channel: "app",
      collection: "compass-app",
      default: false,
    },
    voice: { name: "Northsea", source: "store:northsea", field: "defaults.voice" },
    terms: [
      {
        concept_id: "term:en-GB:seamless",
        term: "seamless",
        status: "forbidden",
        replacement: "unified",
      },
      { concept_id: "term:en-GB:berth", term: "berth", status: "preferred" },
    ],
    rules: [{}, {}],
    constraints: [
      { constraint: { id: "northsea/no-brochure-framing", statement: "Brochure framing." } },
    ],
    provenance: { revision: 84, stale: false },
  },
  "kapi terms search '' --json": {
    concepts: [
      {
        id: "term:en-GB:seamless",
        definition: "Says nothing a reader can check",
        terms: [{ text: "seamless" }],
      },
      {
        id: "term:en-GB:berth",
        definition: "Where a vessel ties up",
        terms: [{ text: "berth" }, { text: "berths" }],
      },
    ],
  },
  "kapi terms occurrences 'seamless' --json": { occurrences: null, blocks: 0 },
  "kapi terms occurrences 'berth' --json": {
    occurrences: [{ document: "site/locales/en-GB.json" }],
    blocks: 9,
  },
  "kapi memory stats --json": {
    entries: 69,
    locale_pairs: { de: 65, "en-GB": 69, nb: 67, nl: 15 },
  },
  "kapi context log --json": {
    operations: [
      {
        id: "a",
        kind: "import",
        actor: { kind: "person", name: "me" },
        subject: { kind: "note", text: "voice profile context/voice.yaml" },
        at: "2026-10-10T10:00:01Z",
        status: "established",
      },
      {
        id: "b",
        kind: "import",
        actor: { kind: "person", name: "me" },
        subject: { kind: "note", text: "decision record context/state, 54 decisions" },
        at: "2026-10-10T10:00:02Z",
        status: "established",
      },
      {
        id: "c",
        kind: "establish",
        actor: { kind: "person", name: "me" },
        subject: { kind: "block", text: "nl.json:nav.berths" },
        at: "2026-10-10T10:00:03Z",
        status: "established",
      },
      {
        id: "d",
        kind: "establish",
        actor: { kind: "person", name: "me" },
        subject: { kind: "block", text: "nl.json:nav.berths" },
        at: "2026-10-10T10:00:04Z",
        status: "established",
      },
    ],
  },
};

const run: LineRunner = async (line) => {
  const a = answers[line];
  return a === undefined ? { code: 1, out: "" } : { code: 0, out: JSON.stringify(a) };
};

describe("buildContextGraph", () => {
  it("draws the content, the point, what applies there and the record", async () => {
    const g = await buildContextGraph(run);
    const ids = g.nodes.map((n) => n.id);
    expect(ids).toContain("project");
    expect(ids).toContain("file:site/locales/en-GB.json");
    expect(ids).toEqual(expect.arrayContaining(["edition:de", "edition:nb", "edition:nl"]));
    expect(ids).toContain("point");
    expect(ids).toContain("voice");
    expect(ids).toContain("terms");
    expect(ids).toContain("concept:term:en-GB:seamless");
    expect(ids).toContain("memory");
    expect(ids.filter((i) => i.startsWith("record:"))).toHaveLength(3);

    const point = g.nodes.find((n) => n.id === "point")!;
    expect(point.label).toBe("northsea / app");
    const nl = g.nodes.find((n) => n.id === "edition:nl")!;
    expect(nl.status).toBe("withheld");
    expect(nl.pct?.translated).toBe(53);
    const seamless = g.nodes.find((n) => n.id === "concept:term:en-GB:seamless")!;
    expect(seamless.status).toBe("forbidden");
    expect(seamless.sub).toBe("say unified");

    expect(g.edges).toContainEqual(
      expect.objectContaining({ from: "point", to: "voice", kind: "governs" }),
    );
    expect(g.edges).toContainEqual(
      expect.objectContaining({
        from: "concept:term:en-GB:berth",
        to: "file:site/locales/en-GB.json",
        kind: "uses",
        weight: 9,
      }),
    );
    expect(g.edges.filter((e) => e.kind === "uses")).toHaveLength(1);
    expect(g.edges).toContainEqual(
      expect.objectContaining({ from: "memory", to: "edition:nl", kind: "recalls", label: "15" }),
    );

    // The record points at what it changed: the voice import at the voice,
    // the decisions at the editions, the two establish operations as one node.
    const voiceImport = g.nodes.find((n) => n.kind === "record" && n.sub?.includes("voice"))!;
    expect(g.edges).toContainEqual(
      expect.objectContaining({ from: voiceImport.id, to: "voice", kind: "records" }),
    );
    const establish = g.nodes.find((n) => n.kind === "record" && n.label === "2 established")!;
    expect(establish.sub).toBe("nl.json:nav.berths");
    expect(voiceImport.label).toBe("import: voice profile");
    const decisions = g.nodes.find((n) => n.kind === "record" && n.sub?.includes("54 decisions"))!;
    expect(decisions.label).toBe("import: 54 decisions");
    expect(decisions.sub).toBe("decision record context/state, 54 decisions");
    expect(g.edges).toContainEqual(
      expect.objectContaining({ from: establish.id, to: "edition:nl" }),
    );

    expect(g.revision).toBe(84);
    expect(g.stale).toBe(false);
    // The blocks a person established show per edition, counted from the share.
    const decided = g.nodes.find((n) => n.id === "decisions:de")!;
    expect(decided.label).toBe("20 established");
    expect(g.edges).toContainEqual(
      expect.objectContaining({ from: "decisions:de", to: "edition:de", kind: "records" }),
    );
    expect(g.nodes.find((n) => n.id === "decisions:nl")).toBeUndefined();

    expect(g.summary).toBe(
      "38 source blocks, 3 editions, 2 concepts, 69 memory entries, 4 recorded operations, 40 block decisions",
    );
  });

  it("leaves a part empty when its command fails, and skips occurrences while the engine is stale", async () => {
    const quiet: LineRunner = async (line) => {
      if (line.startsWith("kapi context 'site")) {
        return {
          code: 0,
          out: JSON.stringify({
            point: { path: "site/locales/en-GB.json", default: true },
            provenance: { stale: true },
          }),
        };
      }
      if (line.startsWith("kapi terms occurrences"))
        throw new Error("must not be asked while stale");
      if (line.startsWith("kapi memory")) return { code: 1, out: "Error: no store" };
      return run(line);
    };
    const g = await buildContextGraph(quiet);
    expect(g.nodes.find((n) => n.id === "point")?.label).toBe("project default");
    expect(g.nodes.some((n) => n.id === "voice")).toBe(false);
    expect(g.nodes.some((n) => n.id === "memory")).toBe(false);
    expect(g.edges.some((e) => e.kind === "uses")).toBe(false);
    expect(g.stale).toBe(true);
  });
});

describe("parseJson", () => {
  it("strips the colour the browser build adds and tolerates a report around the JSON", () => {
    const esc = String.fromCharCode(27);
    expect(parseJson(`${esc}[1m{"a":${esc}[33m1${esc}[0m}${esc}[0m`)).toEqual({ a: 1 });
    expect(parseJson('plan: 3 blocks\n{"ok":true}\n')).toEqual({ ok: true });
    expect(parseJson("")).toBeNull();
    expect(parseJson("not json")).toBeNull();
  });
});

describe("layoutGraph", () => {
  it("places every node once, in its column, with the record along the bottom", async () => {
    const g = await buildContextGraph(run);
    const l = layoutGraph(g, 1040);
    expect(l.placed.size).toBe(g.nodes.length);
    const x = (id: string) => l.placed.get(id)!.x;
    expect(x("project")).toBeLessThan(x("point"));
    expect(x("point")).toBeLessThan(x("voice"));
    for (const n of g.nodes.filter((n) => n.kind === "record")) {
      expect(l.placed.get(n.id)!.y).toBeGreaterThan(l.recordY);
    }
    expect(l.height).toBeGreaterThan(l.recordY);
    // Deterministic: the same model lays out the same way.
    const again = layoutGraph(g, 1040);
    expect([...again.placed.values()].map((p) => [p.node.id, p.x, p.y])).toEqual(
      [...l.placed.values()].map((p) => [p.node.id, p.x, p.y]),
    );
  });
});
