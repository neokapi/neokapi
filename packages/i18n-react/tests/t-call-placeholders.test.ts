/**
 * A `t()` argument is a placeholder, and is carried as one.
 *
 * The arguments of a `t("…", { … })` call are brace tokens the runtime
 * substitutes by literal match. Until they were lifted into runs they were
 * ordinary characters in an ordinary text run, so nothing that protects a
 * placeholder could see them: the editor drew them as prose, an AI pass read
 * them as words, and `DiffRunCodes` had no code to compare. These tests hold
 * the lifting to the two things it must not disturb — the flattened string the
 * runtime consumes, and the hash it looks that string up by.
 */

import { describe, expect, it } from "vitest";

import type { Block, PlaceholderRun, TextRun } from "@neokapi/kapi-format";
import { flattenRuns } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { CONTEXT_SEPARATOR } from "../src/types.ts";

const T_IMPORT = 'import { t } from "@neokapi/i18n-react/runtime";\n';

/** The single `js:t` block a one-call module extracts to. */
function tBlock(call: string): Block {
  const doc = extractDocument(`${T_IMPORT}export const x = ${call};`, { filename: "T.tsx" });
  if (!doc) throw new Error("expected a Document, got null");
  const blocks = doc.blocks.filter((b) => b.type === "js:t");
  expect(blocks, `expected exactly one t() block from ${call}`).toHaveLength(1);
  return blocks[0];
}

/** Each run as it reads: text verbatim, a placeholder as its equiv token. */
function shape(block: Block): string[] {
  return block.source.map((run) =>
    "text" in run ? (run as TextRun).text : `{${(run as PlaceholderRun).ph.equiv}}`,
  );
}

describe("t() arguments become placeholder runs", () => {
  it("lifts an argument out of the text as a jsx:var run", () => {
    const block = tBlock('t("use {replacement}", { replacement })');
    expect(block.source).toHaveLength(2);
    expect((block.source[0] as TextRun).text).toBe("use ");

    const { ph } = block.source[1] as PlaceholderRun;
    // jsx:var, not a new key: the vocabulary entry is the *variable* rendering,
    // and where the extractor found the token is not something a chip says.
    expect(ph.type).toBe("jsx:var");
    expect(ph.equiv).toBe("replacement");
    expect(ph.data).toBe("{replacement}");
  });

  it("records each argument once in the block's metadata table", () => {
    const block = tBlock('t("{count} of {total} clear the bar", { count, total })');
    expect(block.placeholders).toEqual([
      { name: "count", kind: "variable", sourceExpr: "count" },
      { name: "total", kind: "variable", sourceExpr: "total" },
    ]);
  });

  it("keeps the text before, between and after the arguments", () => {
    const block = tBlock('t("{approved} approved, {left} left", { approved, left })');
    expect(shape(block)).toEqual(["{approved}", " approved, ", "{left}", " left"]);
  });

  it("dedupes the metadata table but not the runs", () => {
    const block = tBlock('t("{name} met {name}", { name })');
    expect(block.placeholders).toHaveLength(1);
    expect(block.source.filter((run) => "ph" in run)).toHaveLength(2);
  });

  it("lifts a dotted member path", () => {
    const block = tBlock('t("Hello {user.name}", { "user.name": n })');
    expect(shape(block)).toEqual(["Hello ", "{user.name}"]);
    expect(block.placeholders[0].name).toBe("user.name");
  });
});

describe("t() placeholder lifting disturbs nothing downstream", () => {
  // The hash is the key a running component looks its string up by. Lifting is
  // a reading of the source string, so re-reading it must not re-key it — every
  // translated catalog entry keyed by the old hash would be orphaned.
  it("leaves the block hash a function of the raw argument", () => {
    // The descriptor a t() call hashes under: the "t" channel, no context.
    const desc = `t${CONTEXT_SEPARATOR}`;
    expect(tBlock('t("use {replacement}", { replacement })').hash).toBe(
      hashKey("use {replacement}", desc),
    );
    expect(tBlock('t("{count} left", { count })').hash).toBe(hashKey("{count} left", desc));
  });

  // What compile writes into the runtime dictionary is `flattenRuns(source)`.
  // A `ph` flattens back as `{equiv}`, so the shipped string is byte-identical
  // to the one the single text run produced.
  it("flattens back to the string the runtime substitutes into", () => {
    for (const text of [
      "use {replacement}",
      "{count} of {total} clear the bar",
      "{approved} approved, {left} left",
      "Hello {user.name}",
      "Save",
    ]) {
      const block = tBlock(`t(${JSON.stringify(text)})`);
      expect(flattenRuns(block.source), text).toBe(text);
    }
  });
});

describe("t() strings that are not argument tokens stay text", () => {
  // An ICU picker's braces belong to ICU, not to the substitution pass: the
  // argument, the categories and the `#` are one structure resolveICU parses at
  // render time, and placeholder-check already compares it argument by argument.
  it("leaves an ICU picker message whole", () => {
    const text = "{count, plural, one {# file} other {# files}}";
    const block = tBlock(`t(${JSON.stringify(text)}, { count })`);
    expect(block.source).toHaveLength(1);
    expect((block.source[0] as TextRun).text).toBe(text);
    expect(block.placeholders).toEqual([]);
  });

  it("leaves a string with no arguments as one text run", () => {
    const block = tBlock('t("Save")');
    expect(block.source).toHaveLength(1);
    expect((block.source[0] as TextRun).text).toBe("Save");
    expect(block.placeholders).toEqual([]);
  });

  // `{ name }` with spaces is not a token the runtime substitutes —
  // `replaceAll("{name}", …)` never matches it. Lifting it would flatten back
  // as `{name}` and quietly repair a string that renders its token today. An
  // extractor smuggling in a behaviour change is worse than the bug.
  it("leaves a brace the runtime would not substitute as text", () => {
    for (const text of ["use { replacement }", "a {} b", "{ 1 + 2 }", "{a-b}"]) {
      const block = tBlock(`t(${JSON.stringify(text)})`);
      expect(block.source, text).toHaveLength(1);
      expect((block.source[0] as TextRun).text, text).toBe(text);
      expect(block.placeholders, text).toEqual([]);
    }
  });
});
