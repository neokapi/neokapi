import { describe, it, expect } from "vitest";
import {
  checkIssueTone,
  findingFails,
  findingSeverityBadgeClass,
  findingSeverityTone,
  findingToneBadgeClass,
  findingToneTextClass,
  type FindingTone,
} from "../lib/finding-severity";

// The scale is core/check.Severity, and its weights say where the line falls:
// neutral 0, minor 1, major 5, critical 25. Two of those stop a unit.
describe("findingSeverityTone", () => {
  const cases: [string | undefined, FindingTone][] = [
    ["critical", "destructive"],
    ["major", "destructive"],
    ["minor", "warning"],
    ["neutral", "muted"],
    ["", "muted"],
    [undefined, "muted"],
    ["catastrophic", "muted"],
  ];

  for (const [severity, tone] of cases) {
    it(`reads ${severity ?? "an absent severity"} as ${tone}`, () => {
      expect(findingSeverityTone(severity)).toBe(tone);
    });
  }

  it("reads a severity whatever its casing", () => {
    expect(findingSeverityTone("MAJOR")).toBe("destructive");
    expect(findingSeverityTone("Minor")).toBe("warning");
  });

  it("fails exactly the two severities that stop a unit", () => {
    expect(["critical", "major"].every(findingFails)).toBe(true);
    expect(["minor", "neutral", "", "unheard-of"].some(findingFails)).toBe(false);
  });
});

describe("checkIssueTone", () => {
  it("maps the server's shorter scale onto the same tones", () => {
    expect(checkIssueTone("error")).toBe("destructive");
    expect(checkIssueTone("warning")).toBe("warning");
    expect(checkIssueTone("")).toBe("muted");
  });
});

describe("the classes a tone paints", () => {
  it("gives a failing finding the destructive pair and a minor one warning", () => {
    expect(findingToneBadgeClass("destructive")).toContain("destructive");
    expect(findingToneBadgeClass("warning")).toContain("warning");
    expect(findingToneBadgeClass("muted")).toBe("text-muted-foreground");
  });

  it("paints major as hard as critical, which amber did not", () => {
    expect(findingSeverityBadgeClass("major")).toBe(findingSeverityBadgeClass("critical"));
    expect(findingSeverityBadgeClass("major")).not.toBe(findingSeverityBadgeClass("minor"));
  });

  it("gives ink alone for a message and the icon beside it", () => {
    expect(findingToneTextClass("destructive")).toBe("text-destructive");
    expect(findingToneTextClass("warning")).toBe("text-warning");
    expect(findingToneTextClass("muted")).toBe("text-muted-foreground");
  });
});
