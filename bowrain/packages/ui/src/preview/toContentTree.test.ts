import { describe, expect, it } from "vitest";
import type { Run } from "@neokapi/contract-types";
import { resolveOverlaySpans, runsPlainText } from "@neokapi/ui-primitives/preview";
import type { BlockInfo, BlockTermMatch, EntityInfo, QAIssue } from "../types/api";
import { blockToContentNode, blocksToContentTree, type BlockFinding } from "./toContentTree";

// The fixtures mirror the REST blocks payload: typed runs alongside the flat
// `source` / `targets` text, entities positioned by byte offset.

const codedSource: Run[] = [
  { text: "Utilize the " },
  { pcOpen: { id: "1", type: "link", data: "<a>", equiv: "a", attrs: { href: "/docs" } } },
  { text: "Acme" },
  { pcClose: { id: "1", type: "link", data: "</a>", equiv: "a" } },
  { text: " console." },
];

function block(over: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "Utilize the Acme console.",
    source_runs: codedSource,
    targets: { "fr-FR": { text: "Utilisez la console Acme.", status: "translated" } },
    targets_runs: { "fr-FR": [{ text: "Utilisez la console Acme." }] },
    translatable: true,
    has_spans: true,
    properties: {},
    ...over,
  };
}

const termMatch: BlockTermMatch = {
  source_term: "Acme",
  target_terms: ["Acme"],
  domain: "brand",
  status: "preferred",
  // "Acme" is bytes 12..16 of "Utilize the Acme console."
  start: 12,
  end: 16,
};

const entity: EntityInfo = {
  key: "entity:0",
  text: "console",
  type: "entity:product",
  start: 17,
  end: 24,
  dnt: true,
};

const voiceFinding: BlockFinding = {
  category: "voice-vocabulary",
  severity: "major",
  message: 'Forbidden term "Utilize" found',
  suggestion: 'Use "Use" instead',
  original_text: "Utilize",
  position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: 7 } },
  metadata: { replacement: "Use", concept_id: "c-42" },
};

describe("blockToContentNode — content", () => {
  it("carries the typed source runs through untouched", () => {
    const node = blockToContentNode(block());
    expect(node.kind).toBe("block");
    expect(node.id).toBe("b1");
    expect(node.source).toEqual(codedSource);
    expect(node.translatable).toBe(true);
    // The flattening the kit renders round-trips to the payload's plain source.
    expect(runsPlainText(node.source)).toBe("Utilize the Acme console.");
  });

  it("synthesises a single text run when the payload carries no typed runs", () => {
    const node = blockToContentNode(block({ source_runs: undefined, targets_runs: undefined }));
    expect(node.source).toEqual([{ text: "Utilize the Acme console." }]);
    expect(node.targets).toEqual({ "fr-FR": [{ text: "Utilisez la console Acme." }] });
  });

  it("projects per-locale targets with their status as targetMeta", () => {
    const node = blockToContentNode(block());
    expect(node.targets).toEqual({ "fr-FR": [{ text: "Utilisez la console Acme." }] });
    expect(node.targetMeta).toEqual({ "fr-FR": { status: "translated" } });
  });

  it("reads a bare-string target entry (legacy payload) as target text", () => {
    const node = blockToContentNode(
      block({ targets: { de: "Nutzen Sie die Acme-Konsole." }, targets_runs: undefined }),
    );
    expect(node.targets).toEqual({ de: [{ text: "Nutzen Sie die Acme-Konsole." }] });
    // A bare string carries no status, so the locale gets no targetMeta entry.
    expect(node.targetMeta).toBeUndefined();
  });

  it("omits an empty target rather than emitting an empty run sequence", () => {
    const node = blockToContentNode(
      block({ targets: { "fr-FR": { text: "" } }, targets_runs: undefined }),
    );
    expect(node.targets).toEqual({ "fr-FR": [] });
    expect(node.targetMeta).toBeUndefined();
  });

  it("restricts targets to the requested locales", () => {
    const node = blockToContentNode(
      block({
        targets: { "fr-FR": { text: "Une" }, de: { text: "Zwei" }, ja: { text: "三" } },
        targets_runs: undefined,
      }),
      { locales: ["de", "ja"] },
    );
    expect(Object.keys(node.targets ?? {})).toEqual(["de", "ja"]);
  });

  it("carries properties, the block type and the source locale", () => {
    const node = blockToContentNode(
      block({ properties: { path: "$.intro", type: "json:value" } }),
      { sourceLocale: "en-US", parentId: "guide.json" },
    );
    expect(node.properties).toEqual({ path: "$.intro", type: "json:value" });
    expect(node.type).toBe("json:value");
    expect(node.sourceLocale).toBe("en-US");
    expect(node.parentId).toBe("guide.json");
  });

  it("omits every optional field the web payload cannot fill", () => {
    const node = blockToContentNode(block());
    expect(node.structure).toBeUndefined();
    expect(node.geometry).toBeUndefined();
    expect(node.timing).toBeUndefined();
    expect(node.media).toBeUndefined();
    expect(node.segments).toBeUndefined();
    expect(node.relations).toBeUndefined();
    expect(node.annotations).toBeUndefined();
    expect(node.overlays).toBeUndefined();
    // Status is the only target metadata the payload carries.
    expect(node.targetMeta?.["fr-FR"]).toEqual({ status: "translated" });
    expect(node.targetMeta?.["fr-FR"].score).toBeUndefined();
    expect(node.targetMeta?.["fr-FR"].origin).toBeUndefined();
  });
});

describe("blockToContentNode — overlays", () => {
  it("anchors a term match to its run range, skipping inline codes", () => {
    const node = blockToContentNode(block(), { evidence: { terms: [termMatch] } });
    const term = node.overlays?.find((o) => o.type === "term");
    expect(term?.side).toBe("source");
    expect(term?.spans).toHaveLength(1);
    // "Acme" is run 2, but both byte boundaries land on a run edge, so each
    // attaches to the following run — the link's pcOpen (1) and pcClose (3).
    // That is the engine's convention: a leading code belongs to its span.
    expect(term?.spans[0].range).toEqual({
      kind: "range",
      start: { run: 1 },
      end: { run: 3 },
    });
    expect(term?.spans[0].text).toBe("Acme");
    expect(term?.spans[0].props).toEqual({
      term: "Acme",
      target: "Acme",
      domain: "brand",
      status: "preferred",
    });
  });

  it("anchors an entity to its run range and marks do-not-translate", () => {
    const node = blockToContentNode(block({ entities: [entity] }));
    const entities = node.overlays?.find((o) => o.type === "entity");
    expect(entities?.spans).toHaveLength(1);
    expect(entities?.spans[0].id).toBe("entity:0");
    expect(entities?.spans[0].text).toBe("console");
    expect(entities?.spans[0].props).toEqual({ type: "entity:product", dnt: "true" });
    expect(entities?.spans[0].range.start.run).toBe(4);
  });

  it("keeps a run-anchored finding's position and its category in span props", () => {
    const node = blockToContentNode(block(), { evidence: { findings: [voiceFinding] } });
    const qa = node.overlays?.find((o) => o.type === "qa");
    expect(qa?.side).toBe("source");
    expect(qa?.spans[0].range).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 0, offset: 7 },
    });
    expect(qa?.spans[0].props).toMatchObject({
      category: "voice-vocabulary",
      severity: "major",
      replacement: "Use",
      concept_id: "c-42",
    });
  });

  it("groups findings into one qa overlay per side", () => {
    const node = blockToContentNode(block(), {
      evidence: {
        findings: [
          voiceFinding,
          { ...voiceFinding, side: "fr-FR", original_text: "Utilisez" },
          { ...voiceFinding, category: "doubled-word", message: "Doubled word" },
        ],
      },
    });
    const qa = node.overlays?.filter((o) => o.type === "qa") ?? [];
    expect(qa.map((o) => o.side)).toEqual(["source", "fr-FR"]);
    expect(qa[0].spans).toHaveLength(2);
    expect(qa[1].spans).toHaveLength(1);
  });

  it("gives a finding with no position a zero range rather than guessing", () => {
    const node = blockToContentNode(block(), {
      evidence: { findings: [{ ...voiceFinding, position: undefined }] },
    });
    const qa = node.overlays?.find((o) => o.type === "qa");
    expect(qa?.spans[0].range).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 0 },
    });
  });

  it("drops empty props so a tooltip shows only what the payload knows", () => {
    const sparse: BlockTermMatch = { ...termMatch, domain: "", status: "", target_terms: [] };
    const node = blockToContentNode(block(), { evidence: { terms: [sparse] } });
    expect(node.overlays?.[0].spans[0].props).toEqual({ term: "Acme" });
  });

  it("resolves its own spans through the preview kit's highlighter", () => {
    const node = blockToContentNode(block({ entities: [entity] }), {
      evidence: { terms: [termMatch], findings: [voiceFinding] },
    });
    const text = runsPlainText(node.source);
    const spans = resolveOverlaySpans(node.overlays, "source", text);
    expect(spans.map((s) => text.slice(s.start, s.end))).toEqual(["Utilize", "Acme", "console"]);
    // The voice-vocabulary finding takes the dedicated accent, not generic QA.
    expect(spans[0].style.label).toBe("Voice");
  });

  it("anchors byte offsets over multi-byte source text by code point", () => {
    const runs: Run[] = [{ text: "Æblegrød fra Acme" }];
    // "Acme" is bytes 15..19 (Æ and ø are two bytes each), chars 13..17.
    const node = blockToContentNode(
      block({ source: "Æblegrød fra Acme", source_runs: runs, targets: {} }),
      { evidence: { terms: [{ ...termMatch, start: 15, end: 19 }] } },
    );
    expect(node.overlays?.[0].spans[0].range).toEqual({
      kind: "range",
      start: { run: 0, offset: 13 },
      end: { run: 1 },
    });
    expect(node.overlays?.[0].spans[0].text).toBe("Acme");
  });
});

describe("blockToContentNode — annotations", () => {
  it("records position-less check results as block annotations", () => {
    const issues: QAIssue[] = [
      { type: "placeholder", severity: "error", message: "Missing {count}" },
      { type: "spacing", severity: "warning", message: "Double space" },
    ];
    const node = blockToContentNode(block(), {
      evidence: { issues, issueLocale: "fr-FR" },
    });
    expect(node.annotations).toEqual([
      {
        key: "qa:0",
        type: "qa",
        summary: "Missing {count}",
        fields: { check: "placeholder", severity: "error", locale: "fr-FR" },
      },
      {
        key: "qa:1",
        type: "qa",
        summary: "Double space",
        fields: { check: "spacing", severity: "warning", locale: "fr-FR" },
      },
    ]);
    // They stay off the overlays: an unpositioned issue is not a span.
    expect(node.overlays).toBeUndefined();
  });

  // The QA endpoints dropped core/check.Finding's Position at the boundary, so
  // every issue reached this projection unpositioned and could only ever be an
  // annotation. It rides along now, and a located issue is a span like any
  // other finding.
  it("makes a positioned issue a span on the locale it was raised on", () => {
    const issues: QAIssue[] = [
      {
        type: "placeholder",
        severity: "error",
        message: "Missing {count}",
        position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: 7 } },
        original_text: "Utilize",
        suggestion: "Add {count}",
      },
      { type: "spacing", severity: "warning", message: "Double space" },
    ];
    const node = blockToContentNode(block(), { evidence: { issues, issueLocale: "fr-FR" } });

    const qa = node.overlays?.filter((o) => o.type === "qa") ?? [];
    expect(qa).toHaveLength(1);
    expect(qa[0].side).toBe("fr-FR");
    expect(qa[0].spans).toHaveLength(1);
    expect(qa[0].spans[0].range).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 0, offset: 7 },
    });
    expect(qa[0].spans[0].props).toEqual({
      category: "placeholder",
      severity: "error",
      message: "Missing {count}",
      suggestion: "Add {count}",
    });

    // Only the one the payload could not locate stays an annotation.
    expect(node.annotations).toHaveLength(1);
    expect(node.annotations?.[0].summary).toBe("Double space");
  });

  it("keeps one qa overlay per side when findings and issues share a locale", () => {
    const node = blockToContentNode(block(), {
      evidence: {
        findings: [{ ...voiceFinding, side: "fr-FR" }],
        issues: [
          {
            type: "placeholder",
            severity: "error",
            message: "Missing {count}",
            position: { kind: "range", start: { run: 0 }, end: { run: 0, offset: 7 } },
          },
        ],
        issueLocale: "fr-FR",
      },
    });
    const qa = node.overlays?.filter((o) => o.type === "qa") ?? [];
    expect(qa).toHaveLength(1);
    expect(qa[0].side).toBe("fr-FR");
    expect(qa[0].spans).toHaveLength(2);
  });
});

describe("blocksToContentTree", () => {
  it("wraps the blocks in a layer node named for the item", () => {
    const tree = blocksToContentTree([block(), block({ id: "b2" })], {
      format: "json",
      name: "guide.json",
    });
    expect(tree.format).toBe("json");
    expect(tree.root).toHaveLength(1);
    expect(tree.root[0]).toMatchObject({ kind: "layer", id: "guide.json", name: "guide.json" });
    expect(tree.root[0].children).toHaveLength(2);
    expect(tree.root[0].children?.[0].parentId).toBe("guide.json");
  });

  it("puts the blocks at the root when no item name is given", () => {
    const tree = blocksToContentTree([block()]);
    expect(tree.root).toHaveLength(1);
    expect(tree.root[0].kind).toBe("block");
    expect(tree.root[0].parentId).toBeUndefined();
    expect(tree.format).toBe("");
  });

  it("counts layers, blocks and runs in its stats", () => {
    const tree = blocksToContentTree([block(), block({ id: "b2" })], { name: "guide.json" });
    // Two blocks × (5 source runs + 1 target run).
    expect(tree.stats).toEqual({
      layers: 1,
      groups: 0,
      blocks: 2,
      data: 0,
      media: 0,
      runs: 12,
    });
  });

  it("keys evidence by block id", () => {
    const tree = blocksToContentTree([block(), block({ id: "b2" })], {
      evidence: { b2: { terms: [termMatch] } },
    });
    expect(tree.root[0].overlays).toBeUndefined();
    expect(tree.root[1].overlays?.[0].type).toBe("term");
  });

  it("projects an empty payload to an empty tree", () => {
    const tree = blocksToContentTree([]);
    expect(tree.root).toEqual([]);
    expect(tree.stats).toEqual({
      layers: 0,
      groups: 0,
      blocks: 0,
      data: 0,
      media: 0,
      runs: 0,
    });
  });

  it("projects an empty payload under a named item to an empty layer", () => {
    const tree = blocksToContentTree([], { name: "guide.json" });
    expect(tree.root).toHaveLength(1);
    expect(tree.root[0].children).toEqual([]);
    expect(tree.stats.layers).toBe(1);
    expect(tree.stats.blocks).toBe(0);
  });
});
