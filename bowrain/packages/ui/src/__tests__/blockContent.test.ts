import { describe, it, expect } from "vite-plus/test";
import type { Run } from "@neokapi/kapi-format";

import {
  blockHasInlineCodes,
  hasInlineCodes,
  plainText,
  pluralOf,
  soloPlural,
  sourceRuns,
  targetRuns,
} from "../components/editor/blockContent";
import { toKapiBlock } from "../components/blockAdapter";
import type { BlockInfo } from "../types/api";

const count: Run = { ph: { id: "0", type: "jsx:var", data: "{count}", equiv: "count" } };
const open: Run = { pcOpen: { id: "1", type: "fmt:bold", data: "<b>", equiv: "b" } };
const close: Run = { pcClose: { id: "1", type: "fmt:bold", data: "</b>", equiv: "b" } };
const plural: Run = {
  plural: {
    pivot: "count",
    forms: { one: [count, { text: " message" }], other: [count, { text: " messages" }] },
  },
};

function block(overrides: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b1",
    source: "You have {count} messages",
    targets: {},
    translatable: true,
    properties: {},
    ...overrides,
  };
}

describe("a block's content as runs", () => {
  it("reads the served runs, else the text as one run", () => {
    const runs: Run[] = [{ text: "You have " }, count, { text: " messages" }];
    expect(sourceRuns(block({ source_runs: runs as BlockInfo["source_runs"] }))).toEqual(runs);
    expect(sourceRuns(block())).toEqual([{ text: "You have {count} messages" }]);
    expect(sourceRuns(block({ source: "" }))).toEqual([]);

    expect(
      targetRuns(
        block({ targets: { de: "x" }, targets_runs: { de: runs } as BlockInfo["targets_runs"] }),
        "de",
      ),
    ).toEqual(runs);
    expect(
      targetRuns(block({ targets: { de: { text: "Hallo", status: "draft" } } }), "de"),
    ).toEqual([{ text: "Hallo" }]);
    expect(targetRuns(block(), "de")).toEqual([]);
  });

  it("recognises a translation that is one plural, as a run or as ICU text", () => {
    expect(soloPlural([plural])).toBe(plural);
    expect(soloPlural([{ text: "a" }, plural])).toBeNull();
    expect(pluralOf([plural])).toBe(plural);
    const icu = pluralOf([
      { text: "{count, plural, one {Eine Nachricht} other {{count} Nachrichten}}" },
    ]);
    expect(icu?.plural.pivot).toBe("count");
    expect(Object.keys(icu?.plural.forms ?? {})).toEqual(["one", "other"]);
    expect(pluralOf([{ text: "Plain text" }])).toBeNull();
    expect(pluralOf(undefined)).toBeNull();
  });

  it("finds inline codes anywhere, plural forms included", () => {
    expect(hasInlineCodes([{ text: "a" }])).toBe(false);
    expect(hasInlineCodes([{ text: "a" }, open, { text: "b" }, close])).toBe(true);
    expect(hasInlineCodes([plural])).toBe(true);
    expect(hasInlineCodes([{ plural: { pivot: "n", forms: { other: [{ text: "n" }] } } }])).toBe(
      false,
    );
    // The server's flag wins when present; the runs answer otherwise.
    expect(blockHasInlineCodes(block({ has_inline_codes: true }))).toBe(true);
    expect(
      blockHasInlineCodes(
        block({ source_runs: [{ text: "a" }, count] as BlockInfo["source_runs"] }),
      ),
    ).toBe(true);
    expect(blockHasInlineCodes(block())).toBe(false);
  });

  it("reads plain text as the engine does: codes contribute nothing, a plural its other branch", () => {
    expect(plainText([open, { text: "Bonjour" }, close, { text: " monde" }])).toBe("Bonjour monde");
    expect(plainText([plural])).toBe(" messages");
    expect(plainText(undefined)).toBe("");
  });
});

describe("toKapiBlock — the placeholder table the plural editor reads", () => {
  it("names a variable once, an element by its opening half, and a plural's pivot", () => {
    const b = block({
      source_runs: [{ text: "a" }, open, count, close, count] as BlockInfo["source_runs"],
    });
    expect(toKapiBlock(b).placeholders).toEqual([
      { name: "b", kind: "element", sourceExpr: "<b>" },
      { name: "count", kind: "variable", sourceExpr: "{count}" },
    ]);
    const p = toKapiBlock(block({ source_runs: [plural] as BlockInfo["source_runs"] }));
    expect(p.placeholders[0]).toMatchObject({ name: "count", kind: "icu-pivot" });
    expect(p.placeholders).toHaveLength(1);
  });

  it("carries the source runs as the source edition", () => {
    expect(toKapiBlock(block()).editions[""].runs).toEqual([{ text: "You have {count} messages" }]);
  });
});
