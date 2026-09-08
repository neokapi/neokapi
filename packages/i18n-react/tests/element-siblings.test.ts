/**
 * A sibling expression whose value is a React element.
 *
 * Promotion folds a container into one block and lifts every sibling
 * expression into a named parameter, so `{icon}` beside a sentence reaches the
 * runtime as `{ icon: <Icon /> }`. A parameter used to substitute as text, and
 * text is what `String()` makes of a React element: the platform's translation
 * editor drew `[object Object]` across its progress bar where the coloured
 * segments belonged (#2561).
 *
 * Which of the two a parameter carries is knowable only once it has a value:
 * `{icon}`, `{rows}` and `{count}` are the same shape in the source. So the
 * transform sends every parameter-bearing block to `__tx`, which renders a
 * React-valued parameter as a node and everything else as text.
 *
 * Extraction is untouched by that, and these tests pin it: the flat template
 * still reads `{icon} Save changes`, the hash is the one the transform stamps,
 * and the translator still sees one named placeholder they can move.
 */

import { describe, expect, it } from "vitest";

import type { Block, Document, PlaceholderRun, Run, TextRun } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { transform } from "../src/plugin/transform.ts";
import type { PluginOptions } from "../src/types.ts";

function extract(code: string): Document | null {
  return extractDocument(code, { filename: "Test.tsx" });
}

function onlyBlock(code: string): Block {
  const found = extract(code)?.blocks ?? [];
  expect(found, "expected exactly one block").toHaveLength(1);
  return found[0];
}

function t(code: string, options: Partial<PluginOptions> = {}): string {
  const out = transform(code, "Test.tsx", { mode: "runtime", onWarning: () => {}, ...options });
  expect(out?.code, "expected the transform to rewrite this file").toBeTruthy();
  return out?.code ?? "";
}

function ph(run: Run | undefined): PlaceholderRun["ph"] {
  if (!run || !("ph" in run))
    throw new Error(`expected PlaceholderRun, got ${JSON.stringify(run)}`);
  return run.ph;
}

function text(run: Run | undefined): TextRun {
  if (!run || !("text" in run)) throw new Error(`expected TextRun, got ${JSON.stringify(run)}`);
  return run;
}

/** Every hash the transform stamps into a `__t` / `__tx` call. */
function transformHashes(code: string): string[] {
  return [...t(code).matchAll(/__tx?\("([^"]+)"/g)].map((m) => m[1]);
}

const ICON = "<div>{icon} Save changes</div>";
/** The shape the progress bar has: segments beside the inline child with the text. */
const PROGRESS = "<div>{segments}<span>{progress}% done</span></div>";

describe("extraction of a sibling expression", () => {
  it("keeps a named placeholder run for the expression", () => {
    const block = onlyBlock(ICON);
    expect(ph(block.source[0])).toMatchObject({ type: "jsx:var", equiv: "icon", data: "{icon}" });
    expect(text(block.source[1]).text).toBe(" Save changes");
  });

  it("names the placeholder in the block metadata", () => {
    const block = onlyBlock(ICON);
    expect(block.placeholders).toEqual([{ name: "icon", kind: "variable", sourceExpr: "icon" }]);
  });

  it("puts the expression and the inline child in one block", () => {
    const block = onlyBlock(PROGRESS);
    expect(ph(block.source[0]).equiv).toBe("segments");
    expect(block.source.map((run) => Object.keys(run)[0])).toEqual([
      "ph",
      "pcOpen",
      "ph",
      "text",
      "pcClose",
    ]);
  });

  it("keeps a JSX literal an element marker rather than a named parameter", () => {
    const block = onlyBlock("<div>{<Badge />} Save changes</div>");
    expect(ph(block.source[0])).toMatchObject({ type: "jsx:node", equiv: "=m0" });
  });

  it("carries a member expression and a call under their own names", () => {
    const block = onlyBlock("<div>{a.b} and {f(x)} here</div>");
    expect(ph(block.source[0]).equiv).toBe("a.b");
    expect(ph(block.source[2]).equiv).toBe("f");
  });
});

describe("the runtime call a parameter-bearing block gets", () => {
  it("routes through __tx so the value can be a node", () => {
    const out = t(ICON);
    expect(out).toContain("__tx(");
    expect(out).not.toContain("__t(");
  });

  it("passes the expression itself as the parameter value", () => {
    expect(t(ICON)).toContain('{ "icon": icon }');
  });

  it("carries an empty elements object when the block holds no inline element", () => {
    expect(t(ICON)).toContain('__tx("hjaLJYSFHe7", "{icon} Save changes", {}, ');
  });

  it("keeps the message text intact as the fallback", () => {
    // The old `__t` fallback was a template literal, which stringified the
    // element before any catalog was even consulted.
    expect(t(ICON)).not.toContain("`");
  });

  it("binds the inline element and the sibling expression side by side", () => {
    const out = t(PROGRESS);
    expect(out).toContain('{ "=m1": <span>{progress}% done</span> }');
    expect(out).toContain('{ "segments": segments, "progress": progress }');
  });

  it("escapes nothing and loses nothing for a dotted parameter name", () => {
    expect(t("<div>{a.b} and {f(x)} here</div>")).toContain('{ "a.b": a.b, "f": f(x) }');
  });

  it("still emits __t for a block with no parameters", () => {
    const out = t("<div>Save changes</div>");
    expect(out).toContain("__t(");
    expect(out).not.toContain("__tx(");
  });

  it("still emits __t when the only parameter is a plural pivot", () => {
    // A pivot is a number by construction, so it has no element to lose.
    const out = t("<p><Plural count={n}><One>1 item</One><Other>many items</Other></Plural></p>");
    expect(out).toContain("__t(");
    expect(out).not.toContain("__tx(");
  });

  it("still emits __t for a translatable attribute", () => {
    // An attribute answers with a string and has nowhere to put an element.
    const out = t('<input placeholder="Search" />');
    expect(out).toContain("__t(");
    expect(out).not.toContain("__tx(");
  });
});

describe("inline mode", () => {
  const dict = { hjaLJYSFHe7: "Ŝàvé çĥàñĝéš {icon}" };

  it("splices the expression back into the JSX", () => {
    const out = t(ICON, { mode: "inline", locale: "qps", strict: false, dict });
    expect(out).toContain("{icon}");
    expect(out).not.toContain("__t");
  });

  it("follows the translator when they move the placeholder", () => {
    const out = t(ICON, { mode: "inline", locale: "qps", strict: false, dict });
    expect(out.indexOf("{icon}")).toBeGreaterThan(out.indexOf("Ŝàvé"));
  });
});

describe("extract and transform agree", () => {
  const shapes = [
    ICON,
    PROGRESS,
    "<div>{<Badge />} Save changes</div>",
    "<div>{a.b} and {f(x)} here</div>",
    "<p>Saved {icon} now</p>",
    "<>{icon} Save changes</>",
    "<p>Press <kbd>K</kbd> {icon} to save</p>",
  ];

  for (const code of shapes) {
    it(`emits the same hashes for ${code}`, () => {
      const extracted = new Set((extract(code)?.blocks ?? []).map((b) => b.hash));
      const transformed = new Set(transformHashes(code));
      expect([...transformed].sort()).toEqual([...extracted].sort());
    });
  }
});
