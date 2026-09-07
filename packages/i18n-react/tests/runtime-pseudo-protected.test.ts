/**
 * Runtime pseudo mode over a message that holds a code span.
 *
 * The catalog build protects the text inside a `<code>`, a `<kbd>`, a `<samp>`
 * or a `<var>`: it is a command, a key the reader presses, sample output or an
 * identifier, and accenting it hands the reader a command that does not run.
 * The browser-console transform reaches the same message through `__tx`, so it
 * gets the same answers and leaves the same characters alone.
 *
 * The chain under test runs end to end: the extractor decides who owns the text
 * inside each paired element, the compiler writes those answers into the
 * `__tx` call, and `pseudoTransform` reads them off the context.
 */

import { describe, it, expect, beforeEach } from "vitest";

import { extractDocument } from "../src/extract/index.ts";
import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { __tx, setStringTransform, setTranslations } from "../src/runtime/index.ts";
import { protectionMask } from "../src/runtime/markers.ts";
import { pseudoTransform, setPseudoMode } from "../src/runtime/pseudo.ts";

const PREFIX = "▒ ";
const SUFFIX = " ▒";

/** The body between the default wrap markers. */
function body(out: string): string {
  return out.slice(PREFIX.length, out.length - SUFFIX.length);
}

function compile(code: string): string {
  return transform(code, "Test.tsx", { mode: "runtime" })?.code ?? "";
}

function extract(code: string) {
  const doc = extractDocument(code, { filename: "Test.tsx" });
  expect(doc, "expected a document").not.toBeNull();
  return doc;
}

describe("protectionMask", () => {
  it("returns null when no marker answers no", () => {
    expect(protectionMask("Say {=m0}json{/=m0} now", undefined)).toBeNull();
    expect(protectionMask("Say {=m0}json{/=m0} now", {})).toBeNull();
    expect(protectionMask("Say {=m0}json{/=m0} now", { "=m0": "yes" })).toBeNull();
  });

  it("covers the characters between a protected pair and nothing else", () => {
    const text = "Say {=m0}json{/=m0} now";
    const mask = protectionMask(text, { "=m0": "no" });
    expect(mask).not.toBeNull();
    const covered = [...text]
      .map((ch, i) => (mask?.[i] ? ch : ""))
      .join("")
      .trim();
    expect(covered).toBe("json");
  });

  it("lets an inner pair inherit the protection of the pair around it", () => {
    const text = "Run {=m0}kapi {=m1}up{/=m1}{/=m0} today";
    const mask = protectionMask(text, { "=m0": "no" });
    const covered = [...text].map((ch, i) => (mask?.[i] ? ch : "")).join("");
    expect(covered).toContain("kapi ");
    expect(covered).toContain("up");
    expect(covered).not.toContain("today");
  });

  it("opens a hole where an inner pair answers yes", () => {
    const text = "Run {=m0}kapi {=m1}now{/=m1}{/=m0} please";
    const mask = protectionMask(text, { "=m0": "no", "=m1": "yes" });
    const covered = [...text].map((ch, i) => (mask?.[i] ? ch : "")).join("");
    expect(covered).toContain("kapi ");
    expect(covered).not.toContain("now");
  });

  it("leaves a standalone marker's surroundings alone", () => {
    const text = "Click {=m0} then {=m1}build{/=m1} it";
    const mask = protectionMask(text, { "=m1": "no" });
    const covered = [...text]
      .map((ch, i) => (mask?.[i] ? ch : ""))
      .join("")
      .trim();
    expect(covered).toBe("build");
  });
});

describe("pseudoTransform with protected markers", () => {
  it("accents the prose and carries the code span through verbatim", () => {
    const out = pseudoTransform(
      "Say {=m0}json{/=m0} for the readers.",
      {},
      { markers: { "=m0": "no" } },
    );
    expect(out).toContain("{=m0}json{/=m0}");
    expect(body(out).startsWith("Šàý")).toBe(true); // Šàý
  });

  it("carries a kbd hint through verbatim", () => {
    const out = pseudoTransform("Press {=m0}Ctrl{/=m0} to stop", {}, { markers: { "=m0": "no" } });
    expect(out).toContain("{=m0}Ctrl{/=m0}");
    expect(out).toContain("ţö"); // ţö
  });

  it("keeps a flag inside the span, so the command still runs", () => {
    const out = pseudoTransform(
      "Run {=m0}kapi check --ship{/=m0} before release.",
      {},
      { markers: { "=m0": "no" } },
    );
    expect(out).toContain("kapi check --ship");
  });

  it("accents the span when no marker answers", () => {
    const out = pseudoTransform("Say {=m0}json{/=m0} for the readers.");
    expect(out).toContain("{=m0}ĵšöñ{/=m0}"); // ĵšöñ
  });

  it("accents the span when its element answers yes", () => {
    const out = pseudoTransform(
      "Say {=m0}json{/=m0} for the readers.",
      {},
      { markers: { "=m0": "yes" } },
    );
    expect(out).toContain("{=m0}ĵšöñ{/=m0}");
  });

  it("accents the re-opened text inside a protected span", () => {
    const out = pseudoTransform(
      "Run {=m0}kapi {=m1}now{/=m1}{/=m0} please",
      {},
      { markers: { "=m0": "no", "=m1": "yes" } },
    );
    expect(out).toContain("{=m0}kapi {=m1}ñöŵ{/=m1}{/=m0}"); // ñöŵ
  });

  it("leaves protected characters out of the expansion count", () => {
    const text = "ab {=m0}wxyz{/=m0} cd";
    const markers = { "=m0": "no" } as const;
    // Four letters outside the span at 100% → four fillers, one per letter.
    const out = pseudoTransform(text, { expansion: 100 }, { markers });
    expect((out.match(/·/g) ?? []).length).toBe(4);
    expect(out).toContain("{=m0}wxyz{/=m0}");
  });

  it("places no word-boundary filler inside a protected span", () => {
    const out = pseudoTransform(
      "one {=m0}a b c d{/=m0} two",
      { expansion: 50 },
      { markers: { "=m0": "no" } },
    );
    expect(out).toContain("{=m0}a b c d{/=m0}");
  });

  it("still preserves an ordinary {param} inside a protected span", () => {
    const out = pseudoTransform("Run {=m0}kapi {cmd}{/=m0} now", {}, { markers: { "=m0": "no" } });
    expect(out).toContain("{=m0}kapi {cmd}{/=m0}");
  });
});

describe("the extractor's answer for a paired element", () => {
  it("answers no for a code span", () => {
    const doc = extract("<p>Say <code>json</code> for the readers.</p>");
    expect(doc?.blocks).toHaveLength(1);
    const compiled = compile("<p>Say <code>json</code> for the readers.</p>");
    expect(compiled).toContain('{ "=m0": "no" }');
  });

  it.each(["code", "kbd", "samp", "var"])("answers no for a <%s>", (tag) => {
    const compiled = compile(`<p>Press <${tag}>Ctrl</${tag}> to stop.</p>`);
    expect(compiled).toContain('{ "=m0": "no" }');
  });

  it("answers yes when the span opts its text back in", () => {
    const compiled = compile('<p>Say <code translate="yes">yes</code> to it.</p>');
    expect(compiled).toContain('{ "=m0": "yes" }');
  });

  it("says nothing for an ordinary inline element", () => {
    const compiled = compile("<p>Read <strong>this</strong> first.</p>");
    expect(compiled).toContain("__tx(");
    expect(compiled).not.toContain('"=m0": "no"');
    expect(compiled).not.toContain('"=m0": "yes"');
  });

  it("records both answers when a span re-opens inside a code", () => {
    const compiled = compile(
      '<p>Run <code>kapi <span translate="yes">now</span></code> please.</p>',
    );
    expect(compiled).toContain('{ "=m1": "yes", "=m0": "no" }');
  });

  it("passes undefined for params when a message has markers and no params", () => {
    const compiled = compile("<p>Say <code>json</code> now.</p>");
    expect(compiled).toContain('}, undefined, { "=m0": "no" })');
  });

  it("keeps the params argument in place when the message has both", () => {
    const compiled = compile("<p>Say <code>json</code> to {name} now.</p>");
    expect(compiled).toMatch(/\{ "name": name \}, \{ "=m0": "no" \}\)/);
  });

  it("leaves the hash untouched", () => {
    const doc = extract("<p>Say <code>json</code> for the readers.</p>");
    expect(doc?.blocks[0].hash).toBe(hashKey("Say {=m0}json{/=m0} for the readers.", "p"));
  });
});

describe("setPseudoMode over a __tx call site", () => {
  const SOURCE = "Say {=m0}json{/=m0} for the readers.";

  beforeEach(() => {
    setPseudoMode(null);
    setStringTransform(null);
    setTranslations("", {});
  });

  // Binding no element makes `__tx` return the message as a plain string: the
  // pair's inner content flows on its own. The rendered shape, with a real
  // `<code>` around it, is covered in runtime-pseudo-protected-render.test.tsx.
  it("leaves the code span alone and accents the prose around it", () => {
    setPseudoMode({});
    const out = __tx(hashKey(SOURCE, "p"), SOURCE, {}, undefined, { "=m0": "no" });
    expect(out).toBe(
      "\u2592 \u0160\u00e0\u00fd json \u0192\u00f6\u0155 \u0163\u0125\u00e9 \u0155\u00e9\u00e0\u0111\u00e9\u0155\u0161. \u2592",
    );
    setPseudoMode(null);
  });

  it("accents the span when the call site declares no answers", () => {
    setPseudoMode({});
    const out = __tx(hashKey(SOURCE, "p"), SOURCE, {});
    expect(out).toBe(
      "\u2592 \u0160\u00e0\u00fd \u0135\u0161\u00f6\u00f1 \u0192\u00f6\u0155 \u0163\u0125\u00e9 \u0155\u00e9\u00e0\u0111\u00e9\u0155\u0161. \u2592",
    );
    setPseudoMode(null);
  });

  it("hands the markers to any string transform, not only pseudo", () => {
    let seen: unknown;
    setStringTransform((text, context) => {
      seen = context?.markers;
      return text;
    });
    const source = "Press {=m0}Ctrl{/=m0} to stop";
    __tx(hashKey(source, "p"), source, {}, undefined, { "=m0": "no" });
    expect(seen).toEqual({ "=m0": "no" });
    setStringTransform(null);
  });

  it("protects the span in a translation the catalog supplies", () => {
    const hash = hashKey(SOURCE, "p");
    setTranslations("nb", { [hash]: "Si {=m0}json{/=m0} for leserne." });
    setPseudoMode({});
    const out = __tx(hash, SOURCE, {}, undefined, { "=m0": "no" });
    expect(out).toContain("json");
    expect(out).not.toContain("\u0135\u0161\u00f6\u00f1");
    setPseudoMode(null);
  });
});
