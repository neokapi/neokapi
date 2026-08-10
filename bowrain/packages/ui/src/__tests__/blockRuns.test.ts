import { describe, it, expect } from "vite-plus/test";
import type { Run } from "@neokapi/kapi-format";
import { normalizeServerBlock, type ServerBlockInfo } from "../components/editor/blockRuns";

/** A server block with typed source/target runs (RFC 0001 flat form). */
function serverBlock(over: Partial<ServerBlockInfo>): ServerBlockInfo {
  return {
    id: "b1",
    source: "Hello name",
    translatable: true,
    has_spans: false,
    properties: {},
    targets: {},
    ...over,
  };
}

const boldName: Run[] = [
  { text: "Hello " },
  { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } },
  { text: "name" },
  { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } },
];

describe("normalizeServerBlock", () => {
  it("derives coded text + spans from source_runs", () => {
    const block = normalizeServerBlock(
      serverBlock({ source_runs: boldName, has_inline_codes: true }),
    );
    expect(block.source_coded).toBeTruthy();
    // Two paired inline codes → two spans (one opening, one closing).
    expect(block.source_spans).toHaveLength(2);
    expect(block.source_spans?.[0].span_type).toBe("opening");
    expect(block.source_spans?.[1].span_type).toBe("closing");
    // The coded text carries private-use markers, not the literal text only.
    expect(block.source_coded).toContain("name");
    expect(block.has_spans).toBe(true);
  });

  it("leaves a text-only block plain with no spans", () => {
    const block = normalizeServerBlock(serverBlock({ source_runs: [{ text: "Just text" }] }));
    expect(block.source_coded).toBe("Just text");
    expect(block.source_spans).toEqual([]);
    expect(block.has_spans).toBe(false);
  });

  it("derives targets_coded from targets_runs per locale", () => {
    const block = normalizeServerBlock(
      serverBlock({
        source_runs: boldName,
        targets: { fr: { text: "Bonjour name", status: "translated" } },
        targets_runs: { fr: boldName },
      }),
    );
    expect(block.targets_coded?.fr).toBeTruthy();
    expect(block.targets_coded?.fr).toContain("name");
  });

  it("is idempotent when coded fields already exist", () => {
    const block = normalizeServerBlock(
      serverBlock({
        source_coded: "already coded",
        source_spans: [],
        source_runs: boldName,
      }),
    );
    // Existing coded text wins; runs are not re-applied.
    expect(block.source_coded).toBe("already coded");
  });

  it("falls back to plain source for plural/select runs it can't flatten", () => {
    const plural: Run[] = [
      {
        plural: {
          id: "p1",
          arg: "count",
          forms: { one: [{ text: "one item" }], other: [{ text: "many items" }] },
        },
      } as unknown as Run,
    ];
    const block = normalizeServerBlock(serverBlock({ source: "items", source_runs: plural }));
    // runsToCoded throws on plural → coded stays undefined, plain source shown.
    expect(block.source_coded).toBeUndefined();
    expect(block.has_spans).toBe(false);
  });

  it("keeps the typed runs and drops only the server-side flag", () => {
    const block = normalizeServerBlock(
      serverBlock({
        source_runs: boldName,
        targets_runs: { fr: boldName },
        has_inline_codes: true,
      }),
    ) as ServerBlockInfo;
    // The runs are the content model: the run-native projection reads them,
    // and flattening to coded text would lose the boundaries overlay ranges
    // anchor to.
    expect(block.source_runs).toEqual(boldName);
    expect(block.targets_runs).toEqual({ fr: boldName });
    // `has_inline_codes` is the server's spelling of `has_spans`; only the
    // normalised field survives.
    expect(block.has_inline_codes).toBeUndefined();
    expect(block.has_spans).toBe(true);
  });
});
