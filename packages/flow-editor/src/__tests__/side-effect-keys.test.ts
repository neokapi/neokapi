import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { getSystemEffects } from "../sideEffects";

/**
 * Side-effect keys are a Go→TypeScript wire contract, not display vocabulary.
 *
 * `core/schema/annotations.go` declares the `SideEffect` string values that Go
 * stamps onto every tool's schema; `sideEffects.tsx` looks each one up to decide
 * which satellite badge to draw. A mismatch is invisible: `getSystemEffects`
 * skips keys it does not recognise, so a renamed key means the badge silently
 * stops rendering, with no failing build and no runtime error.
 *
 * That is exactly how a vocabulary sweep once turned `termbase-read` into
 * `terms-read` here while Go kept emitting `termbase-read`. This test reads the
 * Go constants directly so the two can never drift again — rename the labels
 * and descriptions freely, but the keys have to keep matching Go.
 */

const here = dirname(fileURLToPath(import.meta.url));
const annotationsPath = resolve(here, "../../../../core/schema/annotations.go");

/** Extract the `SideEffect` string values declared in the Go const block. */
function goSideEffectValues(): string[] {
  const src = readFileSync(annotationsPath, "utf8");
  const values: string[] = [];
  // e.g. `SideEffectTermsRead   SideEffect = "termbase-read"`
  const re = /\bSideEffect\w*\s+SideEffect\s*=\s*"([^"]+)"/g;
  for (const m of src.matchAll(re)) values.push(m[1]);
  return values;
}

describe("side-effect keys match the Go source of truth", () => {
  const goValues = goSideEffectValues();

  it("finds the Go SideEffect declarations", () => {
    // Guards the regex itself: if annotations.go moves or the const block is
    // reshaped, fail loudly here rather than vacuously passing an empty set.
    expect(goValues.length).toBeGreaterThanOrEqual(6);
    expect(goValues).toContain("tm-read");
    expect(goValues).toContain("termbase-read");
  });

  it.each([
    ["tm-read", "tm"],
    ["tm-write", "tm"],
    ["termbase-read", "terms"],
    ["termbase-write", "terms"],
    ["api-call", "api"],
  ])("resolves the Go value %s to a badge", (goValue, expectedKey) => {
    expect(goValues).toContain(goValue);
    const [effect, ...rest] = getSystemEffects([goValue]);
    expect(rest).toHaveLength(0);
    expect(effect, `no badge is registered for the Go side effect "${goValue}"`).toBeDefined();
    expect(effect.key).toBe(expectedKey);
  });

  it("resolves every store-backed Go side effect", () => {
    // Analytics and the remote-egress markers are deliberately not drawn as
    // satellites, so only assert the ones that carry a badge.
    const drawn = goValues.filter((v) => /^(tm|termbase|api)-/.test(v) || v === "api-call");
    for (const value of drawn) {
      expect(
        getSystemEffects([value]),
        `Go emits side effect "${value}" but sideEffects.tsx has no entry for it`,
      ).toHaveLength(1);
    }
  });

  it("pairs read and write into a single both-direction badge", () => {
    const effects = getSystemEffects(["termbase-read", "termbase-write"]);
    expect(effects).toHaveLength(1);
    expect(effects[0].direction).toBe("both");
  });
});
