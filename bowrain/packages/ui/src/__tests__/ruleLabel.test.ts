import { describe, expect, it } from "vitest";
import { ruleLabel } from "../components/ruleLabel";

describe("ruleLabel", () => {
  it("names a stored rule as the project calls it now", () => {
    expect(ruleLabel("aB3xY9zQ", { aB3xY9zQ: "Checks on push" })).toBe("Checks on push");
  });

  it("names a platform rule from the id, which carries the name", () => {
    expect(ruleLabel("builtin:auto-extract-on-push")).toBe("auto-extract-on-push");
  });

  it("falls back to the name recorded at dispatch when the id resolves to nothing", () => {
    expect(ruleLabel("aB3xY9zQ", {}, "checks-on-push")).toBe("checks-on-push");
  });

  it("shows the raw id when nothing else names the rule", () => {
    expect(ruleLabel("aB3xY9zQ")).toBe("aB3xY9zQ");
  });

  it("shows the recorded name for a record written before ids were carried", () => {
    expect(ruleLabel("", undefined, "checks-on-push")).toBe("checks-on-push");
    expect(ruleLabel(undefined)).toBe("");
  });
});
