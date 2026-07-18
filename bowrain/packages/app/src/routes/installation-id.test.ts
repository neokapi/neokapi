import { describe, expect, it } from "vitest";
import { coerceInstallationId } from "./installation-id";

describe("coerceInstallationId", () => {
  it("passes a string id through", () => {
    expect(coerceInstallationId("147350515")).toBe("147350515");
  });

  it("coerces the JSON-parsed numeric id GitHub's redirect produces", () => {
    expect(coerceInstallationId(147350515)).toBe("147350515");
  });

  it("rejects empties and non-ids", () => {
    expect(coerceInstallationId("")).toBeUndefined();
    expect(coerceInstallationId(undefined)).toBeUndefined();
    expect(coerceInstallationId(null)).toBeUndefined();
    expect(coerceInstallationId(Number.NaN)).toBeUndefined();
    expect(coerceInstallationId({})).toBeUndefined();
  });
});
