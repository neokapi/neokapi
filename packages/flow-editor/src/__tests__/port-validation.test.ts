import { describe, it, expect } from "vitest";

/**
 * Port validation logic — matches the isValidConnection callback in FlowEditor.
 * Extracted here for pure-function testing without React/DOM dependencies.
 */
function isPortCompatible(
  srcOutputs: string[] | undefined,
  tgtInputs: string[] | undefined,
): boolean {
  if (!srcOutputs || !tgtInputs) return true; // no metadata = allow
  return srcOutputs.some((o) => tgtInputs.includes(o));
}

describe("port validation", () => {
  it("tools with matching ports are compatible", () => {
    expect(isPortCompatible(["block"], ["block"])).toBe(true);
    expect(isPortCompatible(["block", "data"], ["block"])).toBe(true);
    expect(isPortCompatible(["data"], ["block", "data"])).toBe(true);
  });

  it("tools with non-overlapping ports are incompatible", () => {
    expect(isPortCompatible(["media"], ["block"])).toBe(false);
    expect(isPortCompatible(["data"], ["block", "layer"])).toBe(false);
    expect(isPortCompatible(["layer"], ["media"])).toBe(false);
  });

  it("tools without port metadata default to compatible", () => {
    expect(isPortCompatible(undefined, ["block"])).toBe(true);
    expect(isPortCompatible(["block"], undefined)).toBe(true);
    expect(isPortCompatible(undefined, undefined)).toBe(true);
  });

  it("empty arrays are incompatible", () => {
    expect(isPortCompatible([], ["block"])).toBe(false);
    expect(isPortCompatible(["block"], [])).toBe(false);
    expect(isPortCompatible([], [])).toBe(false);
  });

  it("multiple overlapping types still match", () => {
    expect(isPortCompatible(["block", "data", "media"], ["media", "layer"])).toBe(true);
  });
});
