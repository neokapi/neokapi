import { describe, expect, it } from "vitest";
import data from "./KapiLessonLifecycleEvidence.json";
import { parseCheckAnswer, parseContextAnswer } from "./KapiLessonRecords";

describe("native lifecycle evidence", () => {
  it("preserves the difference between suggestions and established rules", () => {
    const checks = data.steps.map((step) => parseCheckAnswer(step.check.stdout));
    expect(checks.every(Boolean)).toBe(true);
    expect(checks.map((check) => check?.pass)).toEqual([true, true, false, false, false, true]);
    expect(checks[1]?.summary.reporting).toBeGreaterThan(0);
    expect(checks[1]?.summary.failing).toBe(0);
    expect(data.steps.every((step) => parseContextAnswer(step.context.stdout))).toBe(true);
  });
  it("records native graph freshness and located term occurrences", () => {
    const edited = data.steps.find((step) => step.label === "Source edited")!;
    const refreshed = data.steps.find((step) => step.label === "Source read again")!;
    expect(parseContextAnswer(edited.context.stdout)?.provenance?.stale).toBe(true);
    expect(parseContextAnswer(refreshed.context.stdout)?.provenance?.stale).toBe(false);
    expect(
      parseContextAnswer(refreshed.search.stdout)?.terms?.some(
        (term) =>
          (term.uses ?? 0) > 0 && term.top_uses?.some((use) => use.document === "guide.json"),
      ),
    ).toBe(true);
  });
  it("rejects malformed or incompatible evidence without throwing", () => {
    for (const value of ["not json", "null", "[]", '{"terms":42}', '{"terms":[{"term":false}]}']) {
      expect(parseContextAnswer(value)).toBeNull();
    }
    for (const value of ["not json", "null", "[]", "{}", '{"pass":"true"}']) {
      expect(parseCheckAnswer(value)).toBeNull();
    }
  });
});
