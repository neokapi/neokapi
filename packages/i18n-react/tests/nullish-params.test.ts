import { describe, it, expect } from "vitest";
import { paramText, resolveICU } from "../src/runtime/icu.ts";
import { __t } from "../src/runtime/index.ts";

/**
 * The transform lifts every expression inside a translated text run into a
 * string parameter. `Words{sortIndicator(field)}` is ordinary JSX — the
 * indicator is `null` on the columns that are not sorted, and React renders
 * that as nothing — but once it is a message parameter, "renders as nothing"
 * has to be arranged rather than inherited. It was not, and the header read
 * "Wordsnull".
 */
describe("a parameter React would render as nothing", () => {
  it.each([
    ["null", null],
    ["undefined", undefined],
    ["false", false],
    ["true", true],
  ])("contributes no text when it is %s", (_label, value) => {
    expect(paramText(value)).toBe("");
  });

  it("still renders the values that do have text", () => {
    expect(paramText("↑")).toBe("↑");
    expect(paramText(0)).toBe("0");
    expect(paramText(12)).toBe("12");
  });

  it("leaves no trace in a substituted message", () => {
    expect(__t("h", "Words{indicator}", { indicator: null })).toBe("Words");
    expect(__t("h", "Words{indicator}", { indicator: " ↑" })).toBe("Words ↑");
  });

  it("leaves no trace in an ICU substitution", () => {
    expect(resolveICU("Words{indicator}", { indicator: null }, "en")).toBe("Words");
  });

  /**
   * A name the call site never supplied is a different thing from one it
   * supplied as null: the first is a catalog referring to a placeholder that
   * does not exist, which is worth seeing rather than silently swallowing.
   */
  it("keeps the token for a name nothing bound", () => {
    expect(resolveICU("Words{missing}", { other: 1 }, "en")).toBe("Words{missing}");
  });

  it("falls back to nothing rather than to 'null' on an unformattable value", () => {
    expect(resolveICU("{n, number}", { n: null }, "en")).toBe("");
    expect(resolveICU("{d, date}", { d: null }, "en")).toBe("");
  });
});
