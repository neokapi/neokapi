import { describe, it, expect } from "vite-plus/test";
import { parseMergeError } from "./merge";

describe("parseMergeError", () => {
  it("extracts conflicts from a 409 body", () => {
    const body = JSON.stringify({
      error: "change-set has stale-draft conflicts; re-base the listed ops and resubmit",
      conflicts: [
        {
          seq: 2,
          concept_id: "c-checkout",
          reason: "op authored against revision 4 but concept is at revision 5",
        },
      ],
    });
    const parsed = parseMergeError(new Error(`409: ${body}`));
    expect(parsed.conflicts).toHaveLength(1);
    expect(parsed.conflicts[0].seq).toBe(2);
    expect(parsed.conflicts[0].concept_id).toBe("c-checkout");
    expect(parsed.message).toContain("stale-draft conflicts");
  });

  it("passes a non-conflict status error through with no conflicts", () => {
    const parsed = parseMergeError(
      new Error('409: {"error":"governed change-set requires an approval from someone else"}'),
    );
    expect(parsed.conflicts).toEqual([]);
    expect(parsed.message).toContain("approval from someone else");
  });

  it("keeps a plain (non-status) error message intact", () => {
    const parsed = parseMergeError(new Error("network unreachable"));
    expect(parsed.message).toBe("network unreachable");
    expect(parsed.conflicts).toEqual([]);
  });

  it("falls back when the body is not JSON", () => {
    const parsed = parseMergeError(new Error("500: internal error"));
    expect(parsed.message).toBe("500: internal error");
    expect(parsed.conflicts).toEqual([]);
  });

  it("handles a non-Error value", () => {
    const parsed = parseMergeError("boom");
    expect(parsed.message).toBe("Merge failed.");
    expect(parsed.conflicts).toEqual([]);
  });
});
