/**
 * The explain decision channel: every element reports its
 * classification, gate outcome, and hash.
 */

import { describe, expect, it } from "vitest";

import { extractDocument, type ExplainDecision } from "../src/extract/walker.ts";

function explain(code: string): ExplainDecision[] {
  const decisions: ExplainDecision[] = [];
  extractDocument(code, { filename: "Explain.tsx", onDecision: (d) => decisions.push(d) });
  return decisions;
}

describe("onDecision explain channel", () => {
  it("reports extraction with classification and hash", () => {
    const [d] = explain("<h1>Hello</h1>");
    expect(d.tag).toBe("h1");
    expect(d.classification).toBe("yes");
    expect(d.outcome).toBe("extracted");
    expect(d.hash).toBeTruthy();
  });

  it("reports container promotion distinctly", () => {
    const div = explain("<div>Label</div>").find((d) => d.tag === "div");
    expect(div?.outcome).toBe("extracted-promoted");
  });

  it("reports translate=\"no\" skips", () => {
    const [d] = explain('<h1 translate="no">Skip</h1>');
    expect(d.outcome).toBe("skipped-translate-no");
  });

  it("reports non-translatable classification", () => {
    const code = explain("<code>x = 1</code>").find((d) => d.tag === "code");
    expect(code?.classification).toBe("no");
    expect(code?.outcome).toBe("skipped-not-translatable");
  });

  it("reports block-children skips while children extract", () => {
    const ds = explain("<div><p>Body</p></div>");
    const div = ds.find((d) => d.tag === "div");
    const p = ds.find((d) => d.tag === "p");
    expect(div?.outcome).toBe("skipped-block-children");
    expect(p?.outcome).toBe("extracted");
  });

  it("attaches attribute hashes to the element's decision", () => {
    const [d] = explain('<input placeholder="Search..." />');
    expect(Object.keys(d.attributes)).toEqual(["placeholder"]);
    expect(d.attributes.placeholder).toBeTruthy();
    expect(d.outcome).toBe("skipped-no-text");
  });
});
