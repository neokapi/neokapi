import { describe, expect, it } from "vitest";
import { coerceInstallationId, searchInstallationId, searchSetupAction } from "./installation-id";

describe("searchInstallationId", () => {
  it("keeps the original type so the URL round-trips unquoted", () => {
    expect(searchInstallationId(147350515)).toBe(147350515);
    expect(searchInstallationId("147350515")).toBe("147350515");
  });

  it("rejects empties and non-ids", () => {
    expect(searchInstallationId("")).toBeUndefined();
    expect(searchInstallationId(Number.NaN)).toBeUndefined();
    expect(searchInstallationId(undefined)).toBeUndefined();
  });
});

describe("searchSetupAction", () => {
  it("passes GitHub's install and update actions through unchanged", () => {
    expect(searchSetupAction("install")).toBe("install");
    expect(searchSetupAction("update")).toBe("update");
  });

  it("drops anything else harmlessly", () => {
    expect(searchSetupAction("")).toBeUndefined();
    expect(searchSetupAction("request")).toBeUndefined();
    expect(searchSetupAction(1)).toBeUndefined();
    expect(searchSetupAction(undefined)).toBeUndefined();
    expect(searchSetupAction(null)).toBeUndefined();
  });
});

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
