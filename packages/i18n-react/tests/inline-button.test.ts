/**
 * Prose that contains a button, or any other phrasing-content control.
 *
 * HTML5 lets a control sit in the middle of a sentence, and `button` is written
 * that way often: "[Try it live] — list the registered formats". The paragraph
 * around such a control used to be disqualified whole, so the button's label
 * extracted on its own and the sentence beside it stayed English.
 *
 * A control joins its parent's block where the parent holds prose without it.
 * The control becomes a paired code and its label a translatable run inside, so
 * the sentence is one message and the label is still the translator's. Where
 * the control IS the parent's only content, the parent stays disqualified: the
 * walker descends, and `<div><Button>Save</Button></div>` keeps the block and
 * the key it has always had.
 */

import { describe, expect, it } from "vitest";

import type { Block, Document, PcCloseRun, PcOpenRun, Run, TextRun } from "@neokapi/kapi-format";

import { extractDocument } from "../src/extract/index.ts";
import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";
import type { PluginOptions } from "../src/types.ts";

function extract(code: string, filename = "Test.tsx", opts = {}): Document | null {
  return extractDocument(code, { filename, ...opts });
}

function blocks(code: string, opts = {}): Block[] {
  return extract(code, "Test.tsx", opts)?.blocks ?? [];
}

function onlyBlock(code: string, opts = {}): Block {
  const found = blocks(code, opts);
  expect(found, "expected exactly one block").toHaveLength(1);
  return found[0];
}

function textRun(run: Run | undefined): TextRun {
  if (!run || !("text" in run)) throw new Error(`expected TextRun, got ${JSON.stringify(run)}`);
  return run;
}

function pcOpen(run: Run | undefined): PcOpenRun["pcOpen"] {
  if (!run || !("pcOpen" in run)) throw new Error(`expected PcOpenRun, got ${JSON.stringify(run)}`);
  return run.pcOpen;
}

function pcClose(run: Run | undefined): PcCloseRun["pcClose"] {
  if (!run || !("pcClose" in run))
    throw new Error(`expected PcCloseRun, got ${JSON.stringify(run)}`);
  return run.pcClose;
}

function t(code: string, options: Partial<PluginOptions> = {}): string | null {
  return transform(code, "Test.tsx", { mode: "runtime", ...options })?.code ?? null;
}

/** The text a translator may edit, in order. */
function editable(block: Block): string[] {
  return block.source.flatMap((run) => ("text" in run && !run.noTranslate ? [run.text] : []));
}

/** The `/formats` case from #2460, with the live source's attributes. */
const FORMATS_PAGE = [
  "<p>",
  '<button type="button" className="button button--primary" onClick={() => setOpen(true)}>',
  '<Play size={16} aria-hidden="true" fill="currentColor" />',
  "Try it live",
  "</button>",
  '{" "}',
  "&mdash; list the registered formats from the real <code>kapi</code> binary, in an",
  "in-browser terminal.",
  "</p>",
].join("");

describe("a button inside prose", () => {
  it("extracts the sentence and the label as one block", () => {
    const block = onlyBlock("<p>Press <button>Go</button> to start.</p>");
    expect(block.properties?.element).toBe("p");
    expect(textRun(block.source[0]).text).toBe("Press ");
    expect(pcOpen(block.source[1]).subType).toBe("button");
    expect(textRun(block.source[2]).text).toBe("Go");
    expect(pcClose(block.source[3]).subType).toBe("button");
    expect(textRun(block.source[4]).text).toBe(" to start.");
    expect(block.hash).toBe(hashKey("Press {=m0}Go{/=m0} to start.", "p"));
  });

  it("leaves the label editable, unlike a code span", () => {
    const block = onlyBlock("<p>Press <button>Go</button> to start.</p>");
    expect(editable(block)).toEqual(["Press ", "Go", " to start."]);
    expect(block.source.some((r) => "text" in r && r.noTranslate)).toBe(false);
  });

  it("keeps the button's handler and classes on the paired code", () => {
    const block = onlyBlock('<p>Press <button type="button" onClick={run}>Go</button> now.</p>');
    expect(pcOpen(block.source[1]).data).toBe('<button type="button" onClick={run}>');
    expect(pcClose(block.source[3]).data).toBe("</button>");
    expect(pcOpen(block.source[1]).equiv).toBe(pcClose(block.source[3]).equiv);
  });

  it("resolves a component through componentMap", () => {
    const opts = { componentMap: { Button: "button" } };
    const block = onlyBlock("<div>Press <Button>Go</Button> to start.</div>", opts);
    expect(pcOpen(block.source[1]).subType).toBe("button");
    expect(editable(block)).toEqual(["Press ", "Go", " to start."]);
  });

  it("covers the whole /formats paragraph from #2460", () => {
    const block = onlyBlock(FORMATS_PAGE);
    // Every word of the sentence is in the one block: the button's label, the
    // prose after it, and the protected command in the middle.
    expect(editable(block).join("")).toContain("Try it live");
    expect(editable(block).join("")).toContain("list the registered formats");
    expect(editable(block).join("")).toContain("in-browser terminal.");
    const protectedRuns = block.source.filter((r) => "text" in r && r.noTranslate);
    expect(protectedRuns).toHaveLength(1);
    expect(textRun(protectedRuns[0]).text).toBe("kapi");
  });

  it("emits nothing but that one block for the /formats paragraph", () => {
    const found = blocks(FORMATS_PAGE);
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("p");
  });

  it("carries the button through the transform in place", () => {
    const out = t("<p>Press <button onClick={run}>Go</button> to start.</p>");
    expect(out).toContain("__tx(");
    expect(out).toContain('"Press {=m0}Go{/=m0} to start."');
    expect(out).toContain('"=m0": <button onClick={run}>Go</button>');
  });

  it("hashes the same on both sides", () => {
    const source = "<p>Press <button>Go</button> to start.</p>";
    const extracted = onlyBlock(source);
    const result = transform(source, "Test.tsx", { mode: "runtime" });
    expect(result?.hashes).toContain(extracted.hash);
  });

  it("takes the label with it when the prose sits in a sibling element", () => {
    const block = onlyBlock("<p><strong>Note:</strong> press <button>Go</button>.</p>");
    expect(editable(block)).toEqual(["Note:", " press ", "Go", "."]);
  });

  it("nests a code span inside the button's label", () => {
    const block = onlyBlock("<p>Run <button>the <code>up</code> loop</button> now.</p>");
    expect(editable(block)).toEqual(["Run ", "the ", " loop", " now."]);
    expect(block.source.filter((r) => "text" in r && r.noTranslate)).toHaveLength(1);
  });
});

describe("a button that is its parent's only content", () => {
  // The key `<button>Save</button>` has always had, spelled the way the walker
  // spells it. A change that folded such a button into its wrapper would move
  // every button label in the desktop and platform catalogs to a new key.
  const STANDALONE_HASH = hashKey("Save", "button");

  it("keeps its own block", () => {
    const found = blocks('<div className="toolbar"><button type="button">Save</button></div>');
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("button");
    expect(editable(found[0])).toEqual(["Save"]);
  });

  it("keeps its own hash, so no catalog entry moves", () => {
    const found = blocks('<div className="toolbar"><button type="button">Save</button></div>');
    expect(found[0].hash).toBe(STANDALONE_HASH);
  });

  it("keeps its own block inside a paragraph too", () => {
    const found = blocks("<p><button>Save</button></p>");
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("button");
  });

  it("keeps one block per button when a row holds several", () => {
    const found = blocks("<div><button>Save</button><button>Cancel</button></div>");
    expect(found.map((b) => b.properties?.element)).toEqual(["button", "button"]);
    expect(found.map((b) => editable(b).join(""))).toEqual(["Save", "Cancel"]);
  });

  it("is unaffected by an icon beside it", () => {
    const found = blocks("<div><Icon /><button>Save</button></div>");
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("button");
  });
});

describe("the other phrasing-content controls", () => {
  it("recovers the prose a label wraps around its input", () => {
    const block = onlyBlock('<label>Your name <input type="text" /></label>');
    expect(editable(block)).toEqual(["Your name "]);
    expect(block.hash).toBe(hashKey("Your name {=m0}", "label"));
  });

  it("folds a label that sits in a sentence", () => {
    const block = onlyBlock("<p>Fill in <label>your name</label> below.</p>");
    expect(editable(block)).toEqual(["Fill in ", "your name", " below."]);
  });

  it("leaves a label on its own as its own block", () => {
    const found = blocks("<div><label>Your name</label></div>");
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("label");
  });

  it("folds a select and its options into the sentence", () => {
    const block = onlyBlock("<p>Pick <select><option>Draft</option></select> now.</p>");
    expect(editable(block)).toEqual(["Pick ", "Draft", " now."]);
    expect(pcOpen(block.source[1]).subType).toBe("select");
    expect(pcOpen(block.source[2]).subType).toBe("option");
  });

  it.each([
    ["audio", '<p>Hear <audio src="/a.mp3" /> now.</p>'],
    ["img", '<p>See <img src="/x.png" /> above.</p>'],
    ["input", '<p>Type <input type="text" /> here.</p>'],
    ["meter", "<p>At <meter value={0.4} /> of the way.</p>"],
    ["output", "<p>It reads <output /> today.</p>"],
    ["progress", "<p>Now <progress value={40} /> done.</p>"],
    ["video", '<p>Watch <video src="/v.mp4" /> first.</p>'],
  ])("keeps the sentence around a bare <%s>", (tag, code) => {
    const block = onlyBlock(code);
    const ph = block.source.find((r) => "ph" in r);
    expect(ph && "ph" in ph ? ph.ph.subType : null).toBe(tag);
    expect(editable(block).join("").trim().length).toBeGreaterThan(0);
  });
});

describe("a control carrying a translatable attribute", () => {
  it("leaves an icon button's aria-label its own block", () => {
    const found = blocks('<p>Press <button aria-label="Run the demo"><Play /></button> now.</p>');
    expect(found).toHaveLength(1);
    expect(found[0].type).toBe("jsx:attribute");
    expect(editable(found[0])).toEqual(["Run the demo"]);
  });

  it("leaves an image's alt text its own block", () => {
    const found = blocks('<p>See <img src="/x.png" alt="the chart" /> above.</p>');
    expect(found.map((b) => b.type)).toEqual(["jsx:attribute"]);
    expect(editable(found[0])).toEqual(["the chart"]);
  });

  it("leaves an input's placeholder its own block", () => {
    const found = blocks('<label>Name <input placeholder="Jane" /></label>');
    expect(found.map((b) => b.type)).toEqual(["jsx:attribute"]);
    expect(editable(found[0])).toEqual(["Jane"]);
  });

  it("looks below the control, not only at its own tag", () => {
    const found = blocks('<p>Press <button><img alt="run" /></button> now.</p>');
    expect(found.map((b) => b.type)).toEqual(["jsx:attribute"]);
    expect(editable(found[0])).toEqual(["run"]);
  });

  it("declines to fold a button whose child prop carries copy", () => {
    const found = blocks('<p>Press <button>Go <Icon label="run" /></button> now.</p>');
    expect(found).toHaveLength(1);
    expect(found[0].properties?.element).toBe("button");
  });

  it("still folds a control whose attributes are all machine-facing", () => {
    const block = onlyBlock('<p>Press <button type="submit" name="go">Go</button> now.</p>');
    expect(editable(block)).toEqual(["Press ", "Go", " now."]);
  });

  it("agrees with the transform, which emits no op for the disqualified parent", () => {
    const out = t('<p>See <img src="/x.png" alt="the chart" /> above.</p>');
    expect(out).toContain("__t(");
    expect(out).not.toContain("__tx(");
  });
});

describe('translate="no" around a control', () => {
  it("keeps the paragraph out of the catalog", () => {
    expect(blocks('<p translate="no">Press <button>Go</button> now.</p>')).toHaveLength(0);
  });

  it("carries a marked button through as an opaque placeholder", () => {
    const block = onlyBlock('<p>Press <button translate="no">Go</button> now.</p>');
    expect(editable(block)).toEqual(["Press ", " now."]);
    const ph = block.source.find((r) => "ph" in r);
    expect(ph && "ph" in ph ? ph.ph.subType : null).toBe("button");
  });
});
