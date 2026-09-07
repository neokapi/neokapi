/**
 * Translatable attributes on an inline child of a translated block.
 *
 * A block folds its inline children into one flat template, so the words
 * between an `<abbr>`'s tags belong to the sentence. Its `title` does not:
 * the call site splices the element's own source into the param, and that
 * source carries the attribute. So the attribute is extracted and served
 * where it stands, and the sentence around it keeps every word.
 *
 * Before this, the walker stopped visiting once a parent emitted, and the
 * attribute went quiet: `<p>Use <abbr title="Content memory">CM</abbr> for
 * that.</p>` catalogued the sentence and lost the title. #2524 worked around
 * the control half by declining to fold a control carrying one, which traded
 * the prose for the string inside it. Both are fixed here (#2523).
 */

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseSync } from "@swc/core";
import { afterEach, describe, expect, it } from "vitest";

import type { Block, Run, TextRun } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { transform } from "../src/plugin/transform.ts";
import type { PluginOptions } from "../src/types.ts";

const tmpDirs: string[] = [];

afterEach(() => {
  for (const dir of tmpDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function dictDir(locale: string, entries: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-inline-attr-"));
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

/** The text a translator may edit in one block, in order. */
function editable(block: Block): string[] {
  return block.source
    .filter((r: Run): r is TextRun => "text" in r && !(r as TextRun).noTranslate)
    .map((r) => r.text);
}

function expectParses(code: string): void {
  expect(() => parseSync(code, { syntax: "typescript", tsx: true })).not.toThrow();
}

/** Every extracted hash must reach a call in the transformed output. */
function expectParity(code: string, opts: Partial<PluginOptions> = {}): Block[] {
  const found = blocks(code, opts);
  const out = runtime(code, opts);
  for (const b of found) {
    expect(out?.hashes, `hash ${b.hash} (${b.type}) never reaches the runtime`).toContain(b.hash);
  }
  expect(out?.hashes?.length).toBe(found.length);
  expectParses(out?.code as string);
  return found;
}

describe("an inline child's attributes travel with it", () => {
  it("an abbreviation keeps its title and the sentence keeps its words", () => {
    const code = '<p>Use <abbr title="Content memory">CM</abbr> for that.</p>';
    const found = expectParity(code);
    expect(found.map((b) => b.type)).toEqual(["jsx:element", "jsx:attribute"]);
    expect(editable(found[0])).toEqual(["Use ", "CM", " for that."]);
    expect(found[1].properties?.jsxPath).toBe("abbr[title]");
    expect(runtime(code)?.code).toContain('title={__t("');
  });

  it("a childless component keeps its label prop", () => {
    const found = expectParity('<p>Pick a <Badge label="tag" /> here.</p>');
    expect(found.map((b) => b.properties?.jsxPath)).toEqual(["p", "Badge[label]"]);
  });

  it("an image in a sentence keeps both the prose and the alt", () => {
    const code = '<p>See <img alt="the chart" src="/x.png" /> above.</p>';
    const found = expectParity(code);
    expect(editable(found[0])).toEqual(["See ", " above."]);
    expect(found[1].properties?.jsxPath).toBe("img[alt]");
    expect(runtime(code)?.code).toContain("alt={__t(");
  });

  it("an icon button in a sentence keeps both the prose and the aria-label", () => {
    const code = '<p>Press <button aria-label="Run the demo"><Play /></button> to start.</p>';
    const found = expectParity(code);
    expect(editable(found[0])).toEqual(["Press ", " to start."]);
    expect(found[1].properties?.jsxPath).toBe("button[aria-label]");
  });

  it("an input's placeholder inside its label", () => {
    const found = expectParity('<label>Name <input placeholder="Jane" /></label>');
    expect(found.map((b) => b.properties?.jsxPath)).toEqual(["label", "input[placeholder]"]);
  });

  it("two levels of inline nesting each keep their attribute", () => {
    const found = expectParity('<p>See <a href="/x" title="Tip"><img alt="chart" /></a> now.</p>');
    expect(found.map((b) => b.properties?.jsxPath)).toEqual(["p", "a[title]", "img[alt]"]);
  });

  it("a string-literal ternary attribute extracts a block per branch", () => {
    const code = '<p>Use <abbr title={c ? "A one" : "B two"}>CM</abbr> here.</p>';
    const found = expectParity(code);
    expect(found.map((b) => b.properties?.jsxPath)).toEqual([
      "p",
      "abbr[title::0]",
      "abbr[title::1]",
    ]);
    const out = runtime(code)?.code as string;
    expect(out).toContain("title={c ? __t(");
  });

  it("a conditional under an inline child is still its own block", () => {
    const found = expectParity("<p>Text <strong>bold {cond && <b>inner</b>}</strong> end.</p>");
    expect(found.map((b) => b.properties?.jsxPath)).toEqual(["p", "b"]);
  });
});

describe("what the attribute walk still leaves alone", () => {
  it('a translate="no" island keeps its attributes out of the catalog', () => {
    const found = blocks('<p>Path <span translate="no" title="Tip">{path}</span> ok.</p>');
    expect(found.map((b) => b.properties?.jsxPath)).toEqual(["p"]);
    expect(
      runtime('<p>Path <span translate="no" title="Tip">{path}</span> ok.</p>')?.code,
    ).toContain('title="Tip"');
  });

  it("machine-facing props on an inline child stay out", () => {
    const found = blocks('<p>Press <button type="submit" name="go">Go</button> now.</p>');
    expect(found).toHaveLength(1);
  });

  it("a convention prop on a plain element stays out", () => {
    const found = blocks('<p>Pick a <span label="draft-pending">tag</span> here.</p>');
    expect(found).toHaveLength(1);
  });

  it("the sentence's own words are not duplicated into a second block", () => {
    const found = blocks('<p>Use <abbr title="Content memory">CM</abbr> for that.</p>');
    expect(found.filter((b) => b.type === "jsx:element")).toHaveLength(1);
  });
});

describe("the attribute key does not depend on the sentence around it", () => {
  const STANDALONE = '<abbr title="Content memory">CM</abbr>';
  const IN_PROSE = '<p>Use <abbr title="Content memory">CM</abbr> for that.</p>';

  it("is the same hash standing alone and folded into a paragraph", () => {
    const alone = blocks(STANDALONE).find((b) => b.type === "jsx:attribute");
    const folded = blocks(IN_PROSE).find((b) => b.type === "jsx:attribute");
    expect(alone?.hash).toBe(folded?.hash);
    expect(alone?.hash).toBe(hashKey("Content memory", "abbr[title]"));
  });
});

describe("the translated attribute reaches the output in both modes", () => {
  const CODE = '<p>Use <abbr title="Content memory">CM</abbr> for that.</p>';
  const BLOCK = hashKey("Use {=m0}CM{/=m0} for that.", "p");
  const ATTR = hashKey("Content memory", "abbr[title]");

  it("runtime mode weaves the attribute call into the element param", () => {
    const out = runtime(CODE)?.code as string;
    expect(out).toContain(`{ "=m0": <abbr title={__t("${ATTR}", "Content memory")}>CM</abbr> }`);
  });

  it("inline mode bakes both the sentence and the attribute", () => {
    const dir = dictDir("qps", {
      [BLOCK]: "Ûšé {=m0}ÇM{/=m0} förthàţ.",
      [ATTR]: "Çönţéñţ mémöŕý",
    });
    const out =
      transform(CODE, "Test.tsx", {
        locale: "qps",
        translationsDir: dir,
        strict: false,
        onWarning: () => {},
      })?.code ?? "";
    expect(out).toContain('<abbr title="Çönţéñţ mémöŕý">');
    expect(out).toContain("ÇM");
    expect(out).not.toContain("Content memory");
    expectParses(out);
  });

  it("inline mode with only the attribute translated keeps the source sentence", () => {
    const dir = dictDir("qps", { [ATTR]: "Çönţéñţ mémöŕý" });
    const out =
      transform(CODE, "Test.tsx", {
        locale: "qps",
        translationsDir: dir,
        strict: false,
        onWarning: () => {},
      })?.code ?? "";
    expect(out).toContain('<abbr title="Çönţéñţ mémöŕý">');
    expect(out).toContain("Use ");
    expectParses(out);
  });
});
