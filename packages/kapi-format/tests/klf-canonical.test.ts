import { describe, expect, it } from "vitest";

import type { Block, File } from "../src/index.ts";
import { marshalFile, newFile } from "../src/index.ts";

// Regression guards for the canonical-serialization parity bugs the KLF
// conformance suite (/klf-tests) surfaced: the TypeScript mirror must match the
// canonical Go output (core/klf) byte-for-byte.

function fileWith(block: Block): File {
  return newFile({
    generator: { id: "t", version: "1" },
    project: { id: "p", sourceLocale: "en" },
    documents: [
      { id: "d", documentType: "jsx", path: "a.tsx", blocks: [block] },
    ],
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

describe("canonical KLF serialization parity with Go", () => {
  it("emits placeholders even when empty (required field, no omit)", () => {
    const out = new TextDecoder().decode(
      marshalFile(fileWith({ ...baseBlock, placeholders: [] })),
    );
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
    expect(out.indexOf('"storyId"')).toBeLessThan(
      out.indexOf('"sampleValues"'),
    );
  });
});
