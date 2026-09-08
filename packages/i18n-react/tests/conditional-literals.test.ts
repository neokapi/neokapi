/**
 * String literals in the branches of a conditional in JSX children position.
 *
 * `<Button>{saving ? "Saving..." : "Save"}</Button>` renders one of two
 * sentences, and neither is JSX text. Extraction read the whole conditional as
 * one opaque placeholder, so both strings shipped in English and an author who
 * wanted them translated had to rewrite the ternary as two conditional
 * elements (#2581). The same held for `{cond && "Folder moved"}` and
 * `{label || "Untitled"}`.
 *
 * Each literal branch is now a block of its own, addressed by its slot
 * position in the way the ternary attribute path already addresses `::0` and
 * `::1`, and the transform rewrites the literal in place. Both sides read the
 * branches through `conditionalLiteralBranches`, so neither can name a slot the
 * other does not.
 */

import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { parseSync } from "@swc/core";
import { describe, expect, it } from "vitest";

import type { Block, Document, TextRun } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { transform } from "../src/plugin/transform.ts";
import { CONTEXT_SEPARATOR, type PluginOptions } from "../src/types.ts";

function extract(code: string): Document | null {
  return extractDocument(code, { filename: "Test.tsx" });
}

function blocks(code: string): Block[] {
  return extract(code)?.blocks ?? [];
}

/** The branch blocks only, in emission order: context, then text. */
function branches(code: string): Array<[string, string]> {
  return blocks(code)
    .filter((b) => /::\d+$/.test(b.properties.jsxPath))
    .map((b) => [b.properties.jsxPath, (b.source[0] as TextRun).text]);
}

function t(code: string, options: Partial<PluginOptions> = {}): string {
  const out = transform(code, "Test.tsx", { mode: "runtime", onWarning: () => {}, ...options });
  expect(out?.code, "expected the transform to rewrite this file").toBeTruthy();
  return out?.code ?? "";
}

function expectParses(code: string): void {
  expect(() => parseSync(code, { syntax: "typescript", tsx: true })).not.toThrow();
}

function dictDir(locale: string, entries: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-branch-"));
  writeFileSync(join(dir, `${locale}.json`), JSON.stringify(entries));
  return dir;
}

const SAVE = `<Button>{saving ? "Saving..." : "Save"}</Button>`;

describe("extraction of a conditional's literal branches", () => {
  it("gives a ternary's two branches a block each", () => {
    expect(branches(SAVE)).toEqual([
      ["Button::0", "Saving..."],
      ["Button::1", "Save"],
    ]);
  });

  it("takes the right side of a logical operator", () => {
    expect(branches(`<div>{cond && "Folder moved"}</div>`)).toEqual([["div::0", "Folder moved"]]);
    expect(branches(`<div>{label || "Untitled"}</div>`)).toEqual([["div::0", "Untitled"]]);
    expect(branches(`<div>{label ?? "Untitled"}</div>`)).toEqual([["div::0", "Untitled"]]);
  });

  it("leaves the test of a conditional alone", () => {
    expect(branches(`<div>{mode === "draft" && "Not published yet"}</div>`)).toEqual([
      ["div::0", "Not published yet"],
    ]);
  });

  it("numbers a nested ternary's branches in source order", () => {
    expect(branches(`<p>Saved {a ? "A" : b ? "B" : "C"}</p>`)).toEqual([
      ["p::0", "A"],
      ["p::1", "B"],
      ["p::2", "C"],
    ]);
  });

  it("keeps a slot for a branch that is not a literal", () => {
    // Turning `someVar` into a literal later must not move "D" off ::1.
    expect(branches(`<div>{a ? someVar : "D"}</div>`)).toEqual([["div::1", "D"]]);
  });

  it("keeps a slot for an empty branch", () => {
    expect(branches(`<div>{cond ? "" : "Save"}</div>`)).toEqual([["div::1", "Save"]]);
  });

  it("reads through parentheses", () => {
    expect(branches(`<div>{a ? ("Paren") : "B"}</div>`)).toEqual([
      ["div::0", "Paren"],
      ["div::1", "B"],
    ]);
  });

  it("takes a fragment's children too", () => {
    expect(branches(`<>{a ? "A" : "B"}</>`)).toEqual([
      ["fragment::0", "A"],
      ["fragment::1", "B"],
    ]);
  });

  it("describes the branch as element content", () => {
    const block = blocks(SAVE).find((b) => b.properties.jsxPath === "Button::0");
    expect(block).toMatchObject({
      type: "jsx:element",
      translatable: true,
      source: [{ text: "Saving..." }],
      placeholders: [],
      properties: { file: "Test.tsx", element: "Button", jsxPath: "Button::0" },
    });
  });

  it("carries the element's translator note into the branch descriptor", () => {
    const code = `<div data-i18n-note="status line">{a ? "Open" : "Closed"}</div>`;
    const found = blocks(code).find((b) => b.properties.jsxPath === "div::0");
    expect(found?.properties.locNote).toBe("status line");
    expect(found?.hash).toBe(hashKey("Open", `div::0${CONTEXT_SEPARATOR}status line`));
  });

  it("emits nothing for an element that holds no prose", () => {
    expect(branches(`<code>{a ? "x = 1" : "y = 2"}</code>`)).toEqual([]);
    expect(branches(`<script>{a ? "x" : "y"}</script>`)).toEqual([]);
  });

  it("respects translate=no on the element and on an ancestor", () => {
    expect(branches(`<div translate="no">{a ? "A" : "B"}</div>`)).toEqual([]);
    expect(branches(`<section translate="no"><p>{a ? "A" : "B"}</p></section>`)).toEqual([]);
  });

  it("leaves an attribute ternary to the attribute path", () => {
    const found = blocks(`<div title={a ? "A" : "B"}>Body</div>`).map((b) => b.properties.jsxPath);
    expect(found).toContain("div[title::0]");
    expect(found).not.toContain("div::0");
  });

  it("emits nothing for a conditional with no literal branch", () => {
    expect(branches(`<div>{cond ? <A /> : <B />}</div>`)).toEqual([]);
    expect(branches(`<div>{cond ? one : two}</div>`)).toEqual([]);
  });
});

describe("the transform rewrites each branch in place", () => {
  it("wraps both branches and leaves the condition alone", () => {
    const out = t(SAVE);
    expect(out).toContain(
      `{saving ? __t("${hashKey("Saving...", "Button::0")}", "Saving...") : __t("${hashKey("Save", "Button::1")}", "Save")}`,
    );
    expectParses(out);
  });

  it("wraps the right side of a logical operator", () => {
    const out = t(`<div>{cond && "Folder moved"}</div>`);
    expect(out).toContain(`{cond && __t("${hashKey("Folder moved", "div::0")}", "Folder moved")}`);
    expectParses(out);
  });

  it("imports the string helper", () => {
    expect(t(SAVE)).toContain("import { __t }");
  });

  it("composes with the enclosing element's own block", () => {
    const out = t(`<div>Saved {cond ? "now" : "later"} today</div>`);
    expect(out).toContain(`__tx("${hashKey("Saved {value} today", "div")}"`);
    expect(out).toContain(`__t("${hashKey("now", "div::0")}", "now")`);
    expect(out).toContain(`__t("${hashKey("later", "div::1")}", "later")`);
    expectParses(out);
  });

  it("composes inside a plural form", () => {
    // The form element is the branch's context, the way it is for any other
    // element that holds a conditional.
    const out = t(
      `<p><Plural count={n}><One>{a ? "one item" : "single"}</One><Other>many</Other></Plural></p>`,
    );
    expect(out).toContain(`__t("${hashKey("one item", "One::0")}", "one item")`);
    expectParses(out);
  });

  it("keeps a JSX branch beside a literal branch", () => {
    const out = t(`<div>Prefix {a ? "A" : <span>B here</span>} tail</div>`);
    expect(out).toContain(`__t("${hashKey("A", "div::0")}", "A")`);
    expect(out).toContain(`__t("${hashKey("B here", "span")}", "B here")`);
    expectParses(out);
  });

  it("keeps the parentheses around a parenthesised branch", () => {
    const out = t(`<div>{a ? ("Paren") : "B"}</div>`);
    expect(out).toContain(`(__t("${hashKey("Paren", "div::0")}", "Paren"))`);
    expectParses(out);
  });

  it("rewrites every call site when two conditionals share a hash", () => {
    const out = t(`<p>{a ? "A" : "B"}{a ? "A" : "B"}</p>`);
    const hash = hashKey("A", "p::0");
    expect([...out.matchAll(new RegExp(`__t\\("${hash}"`, "g"))]).toHaveLength(2);
    expectParses(out);
  });

  it("touches nothing where extraction emits nothing", () => {
    expect(transform(`<code>{a ? "x = 1" : "y = 2"}</code>`, "Test.tsx", { mode: "runtime" })).toBe(
      null,
    );
    expect(
      transform(`<div translate="no">{a ? "A" : "B"}</div>`, "Test.tsx", { mode: "runtime" }),
    ).toBe(null);
  });
});

describe("inline mode bakes each branch", () => {
  it("swaps both literals for their translations", () => {
    const dir = dictDir("de", {
      [hashKey("Saving...", "Button::0")]: "Wird gespeichert...",
      [hashKey("Save", "Button::1")]: "Speichern",
    });
    const out = t(SAVE, { mode: "inline", locale: "de", translationsDir: dir, strict: false });
    expect(out).toContain(`{saving ? "Wird gespeichert..." : "Speichern"}`);
    expect(out).not.toContain("__t");
    expectParses(out);
  });

  it("keeps the source literal when the catalog has no entry", () => {
    const out = t(SAVE, {
      mode: "inline",
      locale: "de",
      translationsDir: "./nonexistent",
      strict: false,
    });
    expect(out).toContain(`{saving ? "Saving..." : "Save"}`);
    expectParses(out);
  });
});

describe("extract and transform agree", () => {
  const shapes = [
    SAVE,
    `<div>Saved {cond ? "now" : "later"} today</div>`,
    `<div>{cond && "Folder moved"}</div>`,
    `<div>{label || "Untitled"}</div>`,
    `<p>Saved {a ? "A" : b ? "B" : "C"}</p>`,
    `<>{a ? "A" : "B"}</>`,
    `<div>{a ? someVar : "D"}</div>`,
    `<p>Text <span>{a ? "A" : "B"}</span> more</p>`,
    `<div>Prefix {a ? "A" : <span>B here</span>} tail</div>`,
    `<div title={a ? "A" : "B"}>{c ? "C" : "D"}</div>`,
  ];

  for (const code of shapes) {
    it(`emits the same hashes for ${code}`, () => {
      const extracted = new Set(blocks(code).map((b) => b.hash));
      const transformed = new Set([...t(code).matchAll(/__tx?\("([^"]+)"/g)].map((m) => m[1]));
      expect([...transformed].sort()).toEqual([...extracted].sort());
    });
  }
});
