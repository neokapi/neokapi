/**
 * JSX inside an expression container of a translated element.
 *
 * A block consumes its inline children into one flat template, but an
 * expression container is different: the whole expression travels into
 * the call as a param and is spliced back verbatim. So `<span>a note</span>`
 * inside `{cond && …}` is still ordinary JSX, and it needs a call of its
 * own to be translated at all.
 *
 * The extractor has always emitted a block for it. The transform used to
 * stop at the parent, so every such key was compiled into the runtime
 * dictionary and looked up by nobody: a translator was asked for a string
 * the reader only ever saw in the source language. The two now descend the
 * same containers, and `renderOps` composes the inner call into the source
 * the outer call splices (#2522).
 */

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseSync } from "@swc/core";
import { afterEach, describe, expect, it } from "vitest";

import type { Block } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { transform } from "../src/plugin/transform.ts";
import type { PluginOptions } from "../src/types.ts";

const tmpDirs: string[] = [];

afterEach(() => {
  for (const dir of tmpDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function dictDir(locale: string, entries: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-conditional-"));
  tmpDirs.push(dir);
  writeFileSync(join(dir, `${locale}.json`), JSON.stringify(entries));
  return dir;
}

function blocks(code: string, opts: Partial<PluginOptions> = {}): Block[] {
  return extractDocument(code, { filename: "Test.tsx", ...opts })?.blocks ?? [];
}

function runtime(code: string, opts: Partial<PluginOptions> = {}) {
  return transform(code, "Test.tsx", { mode: "runtime", onWarning: () => {}, ...opts });
}

function inline(code: string, dir: string, opts: Partial<PluginOptions> = {}): string {
  const out = transform(code, "Test.tsx", {
    locale: "qps",
    translationsDir: dir,
    strict: false,
    onWarning: () => {},
    ...opts,
  });
  return out?.code ?? "";
}

/** Assert the transformed output is still parseable TSX. */
function expectParses(code: string): void {
  expect(() => parseSync(code, { syntax: "typescript", tsx: true })).not.toThrow();
}

describe("conditional JSX gets the call its block needs", () => {
  const SHAPES: ReadonlyArray<{ name: string; code: string; inner: readonly string[] }> = [
    {
      name: "logical and",
      code: "<p>Saved {cond && <span>a note</span>} now</p>",
      inner: ["a note"],
    },
    {
      name: "ternary, both branches",
      code: "<p>Saved {cond ? <span>yes note</span> : <span>no note</span>}</p>",
      inner: ["yes note", "no note"],
    },
    {
      name: "logical or",
      code: "<p>Saved {cond || <span>fallback text</span>}</p>",
      inner: ["fallback text"],
    },
    {
      name: "nullish coalescing",
      code: "<p>Saved {cond ?? <span>nullish text</span>}</p>",
      inner: ["nullish text"],
    },
    {
      name: "array map",
      code: "<p>Saved {items.map((i) => <span key={i}>each item</span>)}</p>",
      inner: ["each item"],
    },
    {
      name: "two conditionals in one sentence",
      code: "<p>A {x && <span>one</span>} B {y && <span>two</span>} C</p>",
      inner: ["one", "two"],
    },
    {
      name: "conditional beside a protected code span",
      code: "<p>Press <kbd>K</kbd> {cond && <span>a note</span>} to search</p>",
      inner: ["a note"],
    },
    {
      name: "conditional under a fragment block",
      code: "<><span>Saved</span> {cond && <span>a note</span>}</>",
      inner: ["a note"],
    },
    {
      name: "conditional nested two deep",
      code: "<p>Saved {a && <span>outer {b && <b>inner</b>}</span>}</p>",
      inner: ["outer {=m0}", "inner"],
    },
  ];

  for (const { name, code, inner } of SHAPES) {
    it(`${name}: every extracted hash reaches the transform`, () => {
      const extracted = blocks(code).map((b) => b.hash);
      const out = runtime(code);
      expect(out).not.toBeNull();
      expect(extracted.length).toBeGreaterThan(1);
      for (const hash of extracted) {
        expect(out?.hashes, `hash ${hash} never reaches the runtime`).toContain(hash);
      }
      expectParses(out?.code as string);
    });

    it(`${name}: the inner text is a call, not a literal`, () => {
      const out = runtime(code)?.code as string;
      for (const text of inner) {
        expect(out).toContain(JSON.stringify(text));
        // The inner text appears only as a call argument, never as
        // JSX the reader would see in the source language.
        expect(out).not.toContain(`>${text}<`);
      }
    });
  }
});

describe("the inner call sits inside the param the outer call splices", () => {
  const CODE = "<p>Saved {cond && <span>a note</span>} now</p>";
  const OUTER = hashKey("Saved {=m0} now", "p");
  const INNER = hashKey("a note", "span");

  it("runtime mode nests the inner __t inside the element param", () => {
    const out = runtime(CODE)?.code as string;
    expect(out).toContain(`__tx("${OUTER}"`);
    expect(out).toContain(`{ "=m0": cond && <span>{__t("${INNER}", "a note")}</span> }`);
  });

  it("inline mode bakes both translations", () => {
    const dir = dictDir("qps", { [OUTER]: "Šàvéð {=m0} nöŵ", [INNER]: "à nöţé" });
    const out = inline(CODE, dir);
    expect(out).toContain("Šàvéð");
    expect(out).toContain("<span>à nöţé</span>");
    expect(out).not.toContain("a note");
    expectParses(out);
  });

  it("inline mode with only the outer translation keeps the inner source", () => {
    const dir = dictDir("qps", { [OUTER]: "Šàvéð {=m0} nöŵ" });
    const out = inline(CODE, dir);
    expect(out).toContain("Šàvéð");
    expect(out).toContain("<span>a note</span>");
    expectParses(out);
  });

  it("inline mode drops the whole conditional when the translation drops the token", () => {
    const dir = dictDir("qps", { [OUTER]: "Šàvéð nöŵ", [INNER]: "à nöţé" });
    const out = inline(CODE, dir);
    expect(out).toContain("Šàvéð nöŵ");
    expect(out).not.toContain("cond &&");
    expectParses(out);
  });
});

describe("what else travels inside a conditional", () => {
  it("a translatable attribute on an element with no children", () => {
    const code = '<p>Saved {cond && <button aria-label="Close it" />}</p>';
    const extracted = blocks(code);
    expect(extracted.map((b) => b.type)).toContain("jsx:attribute");
    const out = runtime(code);
    for (const b of extracted) expect(out?.hashes).toContain(b.hash);
    expect(out?.code).toContain(`aria-label={__t("${hashKey("Close it", "button[aria-label]")}"`);
  });

  it("an attribute and a text block on the same element", () => {
    const code = '<p>Saved {cond && <span title="Tip text">a note</span>}</p>';
    const extracted = blocks(code);
    expect(extracted).toHaveLength(3);
    const out = runtime(code);
    for (const b of extracted) expect(out?.hashes).toContain(b.hash);
    expectParses(out?.code as string);
  });

  it("a t() call, which used to be skipped as covered by the block op", () => {
    const code = [
      "import { t } from '@neokapi/i18n-react/runtime';",
      'const x = <p>Saved {cond && t("a note")}</p>;',
    ].join("\n");
    const extracted = blocks(code);
    const out = runtime(code);
    for (const b of extracted) expect(out?.hashes).toContain(b.hash);
    const tBlock = extracted.find((b) => b.type === "js:t");
    expect(tBlock).toBeDefined();
    expect(out?.code).toContain(`__t("${tBlock?.hash}", "a note")`);
    expectParses(out?.code as string);
  });

  it("a fragment, which keeps its own descriptor", () => {
    const code = "<p>Saved {cond && <><span>frag one</span></>}</p>";
    const extracted = blocks(code);
    expect(extracted.map((b) => b.properties?.jsxPath)).toContain("fragment");
    const out = runtime(code);
    for (const b of extracted) expect(out?.hashes).toContain(b.hash);
    expectParses(out?.code as string);
  });

  it("JSX in a prop of an element that was itself consumed", () => {
    const code = "<div actions={<Button>Go now</Button>}>Some text here</div>";
    const extracted = blocks(code);
    expect(extracted).toHaveLength(2);
    const out = runtime(code);
    for (const b of extracted) expect(out?.hashes).toContain(b.hash);
    expect(out?.code).toContain(
      `<Button>{__t("${hashKey("Go now", "Button")}", "Go now")}</Button>`,
    );
  });
});

describe("blocks the parent really does consume stay consumed", () => {
  it("a paired inline child produces one block and one call", () => {
    const code = '<p>Click <a href="/x">here</a> to continue.</p>';
    expect(blocks(code)).toHaveLength(1);
    const out = runtime(code);
    expect(out?.hashes).toHaveLength(1);
    expect(out?.code).toContain('{ "=m0": <a href="/x">here</a> }');
  });

  it("a t() call in the flat template is not rewritten twice", () => {
    const code = [
      "import { t } from '@neokapi/i18n-react/runtime';",
      'const x = <p>Hello {t("world")}</p>;',
    ].join("\n");
    const out = runtime(code)?.code as string;
    const calls = [...out.matchAll(/__t\("/g)].length;
    // The paragraph's own call plus the t() call inside its param.
    expect(calls).toBeGreaterThan(0);
    expectParses(out);
  });
});
