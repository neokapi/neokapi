import { ApiError } from "@neokapi/ui";
import { describe, expect, it } from "vitest";

import { isElevationRequired } from "./webauthn";

describe("isElevationRequired", () => {
  // Regression: adding a passkey returned 409 { error: "elevation_required", message:
  // "This action needs a recent security check…" }. The code is on ApiError.code; the
  // human sentence is Error.message. The old substring-on-message check silently missed
  // it, so the step-up never started and the raw error was shown.
  it("detects the 409 contract by code even when the message is a human sentence", () => {
    const err = new ApiError({
      message: "This action requires a recent security verification. Verify again to continue.",
      status: 409,
      code: "elevation_required",
      bodyText: "",
    });
    expect(isElevationRequired(err)).toBe(true);
  });

  it("is false for an unrelated ApiError", () => {
    const err = new ApiError({
      message: "Project limit reached",
      status: 403,
      code: "project_limit_reached",
      bodyText: "",
    });
    expect(isElevationRequired(err)).toBe(false);
  });

  it("duck-types on code for a plain envelope object", () => {
    expect(isElevationRequired({ code: "elevation_required" })).toBe(true);
  });

  it("falls back to the message marker for a non-envelope Error", () => {
    expect(isElevationRequired(new Error("elevation_required"))).toBe(true);
    expect(isElevationRequired(new Error("something else"))).toBe(false);
  });

  it("ignores non-errors", () => {
    expect(isElevationRequired(null)).toBe(false);
    expect(isElevationRequired("elevation_required")).toBe(false);
    expect(isElevationRequired(undefined)).toBe(false);
  });
});
