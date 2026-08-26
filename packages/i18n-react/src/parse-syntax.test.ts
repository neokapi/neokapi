import { describe, expect, it } from "vitest";

import { parseSyntaxFor } from "./parse-syntax.ts";

// The extractor and the transform both parse source, and both used to decide
// TypeScript by `.tsx` alone. A .ts module was parsed as ECMAScript, threw on
// its first type annotation, and was skipped. Fixing one side moved nothing
// visible: the labels reached the dictionary and still rendered in English,
// because the half that reads a file and the half that rewrites it disagreed
// about what a TypeScript file is.
describe("parseSyntaxFor", () => {
  it("treats .ts as TypeScript, not ECMAScript", () => {
    expect(parseSyntaxFor("labels.ts")).toEqual({
      syntax: "typescript",
      tsx: false,
      jsx: false,
    });
  });

  it("keeps JSX on for .tsx", () => {
    expect(parseSyntaxFor("Component.tsx")).toEqual({
      syntax: "typescript",
      tsx: true,
      jsx: false,
    });
  });

  it("leaves plain JavaScript alone", () => {
    expect(parseSyntaxFor("script.js")).toEqual({
      syntax: "ecmascript",
      tsx: false,
      jsx: false,
    });
    expect(parseSyntaxFor("Component.jsx")).toEqual({
      syntax: "ecmascript",
      tsx: false,
      jsx: true,
    });
  });

  it("covers the module variants", () => {
    expect(parseSyntaxFor("a.mts").syntax).toBe("typescript");
    expect(parseSyntaxFor("a.cts").syntax).toBe("typescript");
    expect(parseSyntaxFor("a.mjs").syntax).toBe("ecmascript");
    expect(parseSyntaxFor("a.cjs").syntax).toBe("ecmascript");
  });

  // tsx must not be on for a .ts file: there `<Foo>x` is a type assertion, and
  // parsing it as JSX changes what the code means rather than merely failing.
  it("never enables tsx for a non-x TypeScript file", () => {
    for (const f of ["a.ts", "a.mts", "a.cts"]) {
      expect(parseSyntaxFor(f).tsx).toBe(false);
    }
  });

  it("answers for a full path, not just a bare name", () => {
    expect(parseSyntaxFor("/abs/src/pages/format-maturity/_types.ts").syntax).toBe("typescript");
  });
});
