import { describe, it, expect } from "vitest";
import {
  hasRedactionWrap,
  redactionIncomplete,
  wrapWithRedaction,
  unwrapRedaction,
} from "../redactionWrap";
import type { FlowSpec } from "../types";

describe("redaction wrap", () => {
  const base: FlowSpec = { steps: [{ tool: "translate" }] };

  it("wraps a flow with redact first and unredact last, as ordered steps", () => {
    const wrapped = wrapWithRedaction(base);
    expect(wrapped.steps.map((s) => s.tool)).toEqual(["redact", "translate", "unredact"]);
    expect(hasRedactionWrap(wrapped)).toBe(true);
  });

  it("is idempotent (no duplicate redact/unredact)", () => {
    const once = wrapWithRedaction(base);
    const twice = wrapWithRedaction(once);
    expect(twice.steps.filter((s) => s.tool === "redact")).toHaveLength(1);
    expect(twice.steps.filter((s) => s.tool === "unredact")).toHaveLength(1);
    expect(twice.steps.map((s) => s.tool)).toEqual(["redact", "translate", "unredact"]);
  });

  it("prepends redact ahead of existing leading transformers", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "case-transform" }, { tool: "x" }],
    };
    const wrapped = wrapWithRedaction(spec);
    expect(wrapped.steps.map((s) => s.tool)).toEqual(["redact", "case-transform", "x", "unredact"]);
  });

  it("unwraps both redact and unredact, wherever they sit", () => {
    const wrapped = wrapWithRedaction(base);
    const unwrapped = unwrapRedaction(wrapped);
    expect(unwrapped.steps.map((s) => s.tool)).toEqual(["translate"]);
    expect(hasRedactionWrap(unwrapped)).toBe(false);
  });

  it("flags an incomplete wrap (redact without unredact)", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "redact" }, { tool: "translate" }],
    };
    expect(redactionIncomplete(spec)).toBe(true);
    expect(redactionIncomplete(wrapWithRedaction(spec))).toBe(false);
    expect(redactionIncomplete(base)).toBe(false);
  });
});
