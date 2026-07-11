/**
 * Fragment-rooted blocks: `<>Save changes</>` extracts and
 * transforms like a promoted container, under the fixed `fragment`
 * descriptor. Fragments with block-level children stay untouched
 * (their inner elements surface as their own blocks).
 */

import { parseSync } from "@swc/core";
import { describe, expect, it } from "vitest";

import { extractDocument } from "../src/extract/walker.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { transform } from "../src/plugin/transform.ts";

function extract(code: string) {
  return extractDocument(code, { filename: "Frag.tsx" });
}

function rt(code: string): string | null {
  return transform(code, "Frag.tsx", { mode: "runtime" })?.code ?? null;
}

describe("fragment extraction", () => {
  it("extracts text directly inside a fragment", () => {
    const doc = extract("const x = <>Save changes</>;");
    expect(doc?.blocks.length).toBe(1);
    const block = doc!.blocks[0];
    expect(block.properties.element).toBe("fragment");
    expect(block.hash).toBe(hashKey("Save changes", "fragment"));
  });

  it("extracts mixed text + params + inline elements in a fragment", () => {
    const doc = extract("const x = <>Welcome <strong>{user.name}</strong>!</>;");
    expect(doc?.blocks.length).toBe(1);
    expect(doc!.blocks[0].hash).toBe(hashKey("Welcome {=m0}{user.name}{/=m0}!", "fragment"));
  });

  it("leaves fragments with block-level children to their inner blocks", () => {
    const doc = extract("const x = <><h1>Title</h1><p>Body</p></>;");
    const paths = doc!.blocks.map((b) => b.properties.jsxPath);
    expect(paths).toEqual(["h1", "p"]);
  });

  it('respects ancestor translate="no"', () => {
    const doc = extract('const x = <div translate="no">{cond && <>skip me</>}</div>;');
    expect(doc).toBeNull();
  });
});

describe("fragment transform (runtime mode)", () => {
  it("wraps fragment text in a __t call", () => {
    const out = rt("const x = <>Save changes</>;");
    expect(out).toContain("__t(");
    expect(out).toContain(`"${hashKey("Save changes", "fragment")}"`);
    expect(() => parseSync(out as string, { syntax: "typescript", tsx: true })).not.toThrow();
  });

  it("extract and transform agree on fragment hashes", () => {
    const code = "const x = <>Welcome <strong>{user.name}</strong>!</>;";
    const doc = extract(code);
    const out = rt(code);
    expect(out).toContain(`"${doc!.blocks[0].hash}"`);
  });

  it("does not touch fragments with block-level children", () => {
    const out = rt("const x = <><h1>Title</h1><p>Body</p></>;");
    // Inner elements each get their own call; the fragment itself is untouched.
    expect(out).toContain("<>");
    expect(out?.match(/__t\(/g)?.length).toBe(2);
  });
});

describe("consumed fragments with conditional JSX (TMFacetSidebar regression)", () => {
  it("emits ONE op for the whole fragment — never nested ops inside its range", () => {
    const code = `const x = (
      <>
        {hiddenCount} more
        {hiddenSelected > 0 && (
          <span className="ml-1">({hiddenSelected} selected)</span>
        )}
      </>
    );`;
    // Pre-fix this threw "[neokapi] overlapping transform ops".
    const out = rt(code);
    expect(out).toContain("__tx(");
    expect(() => parseSync(out as string, { syntax: "typescript", tsx: true })).not.toThrow();
  });
});
