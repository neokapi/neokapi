/**
 * Prose that contains a code span.
 *
 * `<code>`, `<kbd>`, `<samp>` and `<var>` hold a command, a key, sample output
 * or an identifier, and they sit inside a sentence a reader reads. So the
 * element travels with its parent's block as a paired code and its text comes
 * out as a `noTranslate` run: the sentence is translated whole, and the bytes
 * between the tags are the bytes the author wrote.
 *
 * The failing shape this covers: an element with a `<code>` child used to be
 * skipped entirely, so every paragraph mentioning a format id or a flag stayed
 * English while its neighbours were translated.
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

/** The text carried through verbatim, in order. */
function protectedText(block: Block): string[] {
  return block.source.flatMap((run) => ("text" in run && run.noTranslate ? [run.text] : []));
}

describe("a code span inside prose", () => {
  it("extracts the whole sentence as one block", () => {
    const block = onlyBlock("<p>Say <code>json</code> for the faithful readers.</p>");
    expect(block.source).toHaveLength(5);
    expect(textRun(block.source[0]).text).toBe("Say ");
    expect(pcOpen(block.source[1]).subType).toBe("code");
    expect(textRun(block.source[2]).text).toBe("json");
    expect(pcClose(block.source[3]).subType).toBe("code");
    expect(textRun(block.source[4]).text).toBe(" for the faithful readers.");
    expect(block.hash).toBe(hashKey("Say {=m0}json{/=m0} for the faithful readers.", "p"));
  });

  it("marks the span's text noTranslate and leaves the prose editable", () => {
    const block = onlyBlock("<p>Say <code>json</code> for the faithful readers.</p>");
    expect(protectedText(block)).toEqual(["json"]);
    expect(editable(block)).toEqual(["Say ", " for the faithful readers."]);
  });

  it("gives the span the same paired shape a <strong> gets", () => {
    const block = onlyBlock("<p>Say <code>json</code> now.</p>");
    const open = pcOpen(block.source[1]);
    const close = pcClose(block.source[3]);
    expect(open.type).toBe("jsx:element");
    expect(open.data).toBe("<code>");
    expect(close.data).toBe("</code>");
    expect(open.id).toBe(close.id);
    expect(open.equiv).toBe(close.equiv);
    expect(block.placeholders.map((p) => p.name)).toEqual(["=m0"]);
  });

  it("carries the span through the transform in place", () => {
    const out = t("<p>Say <code>json</code> for the faithful readers.</p>");
    expect(out).toContain("__tx(");
    expect(out).toContain('"Say {=m0}json{/=m0} for the faithful readers."');
    expect(out).toContain('"=m0": <code>json</code>');
  });

  it("handles the span as the first child", () => {
    const block = onlyBlock("<p><code>kapi up</code> converges the project.</p>");
    expect(protectedText(block)).toEqual(["kapi up"]);
    expect(editable(block)).toEqual([" converges the project."]);
  });

  it("handles the span as the last child", () => {
    const block = onlyBlock("<p>Run it with <code>--memory</code></p>");
    expect(editable(block)).toEqual(["Run it with "]);
    expect(protectedText(block)).toEqual(["--memory"]);
  });

  it("handles two spans in one sentence", () => {
    const block = onlyBlock("<p>Use <code>json</code> or <code>xliff</code> here.</p>");
    expect(protectedText(block)).toEqual(["json", "xliff"]);
    expect(editable(block)).toEqual(["Use ", " or ", " here."]);
    expect(block.placeholders.map((p) => p.name)).toEqual(["=m0", "=m1"]);
    expect(pcOpen(block.source[1]).id).not.toBe(pcOpen(block.source[5]).id);
  });

  it("protects a span nested inside a link", () => {
    const block = onlyBlock('<p>See <a href="/x">the <code>json</code> reader</a> now.</p>');
    expect(protectedText(block)).toEqual(["json"]);
    expect(editable(block)).toEqual(["See ", "the ", " reader", " now."]);
    expect(pcOpen(block.source[1]).subType).toBe("a");
    expect(pcOpen(block.source[3]).subType).toBe("code");
  });

  it("protects text nested under a span inside the code element", () => {
    const block = onlyBlock("<p>Say <code>js<em>on</em></code> now.</p>");
    expect(protectedText(block)).toEqual(["js", "on"]);
    expect(editable(block)).toEqual(["Say ", " now."]);
  });

  it("gives two back-to-back spans a pair and a marking each", () => {
    const block = onlyBlock("<p>Type <code>a</code><code>b</code> twice.</p>");
    expect(protectedText(block)).toEqual(["a", "b"]);
    expect(editable(block)).toEqual(["Type ", " twice."]);
    expect(block.source.filter((r) => "pcOpen" in r)).toHaveLength(2);
  });
});

describe("kbd, samp and var", () => {
  it.each([
    ["kbd", "<p>Press <kbd>Enter</kbd> to go.</p>", "Enter"],
    ["samp", "<p>It answers <samp>no such file</samp> here.</p>", "no such file"],
    ["var", "<p>Set <var>KAPI_HOME</var> first.</p>", "KAPI_HOME"],
  ])("protects the text inside <%s>", (tag, code, inner) => {
    const block = onlyBlock(code);
    expect(pcOpen(block.source[1]).subType).toBe(tag);
    expect(protectedText(block)).toEqual([inner]);
  });

  it("leaves the hash unchanged, because the flat template is the same string", () => {
    const block = onlyBlock("<p>Press <kbd>Enter</kbd> to go.</p>");
    expect(block.hash).toBe(hashKey("Press {=m0}Enter{/=m0} to go.", "p"));
  });
});

describe("an element holding nothing but a code span", () => {
  it("stays out of the catalog", () => {
    expect(blocks("<p><code>json</code></p>")).toHaveLength(0);
    expect(t("<p><code>json</code></p>")).toBeNull();
  });

  it("stays out when the span is wrapped in another inline element", () => {
    expect(blocks("<p><strong><code>json</code></strong></p>")).toHaveLength(0);
  });

  it("stays out when a componentMap entry resolves the component to code", () => {
    const opts = { componentMap: { Code: "code" } };
    expect(blocks("<p><Code>json</Code></p>", opts)).toHaveLength(0);
    expect(t("<p><Code>json</Code></p>", opts)).toBeNull();
  });

  it("stays out of a promoted container too", () => {
    expect(blocks("<div><code>json</code></div>")).toHaveLength(0);
  });

  it("still extracts the prose when the container has some", () => {
    const block = onlyBlock("<div>Read <code>json</code></div>");
    expect(editable(block)).toEqual(["Read "]);
    expect(protectedText(block)).toEqual(["json"]);
  });
});

describe('translate="yes" on a code span', () => {
  it("extracts a standalone span as ordinary prose", () => {
    const block = onlyBlock('<code translate="yes">Choose a file</code>');
    expect(editable(block)).toEqual(["Choose a file"]);
    expect(protectedText(block)).toEqual([]);
  });

  it("leaves the span's text editable inside a sentence", () => {
    const block = onlyBlock('<p>Run <code translate="yes">the thing</code> now.</p>');
    expect(protectedText(block)).toEqual([]);
    expect(editable(block)).toEqual(["Run ", "the thing", " now."]);
  });

  it("opts a nested element back in inside a protected span", () => {
    const block = onlyBlock('<p>Say <code>js<em translate="yes">on</em></code> now.</p>');
    expect(protectedText(block)).toEqual(["js"]);
    expect(editable(block)).toEqual(["Say ", "on", " now."]);
  });

  it("re-protects after the opted-in element closes", () => {
    const block = onlyBlock('<p>Say <code>a<em translate="yes">b</em>c</code>.</p>');
    expect(protectedText(block)).toEqual(["a", "c"]);
    expect(editable(block)).toEqual(["Say ", "b", "."]);
  });
});

describe('translate="no" still wins', () => {
  it("keeps the whole element opaque rather than paired", () => {
    const block = onlyBlock('<p>Saved to <code translate="no">/etc/kapi</code> just now</p>');
    expect(block.source.some((r) => "pcOpen" in r)).toBe(false);
    expect(protectedText(block)).toEqual([]);
    expect(editable(block)).toEqual(["Saved to ", " just now"]);
    expect(block.hash).toBe(hashKey("Saved to {=m0} just now", "p"));
  });
});

describe("extract and transform agree", () => {
  it.each([
    "<p>Say <code>json</code> for the faithful readers.</p>",
    "<p>Use <code>json</code> or <code>xliff</code> here.</p>",
    '<p>See <a href="/x">the <code>json</code> reader</a> now.</p>',
    "<p>Press <kbd>Enter</kbd> to go.</p>",
    '<code translate="yes">Choose a file</code>',
  ])("hashes %s identically on both sides", (code) => {
    const block = onlyBlock(code);
    const out = t(code);
    expect(out).not.toBeNull();
    expect(out).toContain(`"${block.hash}"`);
  });
});
