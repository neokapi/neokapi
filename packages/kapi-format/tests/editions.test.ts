import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

import type { Block, BlockV1 } from "../src/index.ts";
import {
  SchemaVersion,
  SourceEdition,
  editionRuns,
  flattenRuns,
  isLanguageKey,
  marshalBlock,
  marshalFile,
  parseFile,
  sourceEditions,
  sourceRuns,
  targetKeys,
  upgradeBlock,
} from "../src/index.ts";

// The Go fixtures the KBF format reader and writer are tested against: two
// catalogs kapi wrote in schema 1.0, and what kapi writes for each in the
// current schema (core/formats/jsx/roundtrip_test.go).
const goTestdata = new URL("../../../core/formats/jsx/testdata/", import.meta.url);
const goFixture = (name: string) => readFileSync(new URL(name, goTestdata), "utf-8");
const decode = (bytes: Uint8Array) => new TextDecoder().decode(bytes);

const v1Block: BlockV1 = {
  id: "d1:b1",
  hash: "h1",
  translatable: true,
  type: "jsx:element",
  source: [{ text: "Sign in" }],
  targets: {
    nb: [{ text: "Logg inn" }],
    de: [{ text: "Anmelden" }],
    "": [{ text: "EMPTYLOC" }],
  },
  targetOrigins: {
    nb: { kind: "ai", engine: "claude", context_fingerprint: "cfp-1" },
    "": { kind: "human" },
    fr: { kind: "mt" },
  },
  placeholders: [],
  properties: { file: "a.tsx", line: 1, component: "C", jsxPath: "p", element: "p" },
};

describe("a schema 1.0 block", () => {
  it("reads as the editions it describes", () => {
    const b = upgradeBlock(v1Block);
    expect(flattenRuns(sourceRuns(b))).toBe("Sign in");
    expect(targetKeys(b)).toEqual(["de", "nb"]);
    expect(b.editions.nb).toEqual({
      runs: [{ text: "Logg inn" }],
      origin: { kind: "ai", engine: "claude", context_fingerprint: "cfp-1" },
    });
    expect(b.editions.de).toEqual({ runs: [{ text: "Anmelden" }] });
    expect(b.unlabelled).toEqual({ runs: [{ text: "EMPTYLOC" }], origin: { kind: "human" } });
    expect("source" in b || "targets" in b || "targetOrigins" in b).toBe(false);
  });

  it("is refused when it carries editions too", () => {
    const both = { ...v1Block, editions: sourceEditions([{ text: "x" }]) } as unknown as BlockV1;
    expect(() => upgradeBlock(both)).toThrow(/both editions and the schema 1 source/);
  });

  it("leaves a block already in the current shape as it is", () => {
    const b: Block = { ...upgradeBlock(v1Block) };
    expect(upgradeBlock(b)).toBe(b);
  });
});

describe("parseFile", () => {
  const envelope = (schemaVersion: string, kind = "kapi-bundle") =>
    JSON.stringify({
      schemaVersion,
      kind,
      generator: { id: "x", version: "1" },
      project: { id: "p", sourceLocale: "en" },
      documents: [{ id: "d", documentType: "jsx", path: "a.tsx", blocks: [v1Block] }],
    });

  it("reads a schema 1.0 file and stamps it with the current version", () => {
    const f = parseFile(envelope("1.0"));
    expect(f.schemaVersion).toBe(SchemaVersion);
    expect(targetKeys(f.documents[0].blocks[0])).toEqual(["de", "nb"]);
  });

  it("keeps a minor of the current major as written", () => {
    expect(parseFile(envelope("2.7")).schemaVersion).toBe("2.7");
    expect(parseFile(envelope("1.4")).schemaVersion).toBe(SchemaVersion);
  });

  it("refuses an unknown major, an unknown kind, and a malformed version", () => {
    expect(() => parseFile(envelope("3.0"))).toThrow(/unsupported major schemaVersion 3/);
    expect(() => parseFile(envelope("2.0", "not-a-kbf"))).toThrow(/unexpected kind/);
    expect(() => parseFile(envelope("two"))).toThrow(/invalid schemaVersion/);
  });

  it("takes the root kind @neokapi/i18n-react 1.2.3 stamped", () => {
    expect(() => parseFile(envelope("1.0", "kapi-localization-format"))).not.toThrow();
  });
});

describe("the canonical form matches what Go core/kbf writes", () => {
  for (const [v1, v2] of [
    ["kapi-translated.kbf.json", "kapi-translated.schema2.kbf.json"],
    ["kapi-translated-placeholders.kbf.json", "kapi-translated-placeholders.schema2.kbf.json"],
  ]) {
    it(`writes ${v1} as Go writes it in the current schema`, () => {
      expect(decode(marshalFile(parseFile(goFixture(v1))))).toBe(goFixture(v2));
    });

    it(`writes ${v2} back byte for byte`, () => {
      expect(decode(marshalFile(parseFile(goFixture(v2))))).toBe(goFixture(v2));
    });
  }

  it("writes the source edition and the placeholder list on every block", () => {
    const out = decode(marshalBlock({ id: "b", type: "jsx:element" } as unknown as Block));
    expect(out).toContain('"editions": {\n    "": {\n      "runs": []\n    }\n  }');
    expect(out).toContain('"placeholders": []');
    expect(out).not.toContain("unlabelled");
  });

  it("orders keys and fields as Go does and leaves out what records nothing", () => {
    const b: Block = {
      id: "b",
      hash: "h",
      translatable: true,
      type: "jsx:element",
      editions: {
        nb: {
          runs: [{ text: "Logg inn" }],
          status: "translated",
          origin: { context_fingerprint: "cfp", kind: "ai" },
          score: 0.75,
          derived: { from: SourceEdition, rev: "r:0123456789abcdef" },
        },
        "en;channel=short": { runs: [{ text: "Sign" }], origin: {}, score: 0, status: "" },
        [SourceEdition]: { runs: [{ text: "Sign in" }] },
      },
      unlabelled: { runs: [{ text: "EMPTYLOC" }] },
      placeholders: [],
      properties: { file: "a.tsx", line: 1, component: "C", jsxPath: "p", element: "p" },
    };
    const out = decode(marshalBlock(b));
    const at = (s: string) => out.indexOf(s);
    expect(at('"": {')).toBeLessThan(at('"en;channel=short": {'));
    expect(at('"en;channel=short": {')).toBeLessThan(at('"nb": {'));
    const nb = out.slice(at('"nb": {'));
    const nbAt = (s: string) => nb.indexOf(s);
    expect(nbAt('"runs"')).toBeLessThan(nbAt('"status"'));
    expect(nbAt('"status"')).toBeLessThan(nbAt('"origin"'));
    expect(nbAt('"origin"')).toBeLessThan(nbAt('"score"'));
    expect(nbAt('"score"')).toBeLessThan(nbAt('"derived"'));
    expect(nb.indexOf('"kind"')).toBeLessThan(nb.indexOf('"context_fingerprint"'));
    expect(out).toContain(
      '"en;channel=short": {\n      "runs": [\n        {\n          "text": "Sign"\n        }\n      ]\n    }',
    );
    expect(at('"editions"')).toBeLessThan(at('"unlabelled"'));
    expect(at('"unlabelled"')).toBeLessThan(at('"placeholders"'));
  });

  it("stamps the version of the shape it writes", () => {
    const cases: Array<[string, string]> = [
      ["", SchemaVersion],
      ["1.0", SchemaVersion],
      ["1.4", SchemaVersion],
      ["two", SchemaVersion],
      ["2.0", "2.0"],
      ["2.7", "2.7"],
      ["3.0", SchemaVersion],
      ["2.0x", SchemaVersion],
    ];
    for (const [given, want] of cases) {
      const out = decode(
        marshalFile({
          schemaVersion: given,
          kind: "kapi-bundle",
          generator: { id: "x", version: "1" },
          project: { id: "p", sourceLocale: "en" },
          documents: [],
        }),
      );
      expect(out, given).toContain(`"schemaVersion": "${want}"`);
    }
  });

  it("writes a block still in the schema 1.0 shape as its editions", () => {
    const out = decode(marshalBlock(v1Block as unknown as Block));
    expect(out).toContain('"editions"');
    expect(out).not.toContain('"source"');
    expect(out).not.toContain('"targets"');
  });
});

describe("edition accessors", () => {
  const b = upgradeBlock(v1Block);

  it("read the source and every other edition by key", () => {
    expect(sourceRuns(b)).toEqual([{ text: "Sign in" }]);
    expect(editionRuns(b, "nb")).toEqual([{ text: "Logg inn" }]);
    expect(editionRuns(b, "fr")).toBeUndefined();
    expect(sourceRuns({ editions: {} })).toEqual([]);
  });

  it("tell a language key from a tone or channel key", () => {
    expect(isLanguageKey("nb")).toBe(true);
    expect(isLanguageKey("en;channel=short")).toBe(false);
    expect(isLanguageKey(SourceEdition)).toBe(false);
  });
});
