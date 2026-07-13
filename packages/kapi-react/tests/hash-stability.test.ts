/**
 * Hash-stability suite: the v2 key contract. A block's hash must
 * survive every source change that doesn't alter the block's own
 * meaning — layout refactors, wrapper renames, whitespace reflow,
 * attribute churn — and must change only when the text, the element
 * type, or the explicit context changes.
 *
 * These are the regression guard for the #1 pre-v2 toil generator
 * (ancestor-path keying orphaned translations on every refactor).
 */

import { describe, expect, it } from "vitest";

import { extractDocument } from "../src/extract/walker.ts";

function hashesOf(code: string, componentMap: Record<string, string> = {}): Map<string, string> {
  const doc = extractDocument(code, { filename: "Stable.tsx", componentMap });
  const out = new Map<string, string>();
  for (const b of doc?.blocks ?? []) {
    out.set(`${b.properties.jsxPath}:${flat(b)}`, b.hash);
  }
  return out;
}

function flat(b: { source: unknown[] }): string {
  return JSON.stringify(b.source);
}

function singleHash(code: string, componentMap: Record<string, string> = {}): string {
  const doc = extractDocument(code, { filename: "Stable.tsx", componentMap });
  expect(doc?.blocks.length).toBe(1);
  return doc!.blocks[0].hash;
}

describe("hash stability — refactors that must NOT change the key", () => {
  const base = singleHash("<p>Save changes</p>");

  it("wrapping in layout containers", () => {
    expect(singleHash("<div><p>Save changes</p></div>")).toBe(base);
    expect(singleHash("<div><section><main><p>Save changes</p></main></section></div>")).toBe(base);
  });

  it("wrapping in unmapped components", () => {
    expect(singleHash("<Card><CardBody><p>Save changes</p></CardBody></Card>")).toBe(base);
  });

  it("renaming an ancestor component", () => {
    const inCard = singleHash("<Card><p>Save changes</p></Card>");
    const inPanel = singleHash("<Panel><p>Save changes</p></Panel>");
    expect(inCard).toBe(base);
    expect(inPanel).toBe(base);
  });

  it("mapping an ANCESTOR component later (componentMap on wrappers is hash-neutral)", () => {
    const unmapped = singleHash("<Card><p>Save changes</p></Card>");
    const mapped = singleHash("<Card><p>Save changes</p></Card>", { Card: "div" });
    expect(mapped).toBe(unmapped);
  });

  it("whitespace reflow inside the element", () => {
    expect(singleHash("<p>\n      Save changes\n    </p>")).toBe(base);
    expect(singleHash("<p>Save   changes</p>")).toBe(base);
  });

  it("attribute churn on the element itself", () => {
    expect(singleHash('<p className="mt-2" data-testid="save-hint">Save changes</p>')).toBe(base);
  });

  it("moving the element between files (descriptor is file-independent)", () => {
    const other = extractDocument("<p>Save changes</p>", { filename: "Elsewhere.tsx" });
    expect(other!.blocks[0].hash).toBe(base);
  });

  it("sibling reordering", () => {
    const doc = extractDocument("<div><h1>Title</h1><p>Save changes</p></div>", {
      filename: "Stable.tsx",
    });
    const reordered = extractDocument("<div><p>Save changes</p><h1>Title</h1></div>", {
      filename: "Stable.tsx",
    });
    const find = (d: typeof doc, tag: string) =>
      d!.blocks.find((b) => b.properties.element === tag)!.hash;
    expect(find(reordered, "p")).toBe(find(doc, "p"));
    expect(find(reordered, "h1")).toBe(find(doc, "h1"));
  });

  it("inline structure is part of meaning but ancestor depth is not", () => {
    const rich = singleHash('<p>Click <a href="/x">here</a> now</p>');
    const wrapped = singleHash('<div><p>Click <a href="/x">here</a> now</p></div>');
    expect(wrapped).toBe(rich);
    // href churn on the inline child is also hash-neutral (the
    // template records {=m0}, not the attribute value).
    expect(singleHash('<p>Click <a href="/y" className="x">here</a> now</p>')).toBe(rich);
  });

  it("deep member chains don't collide or depend on order", () => {
    const ab = hashesOf("<p>{a.b.c} and {x.c}</p>");
    const ba = hashesOf("<p>{x.c} and {a.b.c}</p>");
    expect(ab.size).toBe(1);
    expect(ba.size).toBe(1);
    // Different templates (order differs) → hashes differ, but names
    // must be the full dotted paths, never a collapsed "c"/"c_2".
    const doc = extractDocument("<p>{a.b.c} and {x.c}</p>", { filename: "Stable.tsx" });
    const names = doc!.blocks[0].placeholders.map((p) => p.name);
    expect(names).toContain("a.b.c");
    expect(names).toContain("x.c");
  });
});

describe("hash stability — changes that MUST change the key", () => {
  const base = singleHash("<p>Save changes</p>");

  it("text edits", () => {
    expect(singleHash("<p>Save all changes</p>")).not.toBe(base);
  });

  it("element type changes", () => {
    expect(singleHash("<button>Save changes</button>")).not.toBe(base);
  });

  it("explicit context", () => {
    expect(singleHash('<p data-i18n-note="verb">Save changes</p>')).not.toBe(base);
  });

  it("mapping the element ITSELF changes its descriptor (warned by the plugin)", () => {
    const unmapped = singleHash("<Hint>Save changes</Hint>");
    const mapped = singleHash("<Hint>Save changes</Hint>", { Hint: "p" });
    expect(mapped).not.toBe(unmapped);
    expect(mapped).toBe(base);
  });
});
