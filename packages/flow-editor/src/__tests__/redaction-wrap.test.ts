import { describe, it, expect } from "vitest";
import {
  hasRedactionWrap,
  redactionIncomplete,
  wrapWithRedaction,
  unwrapRedaction,
} from "../redactionWrap";
import type { FlowSpec } from "../types";

describe("redaction wrap", () => {
  const base: FlowSpec = { steps: [{ tool: "ai-translate" }] };

  it("wraps a flow with redact (source-transform) + unredact (last)", () => {
    const wrapped = wrapWithRedaction(base);
    expect(wrapped.sourceTransforms).toEqual([{ tool: "redact" }]);
    expect(wrapped.steps.map((s) => s.tool)).toEqual(["ai-translate", "unredact"]);
    expect(hasRedactionWrap(wrapped)).toBe(true);
  });

  it("is idempotent (no duplicate redact/unredact)", () => {
    const once = wrapWithRedaction(base);
    const twice = wrapWithRedaction(once);
    expect(twice.sourceTransforms).toHaveLength(1);
    expect(twice.steps.filter((s) => s.tool === "unredact")).toHaveLength(1);
  });

  it("preserves existing source-transforms when wrapping", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "case-transform" }],
      steps: [{ tool: "x" }],
    };
    const wrapped = wrapWithRedaction(spec);
    expect(wrapped.sourceTransforms!.map((s) => s.tool)).toEqual(["redact", "case-transform"]);
  });

  it("unwraps both redact and unredact, dropping an empty source-transform array", () => {
    const wrapped = wrapWithRedaction(base);
    const unwrapped = unwrapRedaction(wrapped);
    expect(unwrapped.sourceTransforms).toBeUndefined();
    expect(unwrapped.steps.map((s) => s.tool)).toEqual(["ai-translate"]);
    expect(hasRedactionWrap(unwrapped)).toBe(false);
  });

  it("flags an incomplete wrap (redact without unredact)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };
    expect(redactionIncomplete(spec)).toBe(true);
    expect(redactionIncomplete(wrapWithRedaction(spec))).toBe(false);
    expect(redactionIncomplete(base)).toBe(false);
  });
});
