/**
 * End-to-end round-trip for `<Plural>` authored by the developer:
 *
 *   JSX source
 *     → kapi-react extract (walker emits PluralRun)
 *     → writes a .klf file
 *     → kapi-react compile (flattens target runs into runtime dict)
 *
 * We assert that the ICU template in the compiled dict matches what
 * the runtime `resolveICU` path is known to accept. The hash is the
 * same one `plugin/transform.ts` would stamp into `__tx(hash, ...)`
 * at build time (covered by the hash-parity suite).
 */

import { describe, expect, it } from "vitest";

import type { PluralRunWrapper } from "@neokapi/kapi-format";
import { newFile, marshalFile } from "@neokapi/kapi-format";

import { runCompile } from "../src/commands/compile.ts";
import { extractDocument } from "../src/extract/index.ts";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const SHOPPING_CART = `
export default function ShoppingCart({ items }) {
  return (
    <p>
      <Plural count={items.length}>
        <Zero>Your cart is empty</Zero>
        <One>1 item in your cart</One>
        <Other>
          <strong>{items.length}</strong> items in your cart
        </Other>
      </Plural>
    </p>
  );
}
`;

function tempDir(prefix: string): string {
  return mkdtempSync(join(tmpdir(), `${prefix}-`));
}

describe("plural round-trip", () => {
  it("extracts ShoppingCart into a .klf with a PluralRun, compiles it to an ICU runtime dict", async () => {
    // 1. Extract
    const doc = extractDocument(SHOPPING_CART, { filename: "ShoppingCart.tsx" });
    expect(doc).toBeTruthy();
    const block = doc!.blocks[0];
    expect(block).toBeTruthy();
    expect(block.properties.component).toBe("ShoppingCart");

    // The block's source is a single PluralRun carrying three typed
    // forms — zero and one are plain text, other has a jsx:element
    // placeholder for <strong>{items.length}</strong>.
    expect(block.source).toHaveLength(1);
    const wrapper = block.source[0] as PluralRunWrapper;
    expect(wrapper.plural).toBeTruthy();
    expect(wrapper.plural.pivot).toBe("items.length");
    expect(Object.keys(wrapper.plural.forms).sort()).toEqual(["one", "other", "zero"]);

    // 2. Stamp a target on the block simulating a pseudo-translate /
    // TMS that populated block.targets[qps]. Here we populate the
    // target with a duplicated PluralRun (wrap for a pseudo locale).
    const targetPlural: PluralRunWrapper = {
      plural: {
        pivot: "items.length",
        forms: {
          zero: [{ text: "[Your cart is empty]" }],
          one: [{ text: "[1 item in your cart]" }],
          other: wrapper.plural.forms.other ?? [],
        },
      },
    };
    block.targets = { qps: [targetPlural] };

    // 3. Write to a .klf file.
    const dir = tempDir("plural-roundtrip");
    const klfDir = join(dir, "i18n");
    mkdirSync(klfDir, { recursive: true });
    const klfPath = join(klfDir, "ShoppingCart.klf");
    writeFileSync(
      klfPath,
      marshalFile(
        newFile({
          generator: { id: "@neokapi/kapi-react", version: "0.1.0" },
          project: { id: "test", sourceLocale: "en" },
          documents: [doc!],
        }),
      ),
    );

    // 4. Compile the .klf dir → runtime dict.
    const outDir = join(dir, "translations");
    await runCompile([klfDir, "--locale", "qps", "--out", outDir]);

    const qps = JSON.parse(readFileSync(join(outDir, "qps.json"), "utf-8"));

    // 5. Assert the compiled entry is valid ICU that the runtime's
    //    resolveICU can dispatch: `{pivot, plural, zero {…} one {…}
    //    other {…}}` with paired `{=mN}` … `{/=mN}` markers around the
    //    inline element's inner content.
    const entry = qps[block.hash];
    expect(entry).toBeTruthy();
    expect(entry).toContain("{items.length, plural,");
    expect(entry).toContain("zero {[Your cart is empty]}");
    expect(entry).toContain("one {[1 item in your cart]}");
    expect(entry).toMatch(/other \{\{=m\d+\}\{items\.length\}\{\/=m\d+\} items in your cart\}/);
  });
});
