import { describe, expect, it } from "vitest";

import { referenceTitle } from "./titles";

describe("referenceTitle", () => {
  it("appends the category to a name that does not carry it", () => {
    expect(referenceTitle("JSON", "format")).toBe("JSON format");
    expect(referenceTitle("Segment", "tool")).toBe("Segment tool");
  });

  it("names the category once when the display name ends in it", () => {
    // The two pages `kapi check` reported as hygiene.doubled-word.
    expect(referenceTitle("TSV Format", "format")).toBe("TSV Format");
    expect(referenceTitle("Open Document Format", "format")).toBe("Open Document Format");
  });

  it("matches the category regardless of case", () => {
    expect(referenceTitle("Rich Text FORMAT", "format")).toBe("Rich Text FORMAT");
  });

  it("keeps the category when the last word only ends in those letters", () => {
    // MessageFormat is the name of a specification, not the category word.
    expect(referenceTitle("ICU MessageFormat", "format")).toBe("ICU MessageFormat format");
  });
});
