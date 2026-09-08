/**
 * `{" "}` is a space, not a variable.
 *
 * JSX drops the whitespace around a line break, so an author who wants a space
 * between an element and the word after it writes `{" "}`. Read as an
 * expression it became a `jsx:var` named `value`, and the catalog carried a
 * placeholder token for a character: a translator saw `Read the {=m0}docs{/=m0}
 * {value} for more.` and had to guess what `{value}` stood for, with nothing
 * stopping them dropping it or moving it into the middle of a word.
 *
 * It is text now, so the message reads as the sentence it is.
 */

import { describe, expect, it } from "vitest";

import type { Block, Run, TextRun } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";

function blocks(code: string): Block[] {
  return extractDocument(code, { filename: "Test.tsx" })?.blocks ?? [];
}

function onlyBlock(code: string): Block {
  const found = blocks(code);
  expect(found, "expected exactly one block").toHaveLength(1);
  return found[0];
}

function textRun(run: Run | undefined): TextRun {
  if (!run || !("text" in run)) throw new Error(`expected TextRun, got ${JSON.stringify(run)}`);
  return run;
}

/** The names the block records as placeholders. */
function placeholderNames(block: Block): string[] {
  return (block.placeholders ?? []).map((p) => p.name);
}

function compile(code: string, mode: "runtime" | "inline" = "runtime"): string {
  return transform(code, "Test.tsx", { mode })?.code ?? "";
}

const DOCS = '<p>Read the <a href="/d">docs</a>{" "}for more.</p>';

describe("a whitespace-only expression", () => {
  it("is a space in the block text", () => {
    const block = onlyBlock(DOCS);
    expect(block.hash).toBe(hashKey("Read the {=m0}docs{/=m0} for more.", "p"));
  });

  it("leaves no placeholder behind", () => {
    expect(placeholderNames(onlyBlock(DOCS))).toEqual(["=m0"]);
  });

  it("joins the text run beside it", () => {
    const block = onlyBlock(DOCS);
    expect(block.source).toHaveLength(5);
    expect(textRun(block.source[4]).text).toBe(" for more.");
  });

  it.each([
    ["double quotes", '<p>Read the <b>docs</b>{" "}now.</p>'],
    ["single quotes", "<p>Read the <b>docs</b>{' '}now.</p>"],
    ["a template literal", "<p>Read the <b>docs</b>{` `}now.</p>"],
  ])("reads %s the same way", (_name, code) => {
    expect(onlyBlock(code).hash).toBe(hashKey("Read the {=m0}docs{/=m0} now.", "p"));
  });

  it("collapses a run of whitespace the way JSX text is collapsed", () => {
    const block = onlyBlock('<p>Read the <b>docs</b>{"  \\n  "}now.</p>');
    expect(block.hash).toBe(hashKey("Read the {=m0}docs{/=m0} now.", "p"));
  });

  it("does not renumber the element markers around it", () => {
    const block = onlyBlock('<p>Save{" "}<b>now</b> and <i>later</i>.</p>');
    expect(block.hash).toBe(hashKey("Save {=m0}now{/=m0} and {=m1}later{/=m1}.", "p"));
  });

  it("is trimmed at the edges of the block, like any other whitespace", () => {
    const block = onlyBlock('<p>{" "}Read the <b>docs</b>.{" "}</p>');
    expect(block.hash).toBe(hashKey("Read the {=m0}docs{/=m0}.", "p"));
  });

  it("keeps a literal with real text as a variable", () => {
    const block = onlyBlock('<p>Read the <b>docs</b>{"!"} now.</p>');
    expect(placeholderNames(block)).toContain("value");
  });

  it("keeps an ordinary expression as a variable", () => {
    const block = onlyBlock("<p>Read the <b>docs</b> {name} now.</p>");
    expect(placeholderNames(block)).toContain("name");
  });

  it("stays a space inside a code span, where the text is protected", () => {
    const block = onlyBlock('<p>Run <code>kapi{" "}up</code> first.</p>');
    expect(block.hash).toBe(hashKey("Run {=m0}kapi up{/=m0} first.", "p"));
    const protectedText = block.source.flatMap((r) =>
      "text" in r && r.noTranslate ? [r.text] : [],
    );
    expect(protectedText).toEqual(["kapi up"]);
  });

  it("stays a space inside a plural form", () => {
    const found = blocks(
      "<p><Plural count={n}><One>one <b>file</b>{\' \'}left</One>" +
        "<Other>{n} <b>files</b>{\' \'}left</Other></Plural></p>",
    );
    expect(found).toHaveLength(1);
    const wrapper = found[0].source[0];
    if (!("plural" in wrapper)) throw new Error("expected a plural run");
    for (const form of ["one", "other"] as const) {
      const runs = wrapper.plural.forms[form] ?? [];
      const tail = runs[runs.length - 1];
      expect(tail && "text" in tail ? tail.text : null).toBe(" left");
    }
  });

  it("gives an element holding only a space no block of its own", () => {
    expect(blocks('<p>{" "}</p>')).toHaveLength(0);
  });
});

describe("the compiled call site", () => {
  it("binds no param for the space in runtime mode", () => {
    const code = compile(DOCS);
    expect(code).toContain("__tx(");
    expect(code).toContain("Read the {=m0}docs{/=m0} for more.");
    expect(code).not.toContain('"value"');
  });

  it("keeps the space in the inline replacement", () => {
    const code = compile(DOCS, "inline");
    expect(code).toContain("</a> for more.");
  });

  it("agrees with extraction on the hash", () => {
    const block = onlyBlock(DOCS);
    expect(compile(DOCS)).toContain(`__tx("${block.hash}"`);
  });

  it("still binds a param for a literal with real text", () => {
    expect(compile('<p>Read the <b>docs</b>{"!"} now.</p>')).toContain('"value"');
  });
});
