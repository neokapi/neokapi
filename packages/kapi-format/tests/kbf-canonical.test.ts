import { describe, expect, it } from "vitest";

import type { Block, File } from "../src/index.ts";
import { marshalFile, newFile } from "../src/index.ts";

// Regression guards for the canonical-serialization parity bugs the KBF
// conformance suite (/kbf-tests) surfaced: the TypeScript mirror must match the
// canonical Go output (core/kbf) byte-for-byte.

function fileWith(block: Block): File {
  return newFile({
    generator: { id: "t", version: "1" },
    project: { id: "p", sourceLocale: "en" },
    documents: [{ id: "d", documentType: "jsx", path: "a.tsx", blocks: [block] }],
  });
}

const baseBlock: Omit<Block, "placeholders" | "preview"> = {
  id: "b",
  hash: "h",
  translatable: true,
  type: "jsx:element",
  source: [{ text: "hi" }],
  properties: {
    file: "a.tsx",
    line: 1,
    component: "C",
    jsxPath: "p",
    element: "p",
  },
};

describe("canonical KBF serialization parity with Go", () => {
  it("emits placeholders even when empty (required field, no omit)", () => {
    const out = new TextDecoder().decode(marshalFile(fileWith({ ...baseBlock, placeholders: [] })));
    expect(out).toContain('"placeholders": []');
  });

  it("sorts preview.sampleValues keys to match Go map ordering", () => {
    const out = new TextDecoder().decode(
      marshalFile(
        fileWith({
          ...baseBlock,
          placeholders: [],
          preview: {
            sampleValues: { label: "react", index: 3, deletable: true },
          },
        }),
      ),
    );
    // Go's encoding/json sorts map keys: deletable, index, label.
    const di = out.indexOf('"deletable"');
    const ii = out.indexOf('"index"');
    const li = out.indexOf('"label"');
    expect(di).toBeGreaterThan(-1);
    expect(di).toBeLessThan(ii);
    expect(ii).toBeLessThan(li);
  });

  it("emits the preview hints in struct-field order (storyId before sampleValues)", () => {
    const out = new TextDecoder().decode(
      marshalFile(
        fileWith({
          ...baseBlock,
          placeholders: [],
          preview: { storyId: "s--default", sampleValues: { count: 3 } },
        }),
      ),
    );
    expect(out.indexOf('"storyId"')).toBeLessThan(out.indexOf('"sampleValues"'));
  });
});

// The bytes core/kbf.Marshal writes for a block carrying two targets and their
// provenance (TestMarshalTargetOriginsParity in core/kbf/kbf_test.go asserts the
// same literal). Locale keys sort like a Go map's, each origin keeps Go's
// struct-field order, and an unset field is left out as omitempty leaves it
// out: an empty origin is `{}`.
const targetOriginsGolden = `{
  "schemaVersion": "1.0",
  "kind": "kapi-bundle",
  "generator": {
    "id": "t",
    "version": "1"
  },
  "project": {
    "id": "p",
    "sourceLocale": "en"
  },
  "documents": [
    {
      "id": "d",
      "documentType": "jsx",
      "path": "a.tsx",
      "blocks": [
        {
          "id": "b",
          "hash": "h",
          "translatable": true,
          "type": "jsx:element",
          "source": [
            {
              "text": "hi"
            }
          ],
          "targets": {
            "fr": [
              {
                "text": "salut"
              }
            ],
            "qps": [
              {
                "text": "ĥî"
              }
            ]
          },
          "targetOrigins": {
            "de": {},
            "fr": {
              "kind": "ai",
              "engine": "claude",
              "reference": "batch-1",
              "confidence": 0.5,
              "profile": "voice",
              "profile_version": "3",
              "context_fingerprint": "abc"
            },
            "qps": {
              "kind": "mt",
              "tool": "pseudo-translate",
              "timestamp": "2026-09-04T00:00:00Z"
            }
          },
          "placeholders": [],
          "properties": {
            "file": "a.tsx",
            "line": 1,
            "component": "C",
            "jsxPath": "p",
            "element": "p"
          }
        }
      ]
    }
  ]
}
`;

describe("targetOrigins parity with Go", () => {
  it("marshals targets and their origins byte for byte as core/kbf does", () => {
    const out = new TextDecoder().decode(
      marshalFile(
        fileWith({
          ...baseBlock,
          placeholders: [],
          targets: { qps: [{ text: "ĥî" }], fr: [{ text: "salut" }] },
          targetOrigins: {
            qps: { kind: "mt", tool: "pseudo-translate", timestamp: "2026-09-04T00:00:00Z" },
            fr: {
              kind: "ai",
              engine: "claude",
              confidence: 0.5,
              profile: "voice",
              profile_version: "3",
              context_fingerprint: "abc",
              reference: "batch-1",
            },
            de: { kind: "", engine: "", confidence: 0 },
          },
        }),
      ),
    );
    expect(out).toBe(targetOriginsGolden);
  });

  it("omits targetOrigins when absent or empty, as omitempty does", () => {
    for (const targetOrigins of [undefined, {}]) {
      const out = new TextDecoder().decode(
        marshalFile(fileWith({ ...baseBlock, placeholders: [], targetOrigins })),
      );
      expect(out).not.toContain("targetOrigins");
    }
  });
});
