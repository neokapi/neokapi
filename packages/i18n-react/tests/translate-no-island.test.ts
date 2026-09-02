/**
 * `translate="no"` marks an untranslatable island, at every depth.
 *
 * W3C reads the attribute as covering a whole subtree, so a parent
 * carrying one keeps its own text out of the message and travels as
 * an opaque placeholder the runtime substitutes verbatim. Two
 * consequences the extractor and the transform share, both routed
 * through `extract/translatable.ts` and `extract/runs.ts` so hash
 * parity holds by construction:
 *
 *   - a container or unmapped component whose only text sits inside
 *     such a child has no translatable text, so it stays unpromoted;
 *   - inside a parent that does earn a block, the child becomes a
 *     standalone `{=mN}` rather than a paired `{=mN}…{/=mN}`.
 *
 * The defect this guards: `<SimpleTooltip><span translate="no">
 * {file}:{key}</span></SimpleTooltip>` on the desktop Review page
 * shipped a file path into the source catalog as a message.
 */

import { createElement, Fragment, isValidElement } from "react";
import { describe, expect, it } from "vitest";

import type { Block, PlaceholderRun } from "@neokapi/kapi-format";

import { extractDocument, type ExplainDecision } from "../src/extract/walker.ts";
import { transform } from "../src/plugin/transform.ts";
import { __tx, setTranslations } from "../src/runtime/index.ts";

function blocks(code: string): Block[] {
  return extractDocument(code, { filename: "Island.tsx" })?.blocks ?? [];
}

function transformed(code: string): string {
  return transform(code, "Island.tsx", { mode: "runtime", onWarning: () => {} })?.code ?? "";
}

function explain(code: string): ExplainDecision[] {
  const decisions: ExplainDecision[] = [];
  extractDocument(code, { filename: "Island.tsx", onDecision: (d) => decisions.push(d) });
  return decisions;
}

describe('a translate="no" child holds no translatable text', () => {
  const cases: ReadonlyArray<{ name: string; code: string }> = [
    {
      name: "unmapped component wrapping a path span",
      code: '<SimpleTooltip content={label}><span translate="no">{file}:{key}</span></SimpleTooltip>',
    },
    {
      name: "container wrapping a path span",
      code: '<div><span translate="no">{path}</span></div>',
    },
    {
      name: "translatable element wrapping a literal identifier",
      code: '<p><span translate="no">/etc/hosts</span></p>',
    },
    {
      name: "nested inside another inline element",
      code: '<div><span><em translate="no">{path}</em></span></div>',
    },
  ];

  for (const { name, code } of cases) {
    it(`emits no block: ${name}`, () => {
      expect(blocks(code)).toHaveLength(0);
      expect(transformed(code)).toBe("");
    });
  }

  it("still promotes the same shape without the attribute", () => {
    const [block] = blocks(
      "<SimpleTooltip content={label}><span>Open the file</span></SimpleTooltip>",
    );
    expect(block).toBeTruthy();
    expect(
      transformed("<SimpleTooltip content={label}><span>Open the file</span></SimpleTooltip>"),
    ).toContain("__tx(");
  });
});

describe('a translate="no" child inside a translated parent', () => {
  const code = '<div>Saved to <span translate="no">{path}</span> just now</div>';

  it("becomes a standalone placeholder, never a paired range", () => {
    const [block] = blocks(code);
    expect(block).toBeTruthy();
    expect(block.source).toHaveLength(3);
    expect(block.source[0]).toEqual({ text: "Saved to " });
    const ph = block.source[1] as PlaceholderRun;
    expect(ph.ph.type).toBe("jsx:element");
    expect(ph.ph.subType).toBe("span");
    expect(ph.ph.equiv).toBe("=m0");
    expect(ph.ph.data).toBe('<span translate="no">{path}</span>');
    expect(block.source[2]).toEqual({ text: " just now" });
    // No pcOpen/pcClose anywhere: the island's content stays out.
    for (const run of block.source) {
      expect(run).not.toHaveProperty("pcOpen");
      expect(run).not.toHaveProperty("pcClose");
    }
  });

  it("binds the whole element in the runtime call", () => {
    const out = transformed(code);
    expect(out).toContain('"Saved to {=m0} just now"');
    expect(out).toContain('"=m0": <span translate="no">{path}</span>');
    expect(out).not.toContain("{/=m0}");
  });

  it("keeps the message free of the island's own text", () => {
    const [block] = blocks(code);
    const flat = block.source.map((r) => ("text" in r ? r.text : "")).join("");
    expect(flat).toBe("Saved to  just now");
  });

  it("leaves a plain inline child paired", () => {
    const [block] = blocks("<div>Saved to <span>{path}</span> just now</div>");
    expect(block.source.some((r) => "pcOpen" in r)).toBe(true);
  });
});

describe("explain reports the island", () => {
  it("names the parent's missing text and the child's opt-out", () => {
    const decisions = explain(
      '<SimpleTooltip content={label}><span translate="no">{file}:{key}</span></SimpleTooltip>',
    );
    const parent = decisions.find((d) => d.tag === "SimpleTooltip");
    const child = decisions.find((d) => d.tag === "span");
    expect(parent?.outcome).toBe("skipped-no-text");
    expect(child?.outcome).toBe("skipped-translate-no");
  });
});

describe("__tx() shapes its return around its consumers", () => {
  it("returns the element itself when the message is one element", () => {
    setTranslations("", {});
    const span = createElement("span", { className: "truncate" }, "src/App.tsx:title");
    const result = __tx("hash-island", "{=m0}", { "=m0": span });
    // Radix `asChild` clones its single child to pass refs and ARIA
    // props; a Fragment takes neither, and React warns on each prop.
    expect(isValidElement(result)).toBe(true);
    expect((result as { type: unknown }).type).toBe("span");
    expect((result as { props: { children: unknown } }).props.children).toBe("src/App.tsx:title");
    expect((result as { props: { className: string } }).props.className).toBe("truncate");
  });

  it("keeps the island's children when it renders through a message", () => {
    setTranslations("", {});
    const span = createElement("span", null, "/var/log/kapi.log");
    const result = __tx("hash-mixed", "Saved to {=m0} just now", { "=m0": span });
    const children = (result as { props: { children: unknown[] } }).props.children;
    const found = children.find(
      (c) => isValidElement(c) && (c as { type: unknown }).type === "span",
    ) as { props: { children: unknown } } | undefined;
    expect(found?.props.children).toBe("/var/log/kapi.log");
  });

  it("still returns a Fragment for a multi-part message", () => {
    setTranslations("", {});
    const icon = createElement("svg", null);
    const result = __tx("hash-multi", "{=m0} Run", { "=m0": icon });
    expect(isValidElement(result)).toBe(true);
    expect((result as { type: unknown }).type).toBe(Fragment);
    const children = (result as { props: { children: unknown[] } }).props.children;
    expect(children).toHaveLength(2);
  });

  it("keeps a leading space that the message asked for", () => {
    setTranslations("", {});
    const icon = createElement("svg", null);
    const result = __tx("hash-space", " {=m0}", { "=m0": icon });
    const children = (result as { props: { children: unknown[] } }).props.children;
    expect(children).toHaveLength(2);
    expect(children[0]).toBe(" ");
  });
});
